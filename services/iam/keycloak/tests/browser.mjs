import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash, randomBytes, randomUUID } from 'node:crypto';
import { readFileSync, renameSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:http';
import { isIPv4 } from 'node:net';
import { setTimeout as pause } from 'node:timers/promises';
import { chromium } from 'playwright';

const origin = 'http://127.0.0.1:18080';
const realm = 'hit105-test';
const mailOrigin = 'http://127.0.0.1:18025';
const callback = 'http://127.0.0.1:18082/callback';
const form = '#hitpipe-email-code-form';
const secrets = new Set();
const callbacks = new Map();
const envFile = process.env.HIT105_COMPOSE_ENV;
const project = process.env.HIT105_COMPOSE_PROJECT;
const routeFile = process.env.HIT105_LAB_ROUTE;
assert.ok(envFile && routeFile && project === 'hit105-email-lab', 'Use tests/run.sh with a fresh isolated lab');

const fixture = Object.fromEntries(readFileSync(envFile, 'utf8').trim().split('\n').map(line => {
  const sep = line.indexOf('=');
  return [line.slice(0, sep), line.slice(sep + 1)];
}));
Object.values(fixture).forEach(value => { if (value.length > 8) secrets.add(value); });

function sanitized(error) {
  let msg = error instanceof Error ? error.message : String(error);
  for (const value of secrets) msg = msg.replaceAll(value, '[redacted]');
  return msg.replace(/\b\d{8}\b/g, '[redacted]')
    .replace(/([?&](?:code|code_verifier|session_code|password)=)[^&\s'"<>]+/gi, '$1[redacted]');
}

function compose(args, input, overrides) {
  return spawnSync('docker', [
    '--host', 'unix:///var/run/docker.sock', 'compose', '--env-file', envFile,
    '--project-name', project, '-f', 'compose.yaml', ...args,
  ], { input, env: { ...process.env, ...overrides }, encoding: 'utf8', timeout: 30_000 });
}

function isolatedAddress(service) {
  const id = compose(['ps', '--all', '-q', service]).stdout.trim();
  assert.match(id, /^[a-f0-9]{12,64}$/, 'Compose service has one local container');
  const inspect = spawnSync('docker', ['--host', 'unix:///var/run/docker.sock', 'inspect', '--format',
    '{{with index .NetworkSettings.Networks "hit105-email-lab_lab"}}{{.IPAddress}}{{end}}', id],
  { encoding: 'utf8', timeout: 5000 });
  const address = inspect.stdout.trim();
  assert.ok(inspect.status === 0 && isIPv4(address), 'Container has an address in the internal lab network');
  return address;
}

async function restartKeycloak(args, overrides) {
  assert.equal(compose(args, undefined, overrides).status, 0, 'Isolated Keycloak service restarted');
  writeFileSync(`${routeFile}.next`, `${isolatedAddress('keycloak')} ${isolatedAddress('smtp')}\n`, { mode: 0o600 });
  renameSync(`${routeFile}.next`, routeFile);
  await waitUntil(async () => {
    try {
      const r = await fetch(`${origin}/realms/${realm}/.well-known/openid-configuration`,
        { signal: AbortSignal.timeout(2000) });
      return r.ok;
    } catch { return false; }
  }, 'Imported realm did not recover after restart', 120_000);
}

function sql(query) {
  const p = compose(['exec', '-T', 'postgres', 'psql', '-X', '-qAt', '-v', 'ON_ERROR_STOP=1',
    '-U', 'hit105', '-d', 'hit105_keycloak'], query);
  if (p.status !== 0) throw new Error(`PostgreSQL query failed (${p.status ?? 'timeout'})`);
  return p.stdout.trim();
}

const literal = value => `'${value.replaceAll("'", "''")}'`;

function userRow(email) {
  const out = sql(`
    SELECT json_build_object('id',u.id,'verified',u.email_verified,'generation',c.generation,
      'attempts',c.failed_attempts,'consumed',c.consumed_at IS NOT NULL,'keyVersion',c.key_version,
      'macLengths',ARRAY[octet_length(c.email_mac),octet_length(c.flow_mac),octet_length(c.code_mac)])
    FROM user_entity u JOIN realm r ON r.id = u.realm_id
    LEFT JOIN hitpipe_email_code_challenge c ON c.realm_id = r.id AND c.user_id = u.id
    WHERE r.name = '${realm}' AND u.email = ${literal(email)};
  `);
  return out ? JSON.parse(out) : null;
}

function rateRows() {
  return JSON.parse(sql(`
    SELECT COALESCE(json_agg(json_build_object('digest',encode(t.recipient_mac,'hex'),
      'count',cardinality(t.send_times))),'[]')
    FROM hitpipe_email_code_rate t JOIN realm r ON r.id = t.realm_id WHERE r.name = '${realm}';
  `));
}

function rateFor(digest) {
  return rateRows().find(r => r.digest === digest);
}

function editRate(digest, expression) {
  assert.match(digest, /^[a-f0-9]{64}$/);
  sql(`UPDATE hitpipe_email_code_rate SET send_times = (${expression})
    WHERE realm_id = (SELECT id FROM realm WHERE name = '${realm}')
      AND recipient_mac = decode('${digest}', 'hex');`);
}

function ageSends(digest) {
  editRate(digest, `SELECT array_agg(t - interval '61 seconds' ORDER BY ord)
    FROM unnest(send_times) WITH ORDINALITY AS x(t, ord)`);
}

async function json(url, options) {
  const r = await fetch(url, { signal: AbortSignal.timeout(10_000), ...options });
  assert.ok(r.ok, `Local HTTP ${r.status} at ${new URL(url).pathname}`);
  return r.json();
}

async function localMessages(recipient) {
  const data = await json(`${mailOrigin}/api/v1/messages?limit=100`);
  return data.messages.filter(message => message.To.some(to => to.Address.toLowerCase() === recipient));
}

async function waitUntil(get, label, ms = 12_000) {
  const until = Date.now() + ms;
  do {
    const found = await get();
    if (found) return found;
    await pause(150);
  } while (Date.now() < until);
  throw new Error(label);
}

async function codeFromMail(recipient, count = 1) {
  const messages = await waitUntil(async () => {
    const all = await localMessages(recipient);
    return all.length === count && all;
  }, `Expected ${count} message(s) through SMTP`);
  const message = await json(`${mailOrigin}/api/v1/message/${encodeURIComponent(messages[0].ID)}`);
  const matches = message.Text?.match(/(?<!\d)[0-9]{8}(?!\d)/g) ?? [];
  assert.ok(matches.length === 1, 'SMTP text must contain one standalone 8-digit code');
  secrets.add(matches[0]);
  assert.ok(!/\/login-actions\/action-token\b/.test(message.Text), 'SMTP did not contain a native verification link');
  return matches[0];
}

async function startOAuth(page) {
  const state = randomUUID();
  const verifier = randomBytes(48).toString('base64url');
  secrets.add(verifier);
  const url = new URL(`${origin}/realms/${realm}/protocol/openid-connect/auth`);
  url.search = new URLSearchParams({
    client_id: 'hit105-browser', response_type: 'code', redirect_uri: callback,
    scope: 'openid', prompt: 'login', nonce: randomUUID(), state,
    code_challenge_method: 'S256', code_challenge: createHash('sha256').update(verifier).digest('base64url'),
  });
  await page.goto(url.href);
  return state;
}

async function beginRegistration(browser) {
  const ctx = await browser.newContext();
  const page = await ctx.newPage();
  const state = await startOAuth(page);
  const email = `${randomBytes(8).toString('hex')}@hit105.test`;
  const password = `Lab53!${randomBytes(24).toString('base64url')}`;
  secrets.add(password);
  const prior = new Set(rateRows().map(r => r.digest));
  const link = page.locator('a[href*="registrations"], a[href*="registration"]');
  await link.first().click();
  const optional = async (name, value) => {
    const input = page.locator(`input[name="${name}"]`);
    if (await input.isVisible()) await input.fill(value);
  };
  await optional('firstName', 'HIT');
  await optional('lastName', 'Case');
  await optional('username', email);
  await page.locator('input[name="email"]').fill(email);
  await page.locator('input[name="password"]').fill(password);
  await optional('password-confirm', password);
  return { ctx, page, email, password, state, prior };
}

async function completeRegistration(page) {
  await page.locator('#kc-register-form [type="submit"]').click();
  await page.locator(form).waitFor();
}

function newRate(prior) {
  const rates = rateRows().filter(row => !prior.has(row.digest));
  assert.equal(rates.length, 1, 'Registration writes a rate row for the synthetic recipient');
  assert.equal(rates[0].count, 1, 'First issue counts as an SMTP attempt');
  return rates[0].digest;
}

async function register(browser, whileSubmitting) {
  const u = await beginRegistration(browser);
  let actionReturned = false;
  const complete = completeRegistration(u.page).then(() => { actionReturned = true; });
  await Promise.all([complete, whileSubmitting?.(u.email, u.prior, () => actionReturned)]);
  const row = userRow(u.email);
  assert.ok(row && !row.verified && !row.consumed, 'Keycloak account remains unverified until the required action');
  assert.ok(!callbacks.has(u.state), 'OIDC callback is blocked before verification');
  return { ...u, rate: newRate(u.prior) };
}

async function loginWithoutRegistering(browser, credentials) {
  const ctx = await browser.newContext();
  const page = await ctx.newPage();
  const state = await startOAuth(page);
  await page.locator('input[name="username"]').fill(credentials.email);
  await page.locator('input[name="password"]').fill(credentials.password);
  await page.locator('#kc-form-login [type="submit"]').click();
  await page.locator(form).waitFor();
  return { ctx, page, state };
}

async function submitVisible(page, code) {
  await page.locator(`${form} input[name="code"]`).fill(code);
  await clickAndWait(page, `${form} button[name="operation"][value="verify"]`);
}

async function resendVisible(page) {
  await clickAndWait(page, `${form} button[name="operation"][value="resend"]`);
  await page.locator(form).waitFor();
}

async function clickAndWait(page, selector) {
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded' }),
    page.locator(selector).click(),
  ]);
}

async function readForm(page) {
  return page.locator(form).evaluate(element => ({
    url: element.action,
    fields: Object.fromEntries(new FormData(element)),
  }));
}

function submissionFields(request, operation, code, modifications) {
  return { ...request.fields, operation, ...(code ? { code } : {}), ...modifications };
}

async function formResponses(page, operation, code, modifications = {}, concurrency = 1, savedForm) {
  // Use the genuine form and this browser's own cookie jar. Calling its POST
  // directly checks how the server handles stale and competing submissions.
  const request = savedForm ?? await readForm(page);
  return Promise.all(Array.from({ length: concurrency }, () => page.request.post(request.url, {
    form: submissionFields(request, operation, code, modifications), maxRedirects: 0,
  })));
}

async function formPost(page, operation, code, modifications = {}, concurrency = 1, savedForm) {
  const responses = await formResponses(page, operation, code, modifications, concurrency, savedForm);
  return responses.map(r => {
    const location = r.headers().location;
    if (location) {
      const url = new URL(location, origin);
      const authorizationCode = url.searchParams.get('code');
      if (authorizationCode) secrets.add(authorizationCode);
      return url.origin === new URL(callback).origin && url.pathname === '/callback'
        && url.searchParams.has('code');
    }
    return false;
  });
}

async function postNavigation(page, operation, code, modifications = {}, suppliedForm) {
  const request = suppliedForm ?? await readForm(page);
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded' }),
    page.evaluate(({ url, fields }) => {
      const form = document.createElement('form');
      form.method = 'POST';
      form.action = url;
      for (const [name, value] of Object.entries(fields)) {
        const input = document.createElement('input');
        input.type = 'hidden';
        input.name = name;
        input.value = value;
        form.append(input);
      }
      document.body.append(form);
      form.submit();
    }, { url: request.url, fields: submissionFields(request, operation, code, modifications) }),
  ]);
}

