import assert from 'node:assert/strict';
import { createHash, randomBytes } from 'node:crypto';

const app = 'https://127.0.0.2:18444';
const issuer = 'https://127.0.0.1:18443/realms/hit106-test';
const digest = value => createHash('sha256').update(value).digest('hex');

// All accounts and credentials exist only in the disposable Keycloak realm.
// The IAM callback, OIDC verifier, issuer and PostgreSQL adapter remain genuine.
export async function sameEmailScenario({
  browser, check, begin, observeCallback, expectIssued, expectSession,
  keycloakAdmin, newBrowser, counts, remember,
}) {
  await check('two Keycloak subjects with the same verified email remain distinct HitPipe principals', async () => {
    const admin = await keycloakAdmin();
    const settingsResponse = await admin('');
    assert.equal(settingsResponse.status, 200, 'The temporary realm configuration is available');
    const original = await settingsResponse.json();
    assert.ok(original.realm === 'hit106-test' && original.duplicateEmailsAllowed === false,
      'Only the disposable realm may be changed for the duplicate-email fixture');
    const fixtureFlags = { duplicateEmailsAllowed: true, registrationEmailAsUsername: false, loginWithEmailAllowed: false };
    const changed = await admin('', 'PUT', fixtureFlags);
    assert.equal(changed.status, 204, 'The temporary realm accepts the test-only duplicate-email policy');
    const fixture = await admin('');
    assert.equal(fixture.status, 200);
    const settings = await fixture.json();
    assert.ok(settings.duplicateEmailsAllowed === true && settings.registrationEmailAsUsername === false
      && settings.loginWithEmailAllowed === false, 'The isolated fixture permits duplicate emails and unambiguous username login');
    const actionResponse = await admin('authentication/required-actions/hitpipe-email-code');
    assert.equal(actionResponse.status, 200);
    const action = await actionResponse.json();
    assert.ok(action.enabled && action.defaultAction, 'The real email-code provider remains enabled');

    const email = `${randomBytes(16).toString('hex')}@hit106.test`;
    remember(email);
    const accounts = [];
    for (let index = 0; index < 2; index++) {
      const account = {
        username: `same-email-${randomBytes(16).toString('hex')}`,
        password: `Lab67!${randomBytes(24).toString('base64url')}`,
      };
      remember(account.username);
      remember(account.password);
      const created = await admin('users', 'POST', {
        username: account.username, email, emailVerified: true, enabled: true,
        firstName: 'Same', lastName: 'Fixture', requiredActions: [],
        credentials: [{ type: 'password', temporary: false, value: account.password }],
      });
      assert.equal(created.status, 201, 'Real Keycloak accepted the distinct fixture subject');
      const location = created.headers.get('location');
      assert.ok(location, 'Keycloak returned the created subject location');
      const url = new URL(location);
      const prefix = '/admin/realms/hit106-test/users/';
      assert.ok(url.origin === new URL(issuer).origin && url.pathname.startsWith(prefix),
        'Subject location belongs only to the disposable realm');
      account.subject = url.pathname.slice(prefix.length);
      remember(account.subject);
      assert.ok(/^[a-f0-9-]{36}$/.test(account.subject), 'Keycloak generated an opaque subject identifier');
      const recordResponse = await admin(`users/${account.subject}`);
      assert.equal(recordResponse.status, 200);
      const record = await recordResponse.json();
      assert.ok(record.id === account.subject && record.username === account.username
        && record.email === email && record.emailVerified === true && record.enabled === true,
      'The actual Keycloak records have the same verified email and distinct usernames');
      accounts.push(account);
    }
    assert.ok(accounts[0].subject !== accounts[1].subject, 'The fixture has two genuinely different Keycloak subjects');

    const before = await counts();
    async function loginAndRead(account) {
      const user = await newBrowser(browser);
      try {
        await begin(user.page);
        await user.page.locator('input[name="username"]').fill(account.username);
        await user.page.locator('input[name="password"]').fill(account.password);
        const finished = observeCallback(user.page);
        await user.page.locator('#kc-form-login [type="submit"]').click();
        await expectIssued(user, await finished);
        await expectSession(user.context, true);
        const proofResponse = await user.context.request.get(`${app}/__hit106/identity`, { maxRedirects: 0 });
        assert.equal(proofResponse.status(), 200, 'The authenticated session has a real PostgreSQL identity proof');
        const proof = await proofResponse.json();
        assert.ok(Object.keys(proof).sort().join(',') === 'issuer_digest,principal_digest,subject_digest'
          && Object.values(proof).every(value => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value)),
        'The test endpoint returns only bounded identity digests');
        assert.ok(proof.issuer_digest === digest(issuer) && proof.subject_digest === digest(account.subject),
          'The persisted session belongs to the actual local issuer and expected Keycloak subject');
        return proof;
      } finally {
        await user.context.close();
      }
    }

    const first = await loginAndRead(accounts[0]);
    const second = await loginAndRead(accounts[1]);
    assert.ok(first.principal_digest !== second.principal_digest && first.subject_digest !== second.subject_digest,
      'Equal verified email did not link the two distinct subject identities or HitPipe accounts');
    assert.deepEqual(await counts(), { principals: before.principals + 2, sessions: before.sessions + 2 },
      'Real browser callbacks committed two separate principals and sessions');
    const firstAgain = await loginAndRead(accounts[0]);
    assert.ok(firstAgain.principal_digest === first.principal_digest && firstAgain.subject_digest === first.subject_digest,
      'A later real login reuses only its own issuer-subject principal');
    assert.deepEqual(await counts(), { principals: before.principals + 2, sessions: before.sessions + 3 },
      'Repeated login adds a session without merging or duplicating either principal');
  });
}
