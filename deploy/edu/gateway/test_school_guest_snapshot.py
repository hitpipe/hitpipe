from contextlib import closing
from hashlib import sha256
from http.client import HTTPConnection
import json
from pathlib import Path
from tempfile import TemporaryDirectory
from threading import Thread
import unittest
from unittest.mock import patch

import school_public_origin as origin
from school_guest_snapshot import GuestSnapshot


PAGES = {
    'public/hitpipe-entry.html': b'<h1>Guest</h1><a href="/auth/login">Log in</a>',
    'public/hitpipe-operation-entry.html': b'<h1>Setup</h1><a href="/auth/login">Log in</a>',
    'public/hitpipe-mark.svg': b'<svg></svg>',
    'public/_next/static/css/6af3d88d.css': b'body{color:black}',
}


class DirectPublicOriginTests(unittest.TestCase):
    def setUp(self):
        tmp = TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.stage = Path(tmp.name)
        rows = []
        for name, data in PAGES.items():
            path = self.stage / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(data)
            rows.append({'path': name, 'bytes': len(data), 'sha256': sha256(data).hexdigest()})
        self.manifest = self.stage / 'manifest.json'
        self.manifest.write_text(json.dumps({'files': rows}))
        self.digest = sha256(self.manifest.read_bytes()).hexdigest()
        snapshot = GuestSnapshot(self.stage, self.manifest, self.digest)
        self.server = origin.make_server(('127.0.0.1', 0), {'127.0.0.1'}, 'app', 'idp', snapshot,
                                         require_cloudflare_https=True)
        thread = Thread(target=self.server.serve_forever, kwargs={'poll_interval': 0.01})
        thread.start()

        def stop():
            self.server.shutdown()
            thread.join(timeout=2)
            self.server.server_close()

        self.addCleanup(stop)
        self.exchange = self.enterContext(patch.object(origin, 'owner_exchange', return_value=(
            200, (('Content-Type', 'text/plain'),), b'upstream', 8,
        )))

    def request(self, method, path, headers=(), *, scheme='https'):
        with closing(HTTPConnection(*self.server.server_address, timeout=2)) as client:
            client.putrequest(method, path, skip_host=True)
            if not any(key.lower() == 'host' for key, _ in headers):
                client.putheader('Host', origin.PUBLIC_HOST)
            if scheme is not None:
                client.putheader('X-Forwarded-Proto', scheme)
            for name, value in headers:
                client.putheader(name, value)
            client.endheaders()
            response = client.getresponse()
            return response.status, response.headers, response.read()

    def test_anonymous_and_malformed_sessions_get_same_no_store_document(self):
        for path, data in zip(('/', '/operation-setup'), PAGES.values()):
            for headers in ((), (('Cookie', '__Host-hp_session=expired'),),
                            (('Cookie', '__Host-hp_session=' + 'a' * 122 + '.'),),
                            (('Cookie', '__Host-hp_oidc_flow=example'),)):
                status, response, body = self.request('GET', path, headers)
                self.assertEqual((status, body), (200, data))
                self.assertEqual(response['Cache-Control'], 'private, no-store')
                self.assertEqual(response['Vary'], 'Cookie')
                self.assertIsNone(response['Set-Cookie'])
            status, response, body = self.request('HEAD', path)
            self.assertEqual((status, body), (200, b''))
            self.assertEqual(int(response['Content-Length']), len(data))
        self.exchange.assert_not_called()

    def test_sealed_shape_and_dynamic_routes_keep_the_owner_exchange(self):
        valid = '__Host-hp_session=' + 'a' * 123
        for path in ('/?workspace=example', '/operation-setup'):
            self.assertEqual(self.request('GET', path, (('Cookie', valid),))[2], b'upstream')
            args = self.exchange.call_args.args
            self.assertEqual(args[:3], ('app', 'GET', path))
            self.assertEqual(args[3]['Cookie'], valid)
        for path, expected in (('/api/v1/session', 'app'), ('/auth/login', 'app'),
                               ('/webhooks/stripe/abc', 'app'), (origin.AUTHORIZE + '?example=1', 'idp'),
                               ('/_next/static/css/older.css', 'app')):
            self.assertEqual(self.request('GET', path)[2], b'upstream')
            self.assertEqual(self.exchange.call_args.args[:3], (expected, 'GET', path))

    def test_asset_cache_control_and_conditional_response(self):
        status, headers, body = self.request('GET', '/_next/static/css/6af3d88d.css')
        self.assertEqual((status, body), (200, PAGES['public/_next/static/css/6af3d88d.css']))
        self.assertEqual(headers['Content-Type'], 'text/css; charset=utf-8')
        self.assertEqual(headers['Cache-Control'], 'public, max-age=31536000, immutable')
        self.assertEqual(headers['X-Content-Type-Options'], 'nosniff')
        tag = headers['ETag']
        self.assertEqual(self.request('GET', '/_next/static/css/6af3d88d.css', (('If-None-Match', tag),))[0], 304)
        self.assertNotIn('immutable', self.request('GET', '/hitpipe-mark.svg')[1]['Cache-Control'])
        self.exchange.assert_not_called()

    def test_existing_rejections_precede_static_dispatch(self):
        self.assertEqual(self.request('GET', '/', (('Host', 'foreign.example'),))[0], 421)
        self.assertEqual(self.request('GET', '/', (('Cookie', 'a=x'), ('Cookie', 'b=y')))[0], 400)
        self.assertEqual(self.request('GET', '/', (('Content-Length', '1'),))[0], 400)
        self.assertEqual(self.request('GET', '/health/ready')[0], 404)
        self.assertEqual(self.request('POST', '/api/v1/ai/proposals?')[0], 400)
        self.assertFalse(self.server.verify_request(None, ('127.0.0.2', 1234)))
        self.exchange.assert_not_called()

    def test_modified_manifest_and_static_bytes_are_rejected(self):
        with self.assertRaisesRegex(ValueError, 'pinned digest'):
            GuestSnapshot(self.stage, self.manifest, '0' * 64)
        file = self.stage / 'public/hitpipe-mark.svg'
        file.write_bytes(b'<svg> </svg>')
        with self.assertRaisesRegex(ValueError, 'snapshot size'):
            GuestSnapshot(self.stage, self.manifest, self.digest)
        file.write_bytes(b'<svg></SVG>')
        with self.assertRaisesRegex(ValueError, 'does not match'):
            GuestSnapshot(self.stage, self.manifest, self.digest)

    def test_http_safe_reads_redirect_before_owners_and_mutations_are_denied(self):
        for method in ('GET', 'HEAD'):
            path = '/operation-setup?from=landing'
            status, headers, body = self.request(method, path, scheme='http')
            self.assertEqual((status, body), (308, b''))
            self.assertEqual(headers['Location'], f'https://{origin.PUBLIC_HOST}{path}')
            self.assertEqual(headers['Cache-Control'], 'private, no-store')
            self.assertEqual(headers['Content-Length'], '0')
        self.assertEqual(self.request('POST', '/api/v1/ai/proposals', scheme='http')[0], 403)
        self.assertEqual(self.request('DELETE', '/api/v1/session', scheme='http')[0], 403)
        self.exchange.assert_not_called()

    def test_connector_scheme_must_be_present_and_unambiguous(self):
        for scheme in (None, 'ssh', 'https,http', ''):
            self.assertEqual(self.request('GET', '/', scheme=scheme)[0], 400)
        self.assertEqual(self.request('GET', '/', (('X-Forwarded-Proto', 'https'),))[0], 400)
        self.assertEqual(self.request('GET', '/', (('X-Forwarded-Proto', 'http'),))[0], 400)
        self.exchange.assert_not_called()


if __name__ == '__main__':
    unittest.main()