async function newerServerForm(page, responses, generation) {
  for (const response of responses) {
    if (response.status() !== 200) continue;
    const state = await page.evaluate(({ markup, selector }) => {
      const document = new DOMParser().parseFromString(markup, 'text/html');
      const form = document.querySelector(selector);
      if (!form) return null;
      return { url: form.getAttribute('action'), fields: Object.fromEntries(new FormData(form)) };
    }, { markup: await response.text(), selector: form });
    if (state?.fields.generation === String(generation)) return state;
  }
  throw new Error('Neither parallel HTTP response offered a form for the committed generation');
}

async function assertSent(u) {
  const status = await u.page.locator(form).getAttribute('data-status');
  const allowed = ['sent', 'cooldown', 'rate_limited', 'invalid', 'locked', 'expired', 'stale', 'unavailable', 'awaiting', 'smtp_failed'];
  assert.equal(allowed.includes(status) ? status : 'missing-or-unrecognized', 'sent',
    `Issue status, database generation ${userRow(u.email)?.generation}, rate count ${rateFor(u.rate)?.count}`);
}

async function setChaos(recipientError) {
  await json(`${mailOrigin}/api/v1/chaos`, {
    method: 'PUT', headers: { 'content-type': 'application/json' },
    body: JSON.stringify(recipientError ? { Recipient: { ErrorCode: 451, Probability: 100 } } : {}),
  });
}

