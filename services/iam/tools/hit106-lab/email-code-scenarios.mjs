import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { lstatSync } from 'node:fs';
import { basename, dirname, isAbsolute } from 'node:path';
import { fileURLToPath } from 'node:url';

const form = '#hitpipe-email-code-form';
const cooldownSeconds = 60;
const maxWrongAttempts = 5;
const maxSendsPerHour = 5;
const actionPath = '/realms/hit106-test/login-actions/required-action';
const actionOrigin = 'https://127.0.0.1:18443';
const fixtureEmail = /^[a-f0-9]{20}@hit106\.test$/;

const accountChallenge = `
  SELECT c.realm_id,c.user_id,c.issued_at,c.generation,c.failed_attempts,
         c.consumed_at,u.email_verified
  FROM hitpipe_email_code_challenge c
  JOIN user_entity u ON u.id = c.user_id AND u.realm_id = c.realm_id
  JOIN realm r ON r.id = c.realm_id
  WHERE r.name = 'hit106-test' AND u.email = :'fixture_email'
`;

function candidateRate(afterAging) {
  const last = afterAging ? "c.issued_at - interval '61 seconds'" : 'c.issued_at';
  // The provider writes the exact same timestamp to the challenge and to the
  // newest rate reservation. Require one match before updating any row.
  return `
    SELECT rates.realm_id, rates.recipient_mac, rates.send_times
    FROM hitpipe_email_code_rate rates JOIN challenge c ON c.realm_id = rates.realm_id
    WHERE rates.send_times[cardinality(rates.send_times)] = ${last}
      AND NOT c.email_verified AND c.consumed_at IS NULL
  `;
}

export function localEmailCodeFixture() {
  const directory = process.env.HIT106_LAB_DIR ?? '';
  assert.ok(process.env.HIT106_ISOLATED_LAB === '1' && isAbsolute(directory) &&
    /^hit106-lab\.[A-Za-z0-9]{8}$/.test(basename(directory)), 'Use the disposable lab');
  const info = lstatSync(directory);
  assert.ok(info.isDirectory() && (info.mode & 0o077) === 0, 'Use the lab private temporary directory');
  const compose = fileURLToPath(new URL('./compose.yaml', import.meta.url));

  function sql(email, statement) {
    assert.ok(fixtureEmail.test(email), 'SQL fixture is restricted to the generated guest email pattern');
    const child = spawnSync('docker', [
      '--host', 'unix:///var/run/docker.sock', 'compose', '--env-file', '/dev/null',
      '--project-name', 'hit106-browser-lab', '--project-directory', dirname(compose), '-f', compose,
      'exec', '-T', 'idp-db', 'psql', '-X', '-qAt', '-v', 'ON_ERROR_STOP=1',
      '-U', 'hit106_idp', '-d', 'hit106_keycloak',
    ], {
      input: `SET statement_timeout = '3s'; SET lock_timeout = '2s';\n\\set fixture_email ${email}\n${statement}\n`,
      encoding: 'utf8', timeout: 10_000, maxBuffer: 64 * 1024,
    });
    if (child.status !== 0) throw new Error('Disposable Keycloak SQL fixture did not complete');
    return child.stdout.trim();
  }

  function shiftRates(email, afterAging, arrayExpression) {
    const changed = sql(email, `
      WITH challenge AS (${accountChallenge}), candidate AS (${candidateRate(afterAging)}),
      changed AS (
        UPDATE hitpipe_email_code_rate rates SET send_times = (
          SELECT array_agg(${arrayExpression} ORDER BY send_order)
          FROM unnest(candidate.send_times) WITH ORDINALITY AS stamp(sent_at, send_order)
        )
        FROM candidate
        WHERE rates.realm_id = candidate.realm_id AND rates.recipient_mac = candidate.recipient_mac
          AND (SELECT count(*) FROM candidate) = 1
        RETURNING 1
      )
      SELECT count(*) FROM changed;
    `);
    assert.equal(changed, '1', 'Exactly one generated account rate record was moved');
  }

  return {
    read(email) {
      const row = sql(email, `
        WITH challenge AS (${accountChallenge})
        SELECT json_build_object('generation',generation,'attempts',failed_attempts,
          'verified',email_verified,'consumed',consumed_at IS NOT NULL) FROM challenge;
      `);
      return row ? JSON.parse(row) : null;
    },
    expire(email) {
      const changed = sql(email, `
        WITH challenge AS (${accountChallenge}), changed AS (
          UPDATE hitpipe_email_code_challenge c
          SET issued_at = statement_timestamp() - interval '11 minutes',
              expires_at = statement_timestamp() - interval '1 minute'
          FROM challenge mine
          WHERE c.realm_id = mine.realm_id AND c.user_id = mine.user_id
            AND NOT mine.email_verified AND mine.consumed_at IS NULL
            AND (SELECT count(*) FROM challenge) = 1
          RETURNING 1
        )
        SELECT count(*) FROM changed;
      `);
      assert.equal(changed, '1', 'Exactly one unconsumed generated challenge was expired');
    },
    ageCooldown(email) {
      shiftRates(email, false, "sent_at - interval '61 seconds'");
    },
    rateCount(email) {
      const rows = sql(email, `
        WITH challenge AS (${accountChallenge}),
        candidate AS (${candidateRate(false)} UNION ALL ${candidateRate(true)})
        SELECT cardinality(send_times) FROM candidate;
      `);
      assert.ok(/^[1-9][0-9]*$/.test(rows), 'Exactly one generated account rate record was found');
      return Number(rows);
    },
    moveOldestOutsideWindow(email) {
      shiftRates(email, true, "CASE WHEN send_order = 1 THEN statement_timestamp() - interval '61 minutes' ELSE sent_at END");
    },
  };
}

