import { spawn, execFile, execFileSync } from 'node:child_process';
import { randomUUID, X509Certificate } from 'node:crypto';
import { once } from 'node:events';
import { createServer as httpServer, request as httpRequest } from 'node:http';
import { createServer as httpsServer } from 'node:https';
import { createConnection, createServer as tcpServer } from 'node:net';
import { mkdir, mkdtemp, readFile, rename, rm, writeFile } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { setTimeout as sleep } from 'node:timers/promises';
import { promisify } from 'node:util';

process.umask(0o077);
const host = '127.0.0.1';
const input = process.argv.slice(2);
if (input.length !== 3 || input.some(value => !value)) {
  throw new Error('Usage: host.mjs SHARE FRONTEND ARTIFACT_DIR');
}
const [share, frontend, report] = input.map(path => resolve(path));
const webPort = Number(process.env.HITPIPE_BROWSER_PORT ?? '3108');
const entryPort = Number(process.env.HITPIPE_SCHOOL_ENTRY_PORT ?? '3109');
if (![webPort, entryPort].every(port => Number.isInteger(port) && port > 1023 && port <= 65535)
    || new Set([webPort, entryPort, 8443, 9443]).size !== 4) {
  throw new Error('Distinct unprivileged ports are required for browser and entry');
}

const servers = [];
const children = new Set();
let interrupted = false;
let hostPkiDirectory;
const stop = () => { interrupted = true; for (const child of children) child.kill('SIGTERM'); };
process.once('SIGTERM', stop);
process.once('SIGINT', stop);
const json = path => readFile(path, 'utf8').then(value => JSON.parse(value));

function start(program, args, cwd, extraEnv = {}) {
  const child = spawn(program, args, { cwd, env: { ...process.env, ...extraEnv }, stdio: 'inherit' });
  const done = new Promise((resolve, reject) => {
    child.once('error', reject);
    child.once('exit', (code, signal) => resolve({ code, signal }));
  }).finally(() => children.delete(child));
  // The owner can await this from a polling loop or attach a timeout-wrapped child.
  done.catch(() => {});
  children.add(child);
  return { child, done };
}

async function listen(server, port = 0) {
  const peers = new Set();
  server.on('connection', socket => {
    peers.add(socket);
    socket.on('close', () => peers.delete(socket));
  });
  try {
    server.listen({ host, port });
    await once(server, 'listening');
  } catch (error) {
    for (const peer of peers) peer.destroy();
    throw error;
  }
  servers.push(async () => {
    const closed = once(server, 'close');
    server.close();
    for (const peer of peers) peer.destroy();
    await closed;
  });
  return server.address().port;
}

const tlsFingerprint = pem => new X509Certificate(pem).fingerprint256;
const prefixed = text => 'sha256:' + text.toLowerCase().replaceAll(':', '');

function openssl(cwd, ...args) {
  execFileSync('openssl', args, { cwd, stdio: 'ignore', timeout: 15000 });
}

async function hostTLS(dir) {
  const file = name => join(dir, name);
  openssl(dir, 'req', '-x509', '-newkey', 'ec', '-pkeyopt', 'ec_paramgen_curve:P-256',
    '-nodes', '-days', '1', '-subj', '/CN=HitPipe browser stand host',
    '-keyout', 'host-ca.key', '-out', 'host-ca.pem');
  for (const [name, usage] of [
    ['iam.demo.test', 'serverAuth'], ['bff-control.demo.test', 'serverAuth'],
    ['iam-control.demo.test', 'clientAuth'], ['intake.demo.test', 'serverAuth'], ['delivery.demo.test', 'serverAuth'],
  ]) {
    openssl(dir, 'req', '-newkey', 'ec', '-pkeyopt', 'ec_paramgen_curve:P-256', '-nodes',
      '-subj', '/CN=' + name, '-keyout', name + '.key', '-out', name + '.csr');
    await writeFile(file('leaf.ext'), 'subjectAltName=DNS:' + name + '\nextendedKeyUsage=' + usage + '\n');
    openssl(dir, 'x509', '-req', '-in', name + '.csr', '-CA', 'host-ca.pem', '-CAkey', 'host-ca.key',
      '-CAcreateserial', '-days', '1', '-extfile', 'leaf.ext', '-out', name + '.pem');
  }
  return {
    caPath: file('host-ca.pem'),
    certPath: name => file(name + '.pem'),
    keyPath: name => file(name + '.key'),
    async leaf(name) {
      return { cert: await readFile(file(name + '.pem')), key: await readFile(file(name + '.key')) };
    },
  };
}

