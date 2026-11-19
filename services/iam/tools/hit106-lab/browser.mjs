import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { once } from 'node:events';
import { join } from 'node:path';
import { setTimeout as pause } from 'node:timers/promises';
import { chromium } from 'playwright';

const app = 'https://127.0.0.2:18444';
const idp = 'https://127.0.0.1:18443';
const realm = 'hit106-test';
const issuer = `${idp}/realms/${realm}`;
const authorize = `${issuer}/protocol/openid-connect/auth`;
const callbackPath = '/auth/callback';
const callback = `${app}${callbackPath}`;
const complete = `${app}/__hit106/complete`;
const mailpit = 'http://127.0.0.1:18425';
const requiredForm = '#hitpipe-email-code-form';
const sessionName = '__Host-hp_session';
const labDir = process.env.HIT106_LAB_DIR;
assert.ok(labDir && process.env.NODE_EXTRA_CA_CERTS === join(labDir, 'ca.pem'), 'Use run.sh to set up verified local TLS');

const secrets = new Set(Object.entries(process.env)
  .filter(([name, value]) => /^HIT106_.*(?:SECRET|PASSWORD|_KEY)$/.test(name) && value.length > 8)
  .map(([, value]) => value));
const secretParams = /^(?:code|state|nonce|session_state|session_code|client_secret|code_verifier|code_challenge|tab_id|client_data)$/i;
const remember = value => { if (value) secrets.add(value); };

function privateURL(value, base) {
  const url = new URL(value, base);
  for (const [name, argument] of url.searchParams) if (secretParams.test(name)) remember(argument);
  return url;
}