async function actionStatus(page, expected) {
  const challenge = page.locator(form);
  await challenge.waitFor();
  assert.ok(await challenge.getAttribute('data-status') === expected, `Email-code form reported ${expected}`);
}

async function transition(page, operation, code) {
  if (operation === 'verify') {
    await page.locator(`${form} input[name="code"]`).fill(code);
  }
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded' }),
    page.locator(`${form} button[name="operation"][value="${operation}"]`).click(),
  ]);
}

async function saveForm(page, privateURL) {
  const request = await page.locator(form).evaluate(element => ({
    url: element.action,
    fields: Object.fromEntries(new FormData(element)),
  }));
  const url = privateURL(request.url);
  assert.ok(url.origin === actionOrigin && url.pathname === actionPath,
    'Only a genuine HTTPS form of the disposable Keycloak realm may be submitted');
  assert.ok(/^[1-9][0-9]*$/.test(request.fields.generation), 'The required action bound the form to one issued generation');
  return request;
}

async function postOwnForm(context, request, code) {
  return context.request.post(request.url, {
    form: { ...request.fields, operation: 'verify', code },
    maxRedirects: 0,
    timeout: 15_000,
  });
}

function authorizationRedirect(response, callback, privateURL) {
  if (response.status() < 300 || response.status() >= 400) return null;
  const location = response.headers().location;
  if (!location) return null;
  const target = privateURL(location, actionOrigin);
  if (target.origin !== callback.origin || target.pathname !== callback.pathname || !target.searchParams.has('code')) {
    return null;
  }
  return target;
}

function expectMarkers(row, { generation, attempts, verified = false, consumed = false }) {
  assert.ok(row && row.generation === generation && row.attempts === attempts &&
    row.verified === verified && row.consumed === consumed,
  'Keycloak SQL generation, failed attempts and verification/consumption markers agree with the browser result');
}

