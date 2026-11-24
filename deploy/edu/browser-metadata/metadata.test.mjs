import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { X509Certificate } from 'node:crypto';
import { once } from 'node:events';
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { request } from 'node:https';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { startMetadata } from './metadata.mjs';

const ready = {
  workspaceId: '11111111-1111-4111-8111-111111111111',
  sourceConnectionId: '22222222-2222-4222-8222-222222222222',
  destinationConnectionId: 'dddddddd-dddd-4ddd-8ddd-dddddddddddd',
  sessionToken: 'synthetic_test_session',
  securityEpoch: 7,
  priceAlias: 'PRICE_ALIAS',
  listAlias: 'LIST_ALIAS',
  assigneeAlias: 'ASSIGNEE_ALIAS',
};
const providerDemo = {
  sourceAccount: 'acct_Test123',
  sourcePrice: 'price_Test456',
  destinationWorkspace: '123400',
  targetList: '123401',
  assignee: '123402',
  label: 'Демонстрационная услуга',
};
const checkedAt = '2026-11-24T18:00:00.000Z';

test('only exact, credential-free provider binding is accepted', async () => {
  for (const invalid of [
    null, [], { ...providerDemo, token: 'unaccepted-field' },
    { ...providerDemo, sourcePrice: '' }, { ...providerDemo, label: 'a\nb' },
    { ...providerDemo, targetList: 123401 }, { ...providerDemo, assignee: '0' },
  ]) {
    await assert.rejects(startMetadata({
      ready: { ...ready, providerDemo: invalid }, bffFingerprint: 'test',
    }), /missing its exact external provider bindings/);
  }
});

test('provider demo labels and local fixture labels use the same scoped TEST API', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'hitpipe-metadata-'));
  const servers = [];
  const file = name => join(dir, name);
  const openssl = (...args) => execFileSync('openssl', args, { cwd: dir, stdio: 'ignore' });
  try {
    openssl('req', '-x509', '-newkey', 'ec', '-pkeyopt', 'ec_paramgen_curve:P-256', '-nodes', '-days', '1',
      '-subj', '/CN=metadata tests', '-keyout', 'ca.key', '-out', 'ca.pem');
    async function leaf(name, usage) {
      openssl('req', '-newkey', 'ec', '-pkeyopt', 'ec_paramgen_curve:P-256', '-nodes', '-subj', '/CN=' + name,
        '-keyout', name + '.key', '-out', name + '.csr');
      const extensions = name + '.ext';
      await writeFile(file(extensions), 'subjectAltName=DNS:' + name + '\nextendedKeyUsage=' + usage + '\n');
      openssl('x509', '-req', '-in', name + '.csr', '-CA', 'ca.pem', '-CAkey', 'ca.key', '-CAcreateserial',
        '-days', '1', '-extfile', extensions, '-out', name + '.pem');
      return { cert: await readFile(file(name + '.pem')), key: await readFile(file(name + '.key')) };
    }
    const bff = await leaf('bff.test', 'clientAuth');
    const bffCA = await readFile(file('ca.pem'));
    let available = true;
    const common = {
      pki: { caPath: file('ca.pem'), leaf: name => leaf(name, 'serverAuth') },
      bffCA, bffFingerprint: new X509Certificate(bff.cert).fingerprint256,
      async listen(server) {
        servers.push(server);
        server.listen({ port: 0, host: '127.0.0.1' });
        await once(server, 'listening');
        return server.address().port;
      },
      async observe() {
        if (!available) throw new Error('provider unavailable');
        return checkedAt;
      },
    };
    const local = await startMetadata({ ...common, ready });
    const real = await startMetadata({ ...common, ready: { ...ready, providerDemo } });
    function call(owner, path, session = ready.sessionToken) {
      return new Promise((resolve, reject) => {
        const req = request(owner.origin + path, {
          ...bff, ca: bffCA, servername: owner.serverName, headers: { 'X-HitPipe-Session': session }, agent: false,
        }, async response => {
          const chunks = [];
          for await (const data of response) chunks.push(data);
          resolve({ status: response.statusCode, body: JSON.parse(Buffer.concat(chunks)) });
        });
        req.on('error', reject);
        req.end();
      });
    }
    const root = '/api/v1/workspaces/' + ready.workspaceId + '/connections';
    for (const [owners, labels] of [
      [local, ['Stripe · локальный тест', 'ClickUp · локальный тест', 'Тестовый тариф', 'Демонстрационный список', 'Тестовый исполнитель']],
      [real, ['Stripe Sandbox', 'ClickUp · HitPipe Demo', providerDemo.label, 'HitPipe · Demo Sandbox', 'Владелец рабочего списка']],
    ]) {
      for (const [owner, label] of [[owners.intake, labels[0]], [owners.delivery, labels[1]]]) {
        const connections = await call(owner, root);
        assert.equal(connections.status, 200);
        assert.equal(connections.body.items.length, 1);
        assert.equal(connections.body.items[0].mode, 'TEST');
        assert.equal(connections.body.items[0].account_label, label);
        assert.equal(connections.body.items[0].checked_at, checkedAt);
      }
      for (const [owner, connection, kind, alias, label] of [
        [owners.intake, ready.sourceConnectionId, 'PRICE', ready.priceAlias, labels[2]],
        [owners.delivery, ready.destinationConnectionId, 'LIST', ready.listAlias, labels[3]],
        [owners.delivery, ready.destinationConnectionId, 'ASSIGNEE', ready.assigneeAlias, labels[4]],
      ]) {
        const options = await call(owner, root + '/' + connection + '/options?kind=' + kind);
        assert.equal(options.status, 200);
        assert.deepEqual(options.body, {
          items: [{ kind, alias, label, available: true }], complete: true, next_cursor: null, observed_at: checkedAt,
        });
      }
    }
    assert.equal((await call(real.intake, root, 'wrong_session')).status, 403);
    assert.equal((await call(real.intake, root.replace(ready.workspaceId, ready.sourceConnectionId))).status, 404);
    available = false;
    const unavailable = await call(real.intake, root);
    assert.equal(unavailable.status, 503);
    assert.equal(unavailable.body.code, 'PROVIDER_UNAVAILABLE');
  } finally {
    await Promise.all(servers.map(server => new Promise(resolve => server.close(resolve))));
    await rm(dir, { recursive: true });
  }
});
