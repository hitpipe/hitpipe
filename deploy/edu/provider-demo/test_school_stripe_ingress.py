from contextlib import closing
from http.client import HTTPConnection
from http.server import BaseHTTPRequestHandler
from pathlib import Path
from socketserver import UnixStreamServer
from tempfile import TemporaryDirectory
from threading import Thread
import unittest

import school_stripe_ingress as ingress


HOST = "hitpipe.dkar-dev.ru"
WORKSPACE = "11111111-1111-4111-8111-111111111111"
STRIPE_ACCOUNT = "acct_TestWorkspace"
SIGNATURE = "t=1770000000,v1=" + "a" * 64 + ",v1=" + "b" * 64
CONTENT_TYPE = "application/json; charset=utf-8"
BODY = b'{\r\n  "id":"evt_005",\n  "test":"\xd0\x90"\n}\n'


def serve(test, server):
    thread = Thread(target=server.serve_forever, kwargs={"poll_interval": 0.01})
    thread.start()

    def stop():
        server.shutdown()
        thread.join(timeout=3)
        server.server_close()

    test.addCleanup(stop)


class GatewayTests(unittest.TestCase):
    def setUp(self):
        self.deliveries = []
        self.status = 204

        def deliver(signature, content_type, body):
            self.deliveries.append((signature, content_type, body))
            return self.status

        # The real Unix hop is tested separately below.
        class Destination:
            pass

        destination = Destination()
        destination.deliver = deliver
        self.server = ingress.make_server(("127.0.0.1", 0), {"127.0.0.1"}, HOST, destination)
        serve(self, self.server)

    def exchange(self, method="POST", target=ingress.PUBLIC_PATH, *, body=BODY, headers=None):
        if headers is None:
            headers = [("Content-Type", CONTENT_TYPE), ("Stripe-Signature", SIGNATURE),
                       ("Content-Length", str(len(body)))]
        with closing(HTTPConnection(*self.server.server_address, timeout=2)) as client:
            client.putrequest(method, target, skip_host=True, skip_accept_encoding=True)
            if not any(name.lower() == "host" for name, _ in headers):
                client.putheader("Host", HOST)
            for name, value in headers:
                client.putheader(name, value)
            client.endheaders(body)
            with closing(client.getresponse()) as response:
                return response.status, response.getheaders(), response.read()

    def test_delivers_exact_body_and_one_signature_header(self):
        code, headers, reply = self.exchange()
        self.assertEqual((code, reply), (204, b""))
        self.assertEqual(headers, [("Cache-Control", "no-store")])
        self.assertEqual(self.deliveries, [(SIGNATURE, CONTENT_TYPE, BODY)])
        self.status = 401
        self.assertEqual(self.exchange()[0], 401)

    def test_rejects_non_webhook_routes_and_untrusted_host(self):
        for path in ("/", "/auth/callback", "/api/v1/school/stripe-events", "/webhooks/stripe/other",
                     "/webhooks/stripe/demo?", "/webhooks/stripe/demo?q=demo", "//webhooks/stripe/demo",
                     "/webhooks/stripe/demo/", "/webhooks/stripe/%64emo", "http://" + HOST + ingress.PUBLIC_PATH):
            self.assertEqual(self.exchange(target=path)[0], 404, path)
        self.assertEqual(self.exchange(method="GET")[0], 405)
        self.assertEqual(self.exchange(headers=[("Host", "somewhere.invalid")])[0], 421)
        self.assertFalse(self.server.verify_request(None, ("127.0.0.2", 1024)))
        self.assertEqual(self.deliveries, [])

    def test_rejects_ambiguous_and_oversize_requests_before_intake(self):
        base = [("Content-Type", CONTENT_TYPE), ("Stripe-Signature", SIGNATURE), ("Content-Length", "5")]
        for name in ("Host", "Stripe-Signature", "Content-Type", "Content-Length"):
            altered = [("Host", HOST), *base, (name, "unexpected")]
            self.assertEqual(self.exchange(body=b"12345", headers=altered)[0], 400, name)
        for name in ("Transfer-Encoding", "Content-Encoding", "Expect"):
            self.assertEqual(self.exchange(body=b"12345", headers=[*base, (name, "unsafe")])[0], 400)
        blocked = [("Content-Type", CONTENT_TYPE), ("Stripe-Signature", SIGNATURE),
                   ("Content-Length", str(ingress.MAX_BODY + 1))]
        self.assertEqual(self.exchange(body=b"", headers=blocked)[0], 413)
        folded = SIGNATURE + "," + SIGNATURE
        for signature in ("", folded):
            blocked = [("Content-Type", CONTENT_TYPE), ("Stripe-Signature", signature), ("Content-Length", "5")]
            self.assertEqual(self.exchange(body=b"12345", headers=blocked)[0], 401)
        self.assertEqual(self.exchange(headers=base[:2])[0], 411)
        self.assertEqual(self.deliveries, [])


