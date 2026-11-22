import { connect, createServer, isIPv4 } from 'node:net';
import { readFileSync } from 'node:fs';

const route = process.argv[2];
function addresses() {
  const values = readFileSync(route, 'utf8').trim().split(/\s+/);
  if (values.length !== 2 || !values.every(isIPv4)) throw new Error('Invalid Compose addresses');
  return values;
}
addresses();

function forward(target, port, localPort) {
  const server = createServer(client => {
    let upstream;
    try {
      upstream = connect({ host: addresses()[target], port });
    } catch {
      client.destroy();
      return;
    }
    client.on('error', () => upstream.destroy());
    upstream.on('error', () => client.destroy());
    client.pipe(upstream).pipe(client);
  });
  server.on('error', error => {
    console.error(`Lab loopback listener ${localPort} failed (${error.code ?? 'unknown'}).`);
    process.exit(1);
  });
  server.listen(localPort, '127.0.0.1');
}

forward(0, 8080, 18080);
forward(1, 8025, 18025);