function safeError(error) {
  let msg = error instanceof Error ? error.message : String(error);
  for (const value of secrets) {
    msg = msg.replaceAll(value, '[redacted]').replaceAll(encodeURIComponent(value), '[redacted]');
  }
  return msg.replace(/\b\d{8}\b/g, '[redacted]')
    .replace(/https?:\/\/[^\s<>'"\])]+/g, '[url]')
    .replace(/\b(?:authorization|cookie|set-cookie)\s*:\s*[^\n]+/gi, '[redacted header]');
}

async function check(title, test) {
  try {
    await test();
    console.log(`PASS ${title}`);
  } catch (error) {
    console.error(`FAIL ${title}: ${safeError(error)}`);
    process.exitCode = 1;
    throw new Error('Browser scenario failed');
  }
}

async function waitUntil(read, description, timeout = 15_000) {
  const deadline = Date.now() + timeout;
  do {
    const value = await read();
    if (value) return value;
    await pause(150);
  } while (Date.now() < deadline);
  throw new Error(description);
}

async function localFetch(url, options) {
  return fetch(url, { signal: AbortSignal.timeout(6000), redirect: 'error', ...options });
}

async function counts() {
  const r = await localFetch(`${app}/__hit106/count`);
  assert.equal(r.status, 200, 'The private test count endpoint answered');
  const result = await r.json();
  assert.ok(Number.isSafeInteger(result.principals) && Number.isSafeInteger(result.sessions));
  return result;
}

async function expectCounts(principals, sessions) {
  assert.deepEqual(await counts(), { principals, sessions });
}

let iam;
async function startIAM() {
  iam = spawn(join(labDir, 'iam-browser'), ['serve'], { stdio: ['ignore', 'inherit', 'inherit'] });
  const child = iam;
  let spawnError;
  child.once('error', error => { spawnError = error; });
  await waitUntil(async () => {
    if (spawnError) throw spawnError;
    if (child.exitCode !== null || child.signalCode !== null) throw new Error('The private IAM server exited during startup');
    try { return (await localFetch(`${app}/__hit106/health`)).status === 204; } catch { return false; }
  }, 'The private HTTPS IAM server did not become healthy with the trusted test CA');
}

async function stopIAM() {
  const child = iam;
  iam = undefined;
  if (!child || child.exitCode !== null || child.signalCode !== null || !child.pid) return;
  const stopped = once(child, 'exit');
  child.kill('SIGTERM');
  if (!await Promise.race([stopped.then(() => true), pause(5000, false, { ref: false })])) {
    child.kill('SIGKILL');
    await stopped;
  }
}

for (const [signal, status] of [['SIGINT', 130], ['SIGTERM', 143], ['SIGHUP', 129]]) {
  process.once(signal, () => { stopIAM().finally(() => process.exit(status)); });
}

async function newBrowser(browser) {
  const context = await browser.newContext({ ignoreHTTPSErrors: false, serviceWorkers: 'block' });
  context.on('request', request => { privateURL(request.url()); });
  return { context, page: await context.newPage() };
}

function rememberCookie(value) {
  const [first] = value.split(';');
  const delimiter = first.indexOf('=');
  if (delimiter >= 0) remember(first.slice(delimiter + 1).trim());
}

async function responseCookies(response) {
  // A browser Response provides headerValues; APIResponse provides headersArray.
  const cookies = typeof response.headerValues === 'function'
    ? await response.headerValues('set-cookie')
    : response.headersArray().filter(h => h.name.toLowerCase() === 'set-cookie').map(h => h.value);
  cookies.forEach(rememberCookie);
  return cookies;
}

async function expectDenied(response, status) {
  const actual = response.status();
  if (status) assert.equal(actual, status, 'The requested IAM guard returned its documented status');
  else assert.ok((actual >= 400 && actual < 500) || actual === 503, `IAM denied attempt with HTTP ${actual}`);
  assert.ok(!(await responseCookies(response)).some(value => value.startsWith(`${sessionName}=`)),
    'Denied attempt issued no HitPipe session cookie');
}

async function expectSession(ctx, active) {
  const result = await ctx.request.get(`${app}/__hit106/session`, { maxRedirects: 0 });
  assert.equal(result.status(), active ? 204 : 401, 'Existing AuthenticateSession and SessionLookup yield expected verdict');
}

async function expectIssued(u, response) {
  assert.equal(response.status(), 303, 'IAM callback returns the approved redirect status');
  const location = await response.headerValue('location');
  assert.ok(location && privateURL(location, app).href === complete, 'IAM redirected only to the fixed completion URL');
  await u.page.waitForURL(complete);
  const declared = (await responseCookies(response)).filter(value => value.startsWith(`${sessionName}=`));
  assert.equal(declared.length, 1, 'Exactly one HitPipe session cookie was set');
  const attrs = Object.fromEntries(declared[0].split(';').slice(1).map(field => {
    const [key, value = ''] = field.trim().split('=');
    return [key.toLowerCase(), value];
  }));
  assert.ok('secure' in attrs && 'httponly' in attrs && !('domain' in attrs));
  assert.equal(attrs.path, '/');
  assert.equal(attrs.samesite?.toLowerCase(), 'lax');
  const cookies = (await u.context.cookies(app)).filter(value => value.name === sessionName);
  cookies.forEach(cookie => remember(cookie.value));
  assert.equal(cookies.length, 1);
  const deadline = cookies[0].expires;
  const now = Date.now() / 1000;
  assert.ok(deadline > now && deadline <= now + 24 * 3600 + 5, 'Browser lifetime follows the 24h policy bound');
  assert.deepEqual(await u.page.evaluate(() => [localStorage.length, sessionStorage.length]), [0, 0],
    'IAM browser storage is free of token material');
  await expectSession(u.context, true);
}

async function begin(page) {
  const response = page.waitForResponse(r => r.url() === `${app}/auth/login` && r.request().method() === 'GET');
  const idpRequest = page.waitForRequest(r => r.url().startsWith(`${authorize}?`));
  await page.goto(`${app}/auth/login`, { waitUntil: 'domcontentloaded' });
  const started = await response;
  assert.equal(started.status(), 302, 'GET /auth/login starts the code flow');
  await responseCookies(started);
  const location = await started.headerValue('location');
  assert.ok(location, 'Login supplied an IdP redirect');
  const destination = privateURL(location, app);
  assert.ok(`${destination.origin}${destination.pathname}` === authorize, 'Login redirects to the approved issuer');
  const query = destination.searchParams;
  assert.equal(query.get('client_id'), 'hit106-iam');
  assert.equal(query.get('response_type'), 'code');
  assert.equal(query.get('redirect_uri'), callback);
  assert.equal(query.get('code_challenge_method'), 'S256');
  assert.ok(!query.has('code_verifier') && !query.has('client_secret'), 'The browser sees neither client credential nor verifier');
  const state = query.get('state');
  const nonce = query.get('nonce');
  const challenge = query.get('code_challenge');
  assert.ok(state && nonce && state !== nonce && /^[A-Za-z0-9_-]{43}$/.test(challenge ?? ''),
    'Login supplied independent state and nonce plus an S256 challenge');
  const cookies = (await page.context().cookies(app)).filter(cookie => cookie.name.startsWith('__Host-hp_oidc_'));
  assert.equal(cookies.length, 1, 'The browser retained exactly one host-only attempt binding');
  const [binding] = cookies;
  remember(binding.value);
  assert.ok(binding.httpOnly && binding.secure && binding.sameSite === 'Lax' && binding.domain === '127.0.0.2');
  const outbound = await (await idpRequest).headerValue('cookie');
  assert.ok(!outbound?.includes('__Host-hp_'), 'IAM cookies do not travel to the IdP');
  return { state, nonce, challenge, binding };
}

function observeCallback(page) {
  return page.waitForResponse(r => {
    const url = privateURL(r.url());
    return url.origin === app && url.pathname === callbackPath;
  });
}

async function browserLogin(page, identity) {
  await page.locator('input[name="username"]').fill(identity.email);
  await page.locator('input[name="password"]').fill(identity.password);
  await page.locator('#kc-form-login [type="submit"]').click();
}

function identity() {
  const email = `${randomBytes(10).toString('hex')}@hit106.test`;
  const password = `Lab67!${randomBytes(24).toString('base64url')}`;
  remember(email);
  remember(password);
  return { email, password };
}

async function mailFor(email) {
  const r = await localFetch(`${mailpit}/api/v1/messages?limit=30`);
  assert.equal(r.status, 200, 'Mailpit returned the local SMTP spool');
  const { messages } = await r.json();
  return messages.filter(message => message.To.some(to => to.Address.toLowerCase() === email));
}

async function mailCode(email) {
  const message = await waitUntil(async () => {
    const messages = await mailFor(email);
    return messages.length === 1 && messages[0];
  }, 'No genuine email-code arrived in the local SMTP receiver');
  const result = await localFetch(`${mailpit}/api/v1/message/${encodeURIComponent(message.ID)}`);
  assert.equal(result.status, 200, 'Mailpit opened captured local email');
  const payload = await result.json();
  const codes = payload.Text?.match(/(?<!\d)[0-9]{8}(?!\d)/g) ?? [];
  assert.equal(codes.length, 1, 'The genuine email contains one 8-digit code');
  remember(codes[0]);
  assert.ok(!/\/login-actions\/action-token\b/.test(payload.Text), 'Email uses the required action code');
  return codes[0];
}

async function beginRegister(u, account) {
  await u.page.locator('a[href*="registrations"], a[href*="registration"]').first().click();
  const fill = async (name, value) => {
    const field = u.page.locator(`input[name="${name}"]`);
    if (await field.isVisible()) await field.fill(value);
  };
  await fill('firstName', 'HitPipe');
  await fill('lastName', 'Test');
  await fill('username', account.email);
  await u.page.locator('input[name="email"]').fill(account.email);
  await u.page.locator('input[name="password"]').fill(account.password);
  await fill('password-confirm', account.password);
  await u.page.locator('#kc-register-form [type="submit"]').click();
  await u.page.locator(requiredForm).waitFor();
}

async function keycloakAdmin() {
  const r = await localFetch(`${idp}/realms/master/protocol/openid-connect/token`, {
    method: 'POST',
    body: new URLSearchParams({ grant_type: 'password', client_id: 'admin-cli', username: 'hit106-local-admin',
      password: process.env.HIT106_IDP_ADMIN_PASSWORD }),
  });
  assert.equal(r.status, 200, 'Fresh lab administrator authenticated over verified HTTPS');
  const { access_token: token } = await r.json();
  remember(token);
  assert.ok(token, 'The temporary realm administrator token is available');
  return (path, method = 'GET', data) => localFetch(`${idp}/admin/realms/${realm}/${path}`, {
    method, headers: { authorization: `Bearer ${token}`, 'content-type': 'application/json' },
    body: data === undefined ? undefined : JSON.stringify(data),
  });
}

let browser;
try {
  await startIAM();
  browser = await chromium.launch({ headless: true });
  console.log(`Chromium ${browser.version()} (verified local CA; HTTPS errors enforced)`);
  const account = identity();
  let firstAttempt;
  let issuedContext;
  let replay;

  await check('HTTPS registration requires email-code, then issues one real IAM session', async () => {
    const u = await newBrowser(browser);
    issuedContext = u;
    firstAttempt = await begin(u.page);
    await beginRegister(u, account);
    await expectCounts(0, 0);
    await expectSession(u.context, false);
    const code = await mailCode(account.email);
    const input = u.page.locator(`${requiredForm} input[name="code"]`);
    await input.focus();
    await u.page.keyboard.type(code);
    const finish = observeCallback(u.page);
    await u.page.keyboard.press('Enter');
    const response = await finish;
    replay = privateURL(response.url()).href;
    await expectIssued(u, response);
    await expectCounts(1, 1);
  });

  await check('concurrent replay of the completed callback creates no independent sessions', async () => {
    const retries = await Promise.all([newBrowser(browser), newBrowser(browser)]);
    try {
      await Promise.all(retries.map(u => u.context.addCookies([firstAttempt.binding])));
      const responses = await Promise.all(retries.map(u => u.context.request.get(replay, { maxRedirects: 0 })));
      for (const response of responses) await expectDenied(response, 400);
      for (const u of retries) await expectSession(u.context, false);
      await expectCounts(1, 1);
    } finally { await Promise.all(retries.map(u => u.context.close())); }
  });

  await check('an existing session cannot be replaced without the revocation coordinator', async () => {
    try {
      const response = await issuedContext.context.request.get(`${app}/auth/login`, { maxRedirects: 0 });
      await expectDenied(response, 409);
      await expectSession(issuedContext.context, true);
      await expectCounts(1, 1);
    } finally { await issuedContext.context.close(); }
  });

  await check('the confirmed user signs in again with a fresh attempt and the same principal', async () => {
    const u = await newBrowser(browser);
    try {
      const attempt = await begin(u.page);
      assert.ok(attempt.state !== firstAttempt.state && attempt.nonce !== firstAttempt.nonce
        && attempt.challenge !== firstAttempt.challenge, 'Independent browser attempts generated fresh inputs');
      const finish = observeCallback(u.page);
      await browserLogin(u.page, account);
      const response = await finish;
      await expectIssued(u, response);
      assert.equal((await mailFor(account.email)).length, 1, 'Confirmed login required no second email');
      await expectCounts(1, 2);
    } finally { await u.context.close(); }
  });

  await check('foreign or missing state and browser authority assertions do not create a session', async () => {
    const u = await newBrowser(browser);
    try {
      const unknown = randomBytes(32).toString('base64url');
      remember(unknown);
      const requests = await Promise.all([
        u.context.request.get(callback, { maxRedirects: 0 }),
        u.context.request.get(callback, { maxRedirects: 0, params: { code: 'invalid-fixture-code', state: unknown },
          headers: { 'x-hitpipe-user': 'fixture', 'x-hitpipe-role': 'admin' } }),
      ]);
      for (const response of requests) await expectDenied(response, 400);
      await expectSession(u.context, false);
      await expectCounts(1, 2);
    } finally { await u.context.close(); }
  });

  await check('cancelling an attempt also blocks a later genuine IdP callback for that state', async () => {
    const u = await newBrowser(browser);
    try {
      const { state, binding } = await begin(u.page);
      const denied = await u.context.request.get(callback, { maxRedirects: 0, params: { state, error: 'access_denied' } });
      await expectDenied(denied, 400);
      await u.context.addCookies([binding]);
      const finish = observeCallback(u.page);
      await browserLogin(u.page, account);
      const response = await finish;
      assert.ok(privateURL(response.url()).searchParams.has('code'), 'Actual IdP produced a code after cancellation');
      await expectDenied(response, 400);
      await expectSession(u.context, false);
      await expectCounts(1, 2);
    } finally { await u.context.close(); }
  });

  await check('a genuine callback after IAM restart fails with the lost one-use attempt', async () => {
    const u = await newBrowser(browser);
    try {
      await begin(u.page);
      await stopIAM();
      await startIAM();
      const finish = observeCallback(u.page);
      await browserLogin(u.page, account);
      const response = await finish;
      assert.ok(privateURL(response.url()).searchParams.has('code'), 'Actual IdP code reached the restarted IAM');
      await expectDenied(response, 400);
      await expectSession(u.context, false);
      await expectCounts(1, 2);
    } finally { await u.context.close(); }
  });

  await check('a genuine unverified-email claim from the test IdP cannot create a session', async () => {
    const admin = await keycloakAdmin();
    const actionPath = 'authentication/required-actions/hitpipe-email-code';
    const current = await admin(actionPath);
    assert.equal(current.status, 200);
    const action = await current.json();
    assert.ok(action.enabled && action.defaultAction, 'The positive realm tested the enabled required action');
    // Change only this disposable realm for the explicit negative fixture, so
    // IAM receives a genuine signed Keycloak token with email_verified=false.
    const changed = await admin(actionPath, 'PUT', { ...action, enabled: false, defaultAction: false });
    assert.equal(changed.status, 204);
    const u = await newBrowser(browser);
    try {
      const fixture = identity();
      const created = await admin('users', 'POST', {
        enabled: true, emailVerified: false, email: fixture.email, username: fixture.email,
        firstName: 'Negative', lastName: 'Fixture',
        credentials: [{ type: 'password', temporary: false, value: fixture.password }],
      });
      assert.equal(created.status, 201, 'The temporary IdP account has explicitly unverified email');
      await begin(u.page);
      const finish = observeCallback(u.page);
      await browserLogin(u.page, fixture);
      const response = await finish;
      assert.ok(privateURL(response.url()).searchParams.has('code'), 'Real Keycloak returned a code for the controlled fixture');
      await expectDenied(response, 401);
      await expectSession(u.context, false);
      await expectCounts(1, 2);
    } finally {
      await u.context.close();
      const restoreAdmin = await keycloakAdmin();
      assert.equal((await restoreAdmin(actionPath, 'PUT', action)).status, 204, 'Required action restored in the temporary realm');
    }
  });
} catch (error) {
  if (!process.exitCode) console.error(`FAIL private browser stand: ${safeError(error)}`);
  process.exitCode = 1;
} finally {
  if (browser) await browser.close();
  await stopIAM();
}