// All action submissions go to the real HTTPS Keycloak in the guest. The SQL
// fixture only reads markers and moves the test account's expiry/cooldown
// timestamps so the inherited 10 minute / 60 second rules remain unchanged.
export async function runEmailCodeScenarios({
  browser, check, newBrowser, begin, beginRegister, identity, mailFor, mailCode,
  counts, expectSession, expectIssued, observeCallback, privateURL, callback,
  emailCodeFixture = localEmailCodeFixture(),
}) {
  const target = new URL(callback);
  assert.ok(target.origin === 'https://127.0.0.2:18444' && target.pathname === '/auth/callback',
    'The email-code scenario is restricted to the disposable HTTPS IAM server');

  async function withRegistration(test) {
    const u = await newBrowser(browser);
    const account = identity();
    assert.ok(fixtureEmail.test(account.email), 'Use only a freshly generated lab account');
    const initial = await counts();
    let callbackRequests = 0;
    u.page.on('request', request => {
      const url = new URL(request.url());
      if (url.origin === target.origin && url.pathname === target.pathname) callbackRequests++;
    });
    try {
      await begin(u.page);
      await beginRegister(u, account);
      await actionStatus(u.page, 'sent');
      const code = await mailCode(account.email);
      const start = await emailCodeFixture.read(account.email);
      assert.ok(start && Number.isSafeInteger(start.generation) && start.generation > 0,
        'Real mail and an issued SQL generation were established');
      expectMarkers(start, { generation: start.generation, attempts: 0 });
      await expectSession(u.context, false);
      assert.deepEqual(await counts(), initial);
      await test({ ...u, account, initial, code, generation: start.generation, callbackCount: () => callbackRequests });
    } finally {
      await u.context.close();
    }
  }

  async function expectNoSession(u, attempts) {
    expectMarkers(await emailCodeFixture.read(u.account.email), { generation: u.generation, attempts });
    assert.equal(u.callbackCount(), 0, 'The denied code never initiated an IAM callback');
    await expectSession(u.context, false);
    assert.deepEqual(await counts(), u.initial, 'The denied code created neither principal nor IAM session');
  }

  async function expectOneSession(u, response, generation, attempts) {
    await expectIssued(u, response);
    expectMarkers(await emailCodeFixture.read(u.account.email), { generation, attempts, verified: true, consumed: true });
    assert.equal(u.callbackCount(), 1, 'One successful code resulted in one browser callback');
    assert.deepEqual(await counts(), {
      principals: u.initial.principals + 1,
      sessions: u.initial.sessions + 1,
    }, 'The accepted code created one principal and one IAM session');
  }

  await check('HTTPS email code rejects wrong digits and locks after five attempts, including the genuine code', () =>
    withRegistration(async u => {
      const wrong = u.code === '00000000' ? '00000001' : '00000000';
      for (let attempts = 1; attempts <= maxWrongAttempts; attempts++) {
        await transition(u.page, 'verify', wrong);
        await actionStatus(u.page, attempts < maxWrongAttempts ? 'invalid' : 'locked');
        expectMarkers(await emailCodeFixture.read(u.account.email), { generation: u.generation, attempts });
      }
      await transition(u.page, 'verify', u.code);
      await actionStatus(u.page, 'locked');
      await expectNoSession(u, maxWrongAttempts);
    }));

  await check('HTTPS email code rejects the expired genuine mail without consuming it or signing in', () =>
    withRegistration(async u => {
      await emailCodeFixture.expire(u.account.email);
      await transition(u.page, 'verify', u.code);
      await actionStatus(u.page, 'expired');
      await expectNoSession(u, 0);
    }));

  await check('HTTPS email-code resend enforces cooldown, invalidates old digits and rejects reuse after verification', () =>
    withRegistration(async u => {
      await transition(u.page, 'resend');
      await actionStatus(u.page, 'cooldown');
      const retry = Number(await u.page.locator(form).getAttribute('data-retry-seconds'));
      assert.ok(Number.isInteger(retry) && retry > 0 && retry <= cooldownSeconds,
        'Immediate resend was refused with the provider cooldown still in effect');
      assert.equal((await mailFor(u.account.email)).length, 1, 'Cooldown emitted no additional SMTP message');
      assert.equal(await emailCodeFixture.rateCount(u.account.email), 1);
      await expectNoSession(u, 0);

      await emailCodeFixture.ageCooldown(u.account.email);
      await transition(u.page, 'resend');
      await actionStatus(u.page, 'sent');
      const replacement = await mailCode(u.account.email, 2);
      assert.ok(replacement !== u.code, 'A sent replacement has distinct digits');
      assert.equal(await emailCodeFixture.rateCount(u.account.email), 2);
      const nextGeneration = u.generation + 1;
      expectMarkers(await emailCodeFixture.read(u.account.email), { generation: nextGeneration, attempts: 0 });

      await transition(u.page, 'verify', u.code);
      await actionStatus(u.page, 'invalid');
      await expectNoSession({ ...u, generation: nextGeneration }, 1);

      const saved = await saveForm(u.page, privateURL);
      const finish = observeCallback(u.page);
      await transition(u.page, 'verify', replacement);
      await expectOneSession(u, await finish, nextGeneration, 1);
      const after = await counts();

      const reused = await postOwnForm(u.context, saved, replacement);
      try {
        assert.ok(!authorizationRedirect(reused, target, privateURL), 'A reused code produced no second authorization');
      } finally {
        await reused.dispose();
      }
      expectMarkers(await emailCodeFixture.read(u.account.email), {
        generation: nextGeneration, attempts: 1, verified: true, consumed: true,
      });
      await expectSession(u.context, true);
      assert.equal(u.callbackCount(), 1);
      assert.deepEqual(await counts(), after, 'Reusing the consumed code created no independent IAM session');
    }));

  await check('HTTPS email code enforces five sends in the rolling hour and accepts a send when the oldest leaves the window', () =>
    withRegistration(async u => {
      for (let sent = 2; sent <= maxSendsPerHour; sent++) {
        await emailCodeFixture.ageCooldown(u.account.email);
        await transition(u.page, 'resend');
        await actionStatus(u.page, 'sent');
        await mailCode(u.account.email, sent);
        assert.equal(await emailCodeFixture.rateCount(u.account.email), sent);
      }
      const generation = u.generation + maxSendsPerHour - 1;
      await emailCodeFixture.ageCooldown(u.account.email);
      await transition(u.page, 'resend');
      await actionStatus(u.page, 'rate_limited');
      assert.equal(await emailCodeFixture.rateCount(u.account.email), maxSendsPerHour);
      assert.equal((await mailFor(u.account.email)).length, maxSendsPerHour, 'The rate limit emitted no additional SMTP message');
      await expectNoSession({ ...u, generation }, 0);

      await emailCodeFixture.moveOldestOutsideWindow(u.account.email);
      await transition(u.page, 'resend');
      await actionStatus(u.page, 'sent');
      await mailCode(u.account.email, maxSendsPerHour + 1);
      assert.equal(await emailCodeFixture.rateCount(u.account.email), maxSendsPerHour);
      await expectNoSession({ ...u, generation: generation + 1 }, 0);
    }));

  await check('two concurrent HTTPS submissions of one genuine email code yield exactly one authorization and one IAM session', () =>
    withRegistration(async u => {
      const saved = await saveForm(u.page, privateURL);
      const responses = await Promise.all([
        postOwnForm(u.context, saved, u.code),
        postOwnForm(u.context, saved, u.code),
      ]);
      let winner;
      try {
        const issued = responses.map(response => authorizationRedirect(response, target, privateURL)).filter(Boolean);
        assert.equal(issued.length, 1, 'Exactly one competing Keycloak submission returned an authorization code');
        [winner] = issued;
      } finally {
        await Promise.all(responses.map(response => response.dispose()));
      }
      expectMarkers(await emailCodeFixture.read(u.account.email), {
        generation: u.generation, attempts: 0, verified: true, consumed: true,
      });
      await expectSession(u.context, false);
      assert.deepEqual(await counts(), u.initial, 'IAM state stays unchanged until the sole callback is used');

      const finish = observeCallback(u.page);
      await u.page.goto(winner.href, { waitUntil: 'domcontentloaded' });
      await expectOneSession(u, await finish, u.generation, 0);
    }));
}
