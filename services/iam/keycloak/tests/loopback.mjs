import { connect, createServer, isIPv4 } from 'node:net';

const [keycloak, mailpit] = process.argv.slice(2);
if (![keycloak, mailpit].every(value => value && isIPv4(value))) {
  console.error('Lab loopback requires two addresses from the Compose network.');
  process.exit(1);
}

function forward(host, port, localPort) {
  const server = createServer(client => {
    const upstream = connect({ host, port });
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

forward(keycloak, 8080, 18080);
forward(mailpit, 8025, 18025);