function send(res, status, body) {
  res.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' });
  res.end(JSON.stringify(body));
}
const failure = code => ({ code, message: code, request_id: randomUUID() });

async function requestBody(req) {
  const chunks = [];
  let size = 0;
  for await (const chunk of req) {
    size += chunk.length;
    if (size > 4096) throw new Error('Oversized stand request');
    chunks.push(chunk);
  }
  return JSON.parse(Buffer.concat(chunks).toString('utf8'));
}

async function startIAM(ready, hostPki, bffCA, bffFingerprint, runtimeId) {
  const allowedActions = new Set(['CASE_READ', 'CONFIG_READ', 'CONFIG_WRITE', 'ACTIVATE',
    'CONNECTOR_READ', 'PAUSE', 'RESUME', 'RECHECK']);
  const region = {
    workspace_id: ready.workspaceId, name: 'HitPipe · демонстрация', lifecycle: 'ACTIVE',
    role: 'OWNER', revision: 1, security_epoch: ready.securityEpoch,
  };
  const profile = {
    user_id: ready.principalId, display_name: 'Оператор демонстрации', timezone: 'Europe/Moscow', revision: 1,
  };
  let registered;
  const server = httpsServer({
    ...await hostPki.leaf('iam.demo.test'), ca: bffCA, requestCert: true, rejectUnauthorized: true,
    minVersion: 'TLSv1.3', maxVersion: 'TLSv1.3', requestTimeout: 3500, headersTimeout: 3000,
  }, (req, res) => {
    void (async () => {
      if (req.socket.getPeerCertificate().fingerprint256 !== bffFingerprint) return send(res, 403, failure('FORBIDDEN'));
      if (req.method === 'POST' && req.url === '/internal/v1/runtime/register') {
        const identity = await requestBody(req);
        if (identity.service !== 'BFF' || identity.runtime_id !== runtimeId || identity.generation !== 1 ||
            typeof identity.boot_id !== 'string' || identity.certificate_fingerprint !== prefixed(bffFingerprint)) {
          return send(res, 403, failure('FORBIDDEN'));
        }
        registered = identity;
        return send(res, 200, { runtime_id: runtimeId, boot_id: identity.boot_id, generation: identity.generation,
          snapshot_revision: 1, denials: [], gate: 'OPEN' });
      }
      if (!registered || req.headers['x-hitpipe-session'] !== ready.sessionToken) return send(res, 401, failure('UNAUTHORIZED'));
      if (req.method === 'GET' && req.url === '/api/v1/session') {
        return send(res, 200, { profile, user_id: ready.principalId, session_id: ready.sessionId, csrf_token: ready.csrfToken,
          expires_at: new Date(Date.now() + 600000).toISOString(), idle_expires_at: new Date(Date.now() + 180000).toISOString() });
      }
      if (req.method === 'GET' && req.url === '/api/v1/workspaces') return send(res, 200, { items: [region] });
      if (req.method === 'GET' && req.url === '/api/v1/profile') return send(res, 200, profile);
      if (req.method === 'POST' && req.url === '/internal/v1/authorize') {
        const request = await requestBody(req);
        const allowed = request.runtime_id === runtimeId && request.workspace_id === ready.workspaceId && allowedActions.has(request.action);
        return send(res, 200, { allowed, epoch: ready.securityEpoch, checked_at: new Date().toISOString(),
          revocation_pending: false, ...(allowed ? { role: 'OWNER', workspace_id: ready.workspaceId,
            principal_id: ready.principalId, session_id: ready.sessionId } : {}) });
      }
      return send(res, 404, failure('NOT_FOUND'));
    })().catch(() => { if (!res.headersSent) send(res, 503, failure('IAM_FIXTURE_UNAVAILABLE')); });
  });
  const port = await listen(server, 8443);
  return 'https://' + host + ':' + port;
}

async function relay(socket) {
  const server = tcpServer(client => {
    const backend = createConnection({ path: socket });
    client.on('error', () => backend.destroy());
    backend.on('error', () => client.destroy());
    client.on('close', () => backend.destroy());
    backend.on('close', () => client.destroy());
    client.pipe(backend).pipe(client);
  });
  return 'https://' + host + ':' + await listen(server);
}

