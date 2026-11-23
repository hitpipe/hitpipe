import { randomUUID } from 'node:crypto';
import { createServer } from 'node:https';

const uuid = /^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/;
const alias = /^[A-Z][A-Z0-9_]{0,31}$/;

function send(res, status, value) {
  res.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' });
  res.end(JSON.stringify(value));
}

function fail(res, status, code) {
  send(res, status, { code, message: code, request_id: randomUUID() });
}

function buildHandler(ready, owner, bffFingerprint, observe) {
  const root = '/api/v1/workspaces/' + ready.workspaceId + '/connections';
  const connection = {
    connection_id: owner.connectionId,
    provider: owner.provider,
    mode: 'TEST',
    account_label: owner.label,
    health: 'READY',
    revision: 1,
    security_epoch: ready.securityEpoch,
  };

  return async (req, res) => {
    // This test-only adapter has no write or status routes. The provisioned
    // client certificate and session belong to this one fixture workspace.
    if (req.socket.getPeerCertificate().fingerprint256 !== bffFingerprint
        || req.headersDistinct['x-hitpipe-session']?.length !== 1
        || req.headersDistinct['x-hitpipe-session'][0] !== ready.sessionToken) {
      return fail(res, 403, 'FORBIDDEN');
    }
    if (req.method !== 'GET') return fail(res, 404, 'NOT_FOUND');

    if (!req.url?.startsWith('/') || req.url.startsWith('//')) return fail(res, 400, 'INVALID_REQUEST');
    const url = new URL(req.url, 'https://' + owner.serverName);
    if (url.pathname === root && !url.search) {
      return send(res, 200, { items: [{ ...connection, checked_at: await observe() }] });
    }
    if (url.pathname !== root + '/' + owner.connectionId + '/options') return fail(res, 404, 'NOT_FOUND');

    const keys = [...url.searchParams.keys()];
    const kind = url.searchParams.get('kind');
    if (keys.length > 1 || keys.some(key => key !== 'kind')
        || (kind !== null && !Object.hasOwn(owner.options, kind))) return fail(res, 400, 'INVALID_REQUEST');
    const kinds = kind === null ? Object.keys(owner.options) : [kind];
    const items = kinds.map(key => ({ ...owner.options[key], kind: key, available: true }));
    return send(res, 200, { items, complete: true, next_cursor: null, observed_at: await observe() });
  };
}

// Serve connection metadata for the same local TEST fixtures as the Go
// processes. Processing itself handles activation and every case read.
export async function startMetadata({ ready, pki, bffCA, bffFingerprint, listen, observe }) {
  if (![ready.workspaceId, ready.sourceConnectionId, ready.destinationConnectionId]
    .every(value => typeof value === 'string' && uuid.test(value))
    || ready.sourceConnectionId === ready.destinationConnectionId
    || ![ready.priceAlias, ready.listAlias, ready.assigneeAlias]
      .every(value => typeof value === 'string' && alias.test(value))
    || !Number.isSafeInteger(ready.securityEpoch) || ready.securityEpoch < 1
    || !ready.sessionToken || !bffFingerprint) {
    throw new Error('Browser fixture is missing its exact connection bindings');
  }

  const owners = {
    intake: {
      provider: 'STRIPE',
      label: 'Stripe · локальный тест',
      serverName: 'intake.demo.test',
      connectionId: ready.sourceConnectionId,
      options: { PRICE: { alias: ready.priceAlias, label: 'Тестовый тариф' } },
    },
    delivery: {
      provider: 'CLICKUP',
      label: 'ClickUp · локальный тест',
      serverName: 'delivery.demo.test',
      connectionId: ready.destinationConnectionId,
      options: {
        LIST: { alias: ready.listAlias, label: 'Демонстрационный список' },
        ASSIGNEE: { alias: ready.assigneeAlias, label: 'Тестовый исполнитель' },
      },
    },
  };
  return Object.fromEntries(await Promise.all(Object.entries(owners).map(async ([name, owner]) => {
    const handler = buildHandler(ready, owner, bffFingerprint, observe);
    const server = createServer({
      ...await pki.leaf(owner.serverName), ca: bffCA, requestCert: true, rejectUnauthorized: true,
      minVersion: 'TLSv1.3', maxVersion: 'TLSv1.3', requestTimeout: 3500, headersTimeout: 3000,
    }, (req, res) => {
      void handler(req, res).catch(() => {
        if (!res.headersSent) fail(res, 503, 'LOCAL_PROVIDER_UNAVAILABLE');
      });
    });
    const port = await listen(server);
    return [name, { origin: 'https://127.0.0.1:' + port, serverName: owner.serverName, ca: pki.caPath }];
  })));
}