async function changeUser(id, method, changes) {
  const token = await json(`${origin}/realms/master/protocol/openid-connect/token`, {
    method: 'POST',
    body: new URLSearchParams({ grant_type: 'password', client_id: 'admin-cli',
      username: 'hit105-local-admin', password: fixture.HIT105_ADMIN_PASSWORD }),
  });
  secrets.add(token.access_token);
  const response = await fetch(`${origin}/admin/realms/${realm}/users/${encodeURIComponent(id)}`, {
    method, signal: AbortSignal.timeout(10_000),
    headers: { authorization: `Bearer ${token.access_token}`, 'content-type': 'application/json' },
    body: changes ? JSON.stringify(changes) : undefined,
  });
  assert.equal(response.status, 204, `Local realm administrator applied ${method} to the test user`);
}

async function run(name, test) {
  try {
    await test();
    console.log(`PASS ${name}`);
  } catch (error) {
    console.error(`FAIL ${name}: ${sanitized(error)}`);
    process.exitCode = 1;
    // The lab is temporary, but continuing after a failed test can obscure the
    // first failure with leftovers that are irrelevant to its diagnosis.
    throw new Error('test failed');
  }
}

const callbackServer = createServer((request, response) => {
  const url = new URL(request.url, callback);
  if (url.pathname === '/callback' && url.searchParams.has('code')) {
    secrets.add(url.searchParams.get('code'));
    const state = url.searchParams.get('state');
    callbacks.set(state, (callbacks.get(state) ?? 0) + 1);
  }
  response.writeHead(200, { 'content-type': 'text/plain' });
  response.end('Local test callback');
});