function providerObservation(socketPath) {
  let current;
  return () => {
    if (current && Date.now() - current.startedAt < 250) return current.result;
    const result = new Promise((resolve, reject) => {
      const request = httpRequest({ socketPath, path: '/status', method: 'GET' }, response => {
        const chunks = [];
        let size = 0;
        response.on('data', chunk => {
          size += chunk.length;
          if (size > 8192) response.destroy(new Error('Invalid provider status'));
          else chunks.push(chunk);
        });
        response.on('error', reject);
        response.on('end', () => {
          try {
            const status = JSON.parse(Buffer.concat(chunks).toString('utf8'));
            if (response.statusCode !== 200 || !['caseCount', 'providerCreateCount', 'providerReadCount']
              .every(key => Number.isSafeInteger(status[key]) && status[key] >= 0)) {
              throw new Error('Local provider status unavailable');
            }
            resolve(new Date().toISOString());
          } catch (error) {
            reject(error);
          }
        });
      });
      request.setTimeout(2000, () => request.destroy(new Error('Local provider status timed out')));
      request.on('error', reject);
      request.end();
    });
    current = { result, startedAt: Date.now() };
    return result;
  };
}

async function proofServer() {
  const route = '/proof/' + randomUUID();
  const trigger = '/stripe-fixture/' + randomUUID();
  const simulator = process.env.HITPIPE_SCHOOL_EVENT;
  if (simulator && !simulator.startsWith('/')) throw new Error('The event launcher path must be absolute');
  const run = promisify(execFile);
  let submission;
  const port = await listen(httpServer((req, res) => {
    if (simulator && req.method === 'POST' && req.url === trigger) {
      const deliver = async () => {
        await run(simulator, ['paid'], { timeout: 7000 });
        await run(simulator, ['replay'], { timeout: 7000 });
      };
      submission ??= deliver();
      void submission.then(() => send(res, 202, { state: 'SUBMITTED_WITH_REPLAY' }),
        () => send(res, 503, failure('LOCAL_STRIPE_UNAVAILABLE')));
      return;
    }
    if (req.method !== 'GET' || req.url !== route) return send(res, 404, failure('NOT_FOUND'));
    void json(join(share, 'result.json')).then(result => send(res, 200, {
      create_count: result.providerCreateCount, read_count: result.providerReadCount, task_id: result.taskId,
      case_id: result.caseId, title: result.providerTitle ?? result.title, case_state: result.caseState,
      operation_id: result.operationId,
    })).catch(() => send(res, 202, { state: 'AWAITING_READBACK' }));
  }));
  const origin = 'http://' + host + ':' + port;
  return { url: origin + route, trigger: simulator ? origin + trigger : undefined };
}

async function schoolEntry(origin, token) {
  if (typeof token !== 'string' || !/^[A-Za-z0-9_-]{43,128}$/.test(token)) {
    throw new Error('Invalid synthetic session credential');
  }
  const route = '/school-session/' + randomUUID();
  const expectedHost = host + ':' + entryPort;
  await listen(httpServer((req, res) => {
    if (req.method !== 'GET' || req.headers.host !== expectedHost || req.url !== route) {
      return send(res, 404, failure('NOT_FOUND'));
    }
    res.writeHead(303, {
      Location: origin + '/',
      'Set-Cookie': '__Host-hp_session=' + token + '; Path=/; HttpOnly; Secure; SameSite=Lax',
      'Cache-Control': 'no-store',
      'Referrer-Policy': 'no-referrer',
    });
    res.end();
  }), entryPort);
  return 'http://' + expectedHost + route;
}

async function safeWrite(path, data) {
  const temporary = path + '.' + randomUUID();
  await writeFile(temporary, JSON.stringify(data, null, 2) + '\n', { mode: 0o600, flag: 'wx' });
  await rename(temporary, path);
}

async function waitForSession(origin, token, process) {
  for (let attempt = 0; attempt < 100; attempt += 1) {
    if (interrupted || process.exitCode !== null || process.signalCode !== null) throw new Error('Next stopped before session readiness');
    try {
      const response = await fetch(origin + '/api/v1/session', {
        headers: { Cookie: '__Host-hp_session=' + token }, signal: AbortSignal.timeout(1200),
      });
      if (response.status === 200) return;
      await response.body?.cancel();
    } catch {
      // Next can still be starting or registering the local BFF.
    }
    await sleep(150);
  }
  throw new Error('Next did not serve the demo session via mTLS');
}

