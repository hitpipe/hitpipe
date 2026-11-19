import assert from 'node:assert/strict';
import { readFileSync, writeFileSync } from 'node:fs';

const [source, destination] = process.argv.slice(2);
const secret = process.env.HIT106_CLIENT_SECRET;
assert.ok(source && destination && /^[a-f0-9]{64}$/.test(secret ?? ''), 'Use run.sh with an ephemeral confidential client');

const realm = JSON.parse(readFileSync(source, 'utf8'));
assert.equal(realm.realm, 'hit105-test');
assert.ok(realm.requiredActions.some(a => a.providerId === 'hitpipe-email-code' && a.enabled && a.defaultAction));
assert.equal(realm.duplicateEmailsAllowed, false);
assert.equal(realm.verifyEmail, false);
assert.equal(realm.smtpServer.host, 'smtp');

realm.realm = 'hit106-test';
realm.displayName = 'HitPipe HTTPS OIDC test';
realm.sslRequired = 'all';
realm.smtpServer.from = 'no-reply@hit106.test';
realm.clients = [{
  clientId: 'hit106-iam',
  name: 'HIT-106 test IAM',
  enabled: true,
  protocol: 'openid-connect',
  clientAuthenticatorType: 'client-secret',
  secret,
  publicClient: false,
  bearerOnly: false,
  standardFlowEnabled: true,
  implicitFlowEnabled: false,
  directAccessGrantsEnabled: false,
  serviceAccountsEnabled: false,
  fullScopeAllowed: false,
  redirectUris: ['https://127.0.0.2:18444/auth/callback'],
  webOrigins: [],
  defaultClientScopes: ['profile', 'email'],
  attributes: { 'pkce.code.challenge.method': 'S256', 'id.token.signed.response.alg': 'RS256' },
}];

// The parent directory is inside a mode-0700 temporary directory on the guest.
// The read-only bind mount is only available to this Keycloak container.
writeFileSync(destination, `${JSON.stringify(realm, null, 2)}\n`, { flag: 'wx', mode: 0o444 });