let browser;
try {
  await new Promise((resolve, reject) => {
    callbackServer.once('error', reject).listen(18082, '127.0.0.1', resolve);
  });
  browser = await chromium.launch({ headless: true });

  await run('public registration, genuine SMTP, keyboard submit and one-time verification', async () => {
    const u = await register(browser);
    try {
      const code = await codeFromMail(u.email);
      assert.deepEqual(userRow(u.email).macLengths, [32, 32, 32]);
      assert.ok(!(await u.page.content()).includes(code), 'Email code was absent from pre-submit HTML');
      const saved = await readForm(u.page);
      const input = u.page.locator(`${form} input[name="code"]`);
      await input.focus();
      await u.page.keyboard.type(code);
      await u.page.keyboard.press('Enter');
      await waitUntil(() => callbacks.get(u.state) === 1, 'No browser callback after submitting the real code');
      const after = userRow(u.email);
      assert.ok(after.verified && after.consumed, 'Database confirmed both markers after success');
      assert.equal(after.attempts, 0);
      assert.deepEqual(await formPost(u.page, 'verify', code, {}, 1, saved), [false]);
      assert.equal(callbacks.get(u.state), 1, 'Replayed submission did not create a second callback');
    } finally { await u.ctx.close(); }
  });

  await run('five wrong codes lock the challenge; the real code cannot unlock it', async () => {
    const u = await register(browser);
    try {
      const code = await codeFromMail(u.email);
      const wrong = code === '00000000' ? '00000001' : '00000000';
      for (let count = 1; count <= 5; count++) {
        await submitVisible(u.page, wrong);
        await u.page.locator(form).waitFor();
        assert.equal(userRow(u.email).attempts, count);
      }
      const rejected = await formPost(u.page, 'verify', code);
      assert.deepEqual(rejected, [false]);
      const current = userRow(u.email);
      assert.ok(!current.verified && !current.consumed && current.attempts === 5);
      assert.ok(!callbacks.has(u.state));
    } finally { await u.ctx.close(); }
  });

  await run('expired challenge and unknown key version fail closed without consuming a code', async () => {
    for (const update of [
      `issued_at = now() - interval '11 minutes', expires_at = now() - interval '1 minute'`,
      'key_version = 999',
    ]) {
      const u = await register(browser);
      try {
        const code = await codeFromMail(u.email);
        const account = userRow(u.email);
        sql(`UPDATE hitpipe_email_code_challenge SET ${update} WHERE user_id = ${literal(account.id)};`);
        assert.deepEqual(await formPost(u.page, 'verify', code), [false]);
        const current = userRow(u.email);
        assert.ok(!current.verified && !current.consumed && current.attempts === 0);
        assert.ok(!callbacks.has(u.state));
      } finally { await u.ctx.close(); }
    }
  });

  await run('resend enforces cooldown and rolling cap; changing generation invalidates the older code', async () => {
    const u = await register(browser);
    try {
      const code = await codeFromMail(u.email);
      const original = userRow(u.email).generation;
      await resendVisible(u.page);
      assert.equal(await u.page.locator(form).getAttribute('data-status'), 'cooldown');
      assert.equal(userRow(u.email).generation, original);
      assert.equal(rateFor(u.rate).count, 1);
      assert.equal((await localMessages(u.email)).length, 1);

      ageSends(u.rate);
      await resendVisible(u.page);
      await assertSent(u);
      const newCode = await codeFromMail(u.email, 2);
      assert.ok(newCode !== code, 'A replacement must not reuse the previous numeric code');
      assert.equal(userRow(u.email).generation, original + 1);
      await submitVisible(u.page, code);
      assert.equal(await u.page.locator(form).getAttribute('data-status'), 'invalid');
      assert.ok(!userRow(u.email).consumed);
      await postNavigation(u.page, 'verify', newCode, { generation: String(original) });
      assert.equal(await u.page.locator(form).getAttribute('data-status'), 'stale');
      assert.ok(!userRow(u.email).consumed);

      for (let count = 3; count <= 5; count++) {
        ageSends(u.rate);
        await resendVisible(u.page);
        await assertSent(u);
        await codeFromMail(u.email, count);
      }
      ageSends(u.rate);
      await resendVisible(u.page);
      assert.equal(await u.page.locator(form).getAttribute('data-status'), 'rate_limited');
      assert.equal(rateFor(u.rate).count, 5);
      assert.equal((await localMessages(u.email)).length, 5);
      editRate(u.rate, `SELECT array_agg(CASE WHEN ord = 1 THEN now() - interval '61 minutes'
        ELSE t END ORDER BY ord) FROM unnest(send_times) WITH ORDINALITY AS x(t, ord)`);
      await resendVisible(u.page);
      await assertSent(u);
      await codeFromMail(u.email, 6);
      assert.equal(rateFor(u.rate).count, 5);
    } finally { await u.ctx.close(); }
  });

  await run('a second browser flow keeps the rate limit and cannot use the first flow code', async () => {
    const u = await register(browser);
    let next;
    try {
      const code = await codeFromMail(u.email);
      next = await loginWithoutRegistering(browser, u);
      assert.equal(rateFor(u.rate).count, 1);
      assert.equal((await localMessages(u.email)).length, 1);
      assert.deepEqual(await formPost(next.page, 'verify', code), [false]);
      assert.ok(!userRow(u.email).verified && !callbacks.has(next.state));
    } finally {
      if (next) await next.ctx.close();
      await u.ctx.close();
    }
  });

  await run('a changed Keycloak email invalidates the outstanding code', async () => {
    const u = await register(browser);
    try {
      const code = await codeFromMail(u.email);
      const before = userRow(u.email);
      const flowMac = () => sql(`SELECT encode(flow_mac, 'hex') FROM hitpipe_email_code_challenge
        WHERE user_id = ${literal(before.id)};`);
      const originalFlow = flowMac();
      const changedEmail = `${randomBytes(8).toString('hex')}@hit105.test`;
      await changeUser(before.id, 'PUT', { email: changedEmail, emailVerified: false });
      assert.equal(userRow(changedEmail)?.id, before.id, 'Only the recipient of this synthetic user changed');
      await postNavigation(u.page, 'verify', code);
      assert.equal(await u.page.locator(form).getAttribute('data-status'), 'stale');
      const after = userRow(changedEmail);
      assert.ok(!after.verified && !after.consumed && !callbacks.has(u.state));
      assert.ok(flowMac() && flowMac() !== originalFlow, 'Email update revoked the outstanding flow');
      await changeUser(before.id, 'PUT', { email: u.email, emailVerified: false });
      assert.ok(flowMac() !== originalFlow, 'Restoring the address did not restore the old flow');
      await postNavigation(u.page, 'verify', code);
      assert.equal(await u.page.locator(form).getAttribute('data-status'), 'stale');
      assert.ok(!userRow(u.email).verified && !callbacks.has(u.state));
    } finally { await u.ctx.close(); }
  });

  await run('two parallel resends yield one replacement and one actual SMTP message', async () => {
    const u = await register(browser);
    try {
      await codeFromMail(u.email);
      const startGen = userRow(u.email).generation;
      ageSends(u.rate);
      const responses = await formResponses(u.page, 'resend', undefined, {}, 2);
      const replacement = await codeFromMail(u.email, 2);
      assert.equal(rateFor(u.rate).count, 2);
      assert.equal(userRow(u.email).generation, startGen + 1);
      const fresh = await newerServerForm(u.page, responses, startGen + 1);
      await postNavigation(u.page, 'verify', replacement, {}, fresh);
      await waitUntil(() => callbacks.get(u.state) === 1, 'Genuine refreshed form rejected the parallel-resend replacement');
      assert.ok(userRow(u.email).verified && userRow(u.email).consumed);
    } finally { await u.ctx.close(); }
  });

  await run('two parallel submits yield exactly one authorization and one database consumption', async () => {
    const u = await register(browser);
    try {
      const code = await codeFromMail(u.email);
      const results = await formPost(u.page, 'verify', code, {}, 2);
      assert.equal(results.filter(Boolean).length, 1, 'Exactly one competing submit received an OAuth callback');
      const current = userRow(u.email);
      assert.ok(current.verified && current.consumed);
    } finally { await u.ctx.close(); }
  });

  await run('an actual database rollback leaves both verification markers unset', async () => {
    const u = await register(browser);
    try {
      const code = await codeFromMail(u.email);
      sql(`
        CREATE FUNCTION hit105_test_abort_verify() RETURNS trigger LANGUAGE plpgsql AS $$
        BEGIN
          RAISE EXCEPTION 'HIT105 synthetic verification transaction rejection';
        END;
        $$;
        CREATE TRIGGER hit105_test_abort_verify BEFORE UPDATE OF email_verified ON user_entity
        FOR EACH ROW WHEN (OLD.email_verified IS FALSE AND NEW.email_verified IS TRUE)
        EXECUTE FUNCTION hit105_test_abort_verify();
      `);
      try {
        assert.deepEqual(await formPost(u.page, 'verify', code), [false]);
        const row = userRow(u.email);
        assert.ok(!row.verified && !row.consumed && !callbacks.has(u.state));
      } finally {
        sql('DROP TRIGGER hit105_test_abort_verify ON user_entity; DROP FUNCTION hit105_test_abort_verify();');
      }
    } finally { await u.ctx.close(); }
  });

  await run('actual SMTP 451 leaves the account unverified and retains cooldown', async () => {
    await setChaos(true);
    let u;
    try {
      u = await register(browser);
      assert.equal((await localMessages(u.email)).length, 0, 'Rejected mail did not reach the local receiver');
      assert.equal(await u.page.locator(form).getAttribute('data-status'), 'smtp_failed');
      await formPost(u.page, 'resend');
      const current = userRow(u.email);
      assert.ok(!current.verified && !current.consumed && !callbacks.has(u.state));
      assert.equal(rateFor(u.rate).count, 1, 'SMTP failure does not undo rate accounting');
    } finally {
      await setChaos(false);
      if (u) await u.ctx.close();
    }
  });

  await run('SMTP greeting timeout cannot hold the rate and challenge row locks', async () => {
    assert.equal(compose(['pause', 'smtp']).status, 0, 'Isolated SMTP receiver was paused');
    let u;
    try {
      u = await register(browser, async (email, prior, returned) => {
        const account = await waitUntil(() => {
          const row = userRow(email);
          return row?.generation && row;
        }, 'The challenge did not commit while SMTP awaited its greeting', 3500);
        const newRates = rateRows().filter(row => !prior.has(row.digest));
        assert.equal(newRates.length, 1);
        const rate = newRates[0].digest;
        assert.match(rate, /^[a-f0-9]{64}$/);
        assert.equal(sql(`
          BEGIN;
          SET LOCAL lock_timeout = '500ms';
          SELECT 1 FROM hitpipe_email_code_rate WHERE recipient_mac = decode('${rate}', 'hex') FOR UPDATE;
          SELECT 1 FROM hitpipe_email_code_challenge WHERE user_id = ${literal(account.id)} FOR UPDATE;
          ROLLBACK;
        `), '1\n1');
        assert.ok(!returned(), 'Both row locks were available before the SMTP call returned');
      });
      assert.equal(await u.page.locator(form).getAttribute('data-status'), 'smtp_failed');
      assert.ok(!userRow(u.email).verified && !callbacks.has(u.state));
    } finally {
      assert.equal(compose(['unpause', 'smtp']).status, 0, 'Isolated SMTP receiver resumed');
      if (u) await u.ctx.close();
    }
    assert.equal((await localMessages(u.email)).length, 0, 'A transport timeout did not deliver mail');
  });

  await run('deleting an IdP user removes their challenge', async () => {
    const u = await register(browser);
    try {
      await codeFromMail(u.email);
      const { id } = userRow(u.email);
      await changeUser(id, 'DELETE');
      assert.equal(userRow(u.email), null);
      assert.equal(sql(`SELECT count(*) FROM hitpipe_email_code_challenge WHERE user_id = ${literal(id)};`), '0');
    } finally { await u.ctx.close(); }
  });

  await run('restart after database issuance and before SMTP preserves cooldown and permits a later resend', async () => {
    const u = await beginRegistration(browser);
    let rate;
    let generation;
    let replied = false;
    assert.equal(compose(['pause', 'smtp']).status, 0, 'Isolated SMTP receiver paused before issuance');
    const pending = completeRegistration(u.page).then(() => { replied = true; }, () => undefined);
    try {
      const issued = await waitUntil(() => {
        const row = userRow(u.email);
        return row?.generation && row;
      }, 'No committed issuance was observed before restart', 4500);
      rate = newRate(u.prior);
      generation = issued.generation;
      assert.ok(!replied && !issued.verified && !issued.consumed && !callbacks.has(u.state));
      assert.equal(compose(['kill', '-s', 'SIGKILL', 'keycloak']).status, 0, 'Only the isolated Keycloak was interrupted');
    } finally {
      assert.equal(compose(['unpause', 'smtp']).status, 0, 'Isolated SMTP receiver resumed');
      await u.ctx.close();
      await pending;
    }
    assert.equal((await localMessages(u.email)).length, 0, 'No SMTP message was delivered before the interruption');
    await restartKeycloak(['start', 'keycloak']);
    assert.equal(userRow(u.email).generation, generation, 'Issue survived the IdP process interruption');
    const fresh = await loginWithoutRegistering(browser, u);
    try {
      assert.equal(await fresh.page.locator(form).getAttribute('data-status'), 'cooldown');
      assert.equal(rateFor(rate).count, 1);
      assert.equal((await localMessages(u.email)).length, 0);
      ageSends(rate);
      await resendVisible(fresh.page);
      const code = await codeFromMail(u.email);
      assert.equal(userRow(u.email).generation, generation + 1);
      await submitVisible(fresh.page, code);
      await waitUntil(() => callbacks.get(fresh.state) === 1, 'No authorization after the recovered resend');
      assert.ok(userRow(u.email).verified && userRow(u.email).consumed);
    } finally { await fresh.ctx.close(); }
  });

  await run('rotating the actual Keycloak code key invalidates version 1 and preserves recipient limits', async () => {
    const u = await register(browser);
    let fresh;
    try {
      const oldCode = await codeFromMail(u.email);
      const old = userRow(u.email);
      assert.equal(old.keyVersion, 1);
      const newKey = randomBytes(32).toString('base64');
      secrets.add(newKey);
      await restartKeycloak(['up', '-d', '--no-deps', '--no-build', '--force-recreate', 'keycloak'], {
        HITPIPE_EMAIL_CODE_KEY: newKey, HITPIPE_EMAIL_CODE_KEY_VERSION: '2',
      });
      fresh = await loginWithoutRegistering(browser, u);
      assert.equal(await fresh.page.locator(form).getAttribute('data-status'), 'cooldown');
      assert.equal(rateFor(u.rate).count, 1, 'The stable recipient ledger survived code key rotation');
      assert.equal(userRow(u.email).keyVersion, 1, 'No refreshed code bypassed the previous cooldown');
      await postNavigation(fresh.page, 'verify', oldCode, { generation: String(old.generation) });
      assert.ok(!userRow(u.email).consumed && !callbacks.has(fresh.state));
      ageSends(u.rate);
      await resendVisible(fresh.page);
      const currentCode = await codeFromMail(u.email, 2);
      const current = userRow(u.email);
      assert.equal(current.keyVersion, 2);
      assert.equal(current.generation, old.generation + 1);
      assert.equal(rateFor(u.rate).count, 2);
      await submitVisible(fresh.page, currentCode);
      await waitUntil(() => callbacks.get(fresh.state) === 1, 'No authorization after submitting the version 2 code');
      assert.ok(userRow(u.email).verified && userRow(u.email).consumed);
    } finally {
      if (fresh) await fresh.ctx.close();
      await u.ctx.close();
    }
  });

  await run('scheduled cleanup purges only expired challenge and recipient history', async () => {
    const old = await register(browser);
    const active = await register(browser);
    try {
      await codeFromMail(old.email);
      await codeFromMail(active.email);
      const account = userRow(old.email);
      sql(`UPDATE hitpipe_email_code_challenge
        SET issued_at = now() - interval '70 minutes', expires_at = now() - interval '65 minutes'
        WHERE user_id = ${literal(account.id)};`);
      editRate(old.rate, `ARRAY[now() - interval '61 minutes']`);
      await waitUntil(() => userRow(old.email)?.generation === null && !rateFor(old.rate),
        'The running maintenance scheduler did not purge the aged rows', 100_000);
      assert.ok(userRow(active.email)?.generation > 0 && rateFor(active.rate)?.count === 1,
        'Maintenance preserved an unexpired challenge and its recipient history');
    } finally { await active.ctx.close(); await old.ctx.close(); }
  });
} catch (error) {
  if (!process.exitCode) console.error(`FAIL lab initialization: ${sanitized(error)}`);
  process.exitCode = 1;
} finally {
  if (browser) await browser.close();
  callbackServer.close();
}