async function run() {
  const ready = await json(join(share, 'ready.json'));
  ready.securityEpoch ??= 7;
  await mkdir(report, { recursive: true, mode: 0o700 });
  hostPkiDirectory = await mkdtemp(join(share, 'host-pki-'));
  const pki = await hostTLS(hostPkiDirectory);
  const path = name => join(share, name);
  const bffCert = path(ready.certPath);
  const bffKey = path(ready.keyPath);
  const processingCa = path(ready.caPath);
  const bffCA = await readFile(processingCa);
  const bffFingerprint = tlsFingerprint(await readFile(bffCert));
  const runtimeId = randomUUID();
  const iamOrigin = await startIAM(ready, pki, bffCA, bffFingerprint, runtimeId);
  const processing = {
    origin: await relay(path(ready.processingSocket)), serverName: ready.processingServerName, ca: processingCa,
  };
  const { startMetadata } = await import('../browser-metadata/metadata.mjs');
  const observe = providerObservation(path('provider.sock'));
  const metadata = await startMetadata({ ready, pki, bffCA, bffFingerprint, listen, observe });
  const publicOrigin = 'http://' + host + ':' + webPort;
  const bffConfig = path('bff-config.json');
  await writeFile(bffConfig, JSON.stringify({
    publicOrigin, iamOrigin, iamServerName: 'iam.demo.test', iamCa: pki.caPath, bffCert, bffKey,
    controlHost: host, controlCa: pki.caPath, controlCert: pki.certPath('bff-control.demo.test'),
    controlKey: pki.keyPath('bff-control.demo.test'), controlIamName: 'iam-control.demo.test',
    controlIamFingerprint: tlsFingerprint(await readFile(pki.certPath('iam-control.demo.test'))),
    runtimeId, generation: 1, owners: { processing, ...metadata },
  }));
  const next = start(process.execPath, ['server.mjs', '--iam-test-transport'], frontend, {
    PORT: new URL(publicOrigin).port, HOST: host, HITPIPE_BFF_CONFIG: bffConfig, NEXT_TELEMETRY_DISABLED: '1',
  });
  await waitForSession(publicOrigin, ready.sessionToken, next.child);
  const connectionsResponse = await fetch(publicOrigin + '/api/v1/workspaces/' + ready.workspaceId + '/connections', {
    headers: { Cookie: '__Host-hp_session=' + ready.sessionToken }, signal: AbortSignal.timeout(8000),
  });
  const connections = await connectionsResponse.json();
  if (connectionsResponse.status !== 200 || !connections.items.some(item => item.connection_id === ready.sourceConnectionId && item.provider === 'STRIPE')
      || !connections.items.some(item => item.connection_id === ready.destinationConnectionId && item.provider === 'CLICKUP')) {
    throw new Error('BFF did not return the two provisioned TEST connections: ' + (connections.code ?? connectionsResponse.status));
  }
  for (const [connection, kind, alias] of [
    [ready.sourceConnectionId, 'PRICE', ready.priceAlias],
    [ready.destinationConnectionId, 'LIST', ready.listAlias],
    [ready.destinationConnectionId, 'ASSIGNEE', ready.assigneeAlias],
  ]) {
    const target = publicOrigin + '/api/v1/workspaces/' + ready.workspaceId + '/connections/' + connection + '/options?kind=' + kind;
    const response = await fetch(target, {
      headers: { Cookie: '__Host-hp_session=' + ready.sessionToken }, signal: AbortSignal.timeout(8000),
    });
    const options = await response.json();
    if (response.status !== 200 || !options.items.some(item => item.alias === alias && item.kind === kind && item.available)) {
      throw new Error('BFF did not return the provisioned ' + kind + ': ' + (options.code ?? response.status));
    }
  }
  const proof = await proofServer();
  const entryUrl = await schoolEntry(publicOrigin, ready.sessionToken);
  await safeWrite(path('browser-config.json'), {
    origin: publicOrigin, workspaceId: ready.workspaceId, sessionToken: ready.sessionToken,
    stripeConnectionId: ready.sourceConnectionId, clickupConnectionId: ready.destinationConnectionId,
    priceAlias: ready.priceAlias, listAlias: ready.listAlias, assigneeAlias: ready.assigneeAlias,
    serviceLabel: ready.serviceLabel, clickupProofUrl: proof.url, stripeTriggerUrl: proof.trigger, artifactDir: report,
    frontendRoot: frontend, chromiumExecutable: process.env.HITPIPE_CHROMIUM_EXECUTABLE,
  });
  await safeWrite(join(report, 'web-ready.json'), { origin: publicOrigin, entryUrl, proofUrl: proof.url, mode: 'school-fixture' });
  console.log('HitPipe school UI ready on ' + publicOrigin + ' (loopback).');
  const result = await next.done;
  if (!interrupted) throw new Error('Next stopped: ' + (result.signal ?? result.code));
}

try {
  await run();
} catch (error) {
  console.error(error.message);
  process.exitCode = 1;
} finally {
  stop();
  await Promise.allSettled(servers.reverse().map(close => close()));
  await rm(join(report, 'web-ready.json'), { force: true }).catch(() => {});
  if (hostPkiDirectory) await rm(hostPkiDirectory, { recursive: true, force: true }).catch(() => {});
}
