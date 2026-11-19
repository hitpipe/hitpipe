import assert from 'node:assert/strict';
import { connect, createServer, isIPv4 } from 'node:net';

const [idp, smtp, iamDB] = process.argv.slice(2);
assert.ok([idp, smtp, iamDB].every(addr => Boolean(addr) && isIPv4(addr)), 'Require three addresses from the isolated Compose network');

function forward(host, port, localPort) {
  const server = createServer(client => {
    const upstream = connect({ host, port });
    client.on('error', () => upstream.destroy());
    upstream.on('error', () => client.destroy());
    client.pipe(upstream).pipe(client);
  });
  server.on('error', error => {
    console.error(`Lab loopback ${localPort} failed (${error.code ?? 'unknown'}).`);
    process.exit(1);
  });
  server.listen(localPort, '127.0.0.1');
}

forward(idp, 8443, 18443);
forward(smtp, 8025, 18425);
forward(iamDB, 5432, 15444);