class IntakeBindingTests(unittest.TestCase):
    def setUp(self):
        temporary = TemporaryDirectory(prefix="stripe-ingress-", dir="/tmp")
        self.addCleanup(temporary.cleanup)
        self.share = Path(temporary.name)
        self.binding = ingress.IntakeBinding(WORKSPACE, self.share, STRIPE_ACCOUNT)
        self.ready = self.share / "ready.json"
        self.ready.touch(mode=0o600)

    def write_ready(self, workspace=WORKSPACE, stripe_account=STRIPE_ACCOUNT):
        self.ready.write_text('{"workspaceId":"' + workspace + '","providerDemo":{"sourceAccount":"'
                              + stripe_account + '"}}')

    def test_wrong_workspace_and_untrusted_files_never_select_a_relay(self):
        self.write_ready(workspace="00000000-0000-0000-0000-000000000000")
        with self.assertRaisesRegex(ValueError, "another workspace"):
            with self.binding.destination():
                self.fail("accepted a different workspace")
        self.write_ready(stripe_account="acct_Foreign")
        with self.assertRaisesRegex(ValueError, "another Stripe account"):
            with self.binding.destination():
                self.fail("accepted a different Stripe account")
        self.write_ready()
        (self.share / ingress.SOCKET_NAME).symlink_to("/tmp/foreign-intake.sock")
        with self.assertRaisesRegex(ValueError, "owner-only socket"):
            with self.binding.destination():
                self.fail("accepted an untrusted relay path")
        self.ready.chmod(0o644)
        with self.assertRaisesRegex(ValueError, "ownership, mode"):
            with self.binding.destination():
                self.fail("accepted an untrusted manifest")

    def test_unix_hop_sends_exact_bytes_to_only_the_fixed_installation(self):
        self.write_ready()
        received = []
        response_status = 204

        class TinyIntake(BaseHTTPRequestHandler):
            def do_POST(self):
                payload = self.rfile.read(int(self.headers["Content-Length"]))
                received.append((self.path, self.headers.get_all("Stripe-Signature"), self.headers["Content-Type"],
                                 self.headers["Host"], self.headers.get("Cookie"), payload))
                self.send_response_only(response_status)
                self.send_header("Set-Cookie", "internal=not-public")
                self.end_headers()

        relay_path = self.share / ingress.SOCKET_NAME
        serve(self, UnixStreamServer(str(relay_path), TinyIntake))
        relay_path.chmod(0o644)
        with self.assertRaisesRegex(ValueError, "owner-only socket"):
            self.binding.deliver(SIGNATURE, CONTENT_TYPE, BODY)
        relay_path.chmod(0o600)
        self.assertEqual(self.binding.deliver(SIGNATURE, CONTENT_TYPE, BODY), 204)
        self.assertEqual(received, [("/webhooks/stripe/demo", [SIGNATURE], CONTENT_TYPE,
                                     "school-stripe-ingress", None, BODY)])
        response_status = 302
        self.assertEqual(self.binding.deliver(SIGNATURE, CONTENT_TYPE, BODY), 503)
        response_status = 412
        self.assertEqual(self.binding.deliver(SIGNATURE, CONTENT_TYPE, BODY), 412)


if __name__ == "__main__":
    unittest.main()
