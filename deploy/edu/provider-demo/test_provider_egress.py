"""Exercise the single-workspace provider boundary without a real account."""
import json
from pathlib import Path
import tempfile
import unittest

from provider_egress import Application, CLICKUP, STRIPE, stripe_allowed


BINDING = {
    "sourceAccount": "acct_abcd1234",
    "sourcePrice": "price_abcd1234",
    "destinationWorkspace": "100",
    "targetList": "200",
    "assignee": "300",
    "label": "Test service",
}
KEYS = {"stripe_api_key": "sk_test_" + "a" * 24, "clickup_token": "pk_" + "b" * 24}


class EgressTests(unittest.TestCase):
    def setUp(self):
        self.requests = []
        self.path = tempfile.TemporaryDirectory()
        self.addCleanup(self.path.cleanup)
        self.app = Application(BINDING, KEYS, Path(self.path.name) / "proof.db", self.provider)

    def provider(self, url, token, *, method="GET", body=None, stripe=False):
        self.requests.append((method, url, stripe))
        if stripe:
            self.assertEqual(token, "Bearer " + KEYS["stripe_api_key"])
        else:
            self.assertEqual(token, KEYS["clickup_token"])
        records = {
            STRIPE + "/v1/account": {"id": BINDING["sourceAccount"]},
            STRIPE + "/v1/prices/price_abcd1234": {
                "id": BINDING["sourcePrice"], "type": "one_time", "livemode": False, "active": True
            },
            CLICKUP + "/api/v2/user": {"user": {"id": 300}},
            CLICKUP + "/api/v2/list/200/member": {"members": []},
            CLICKUP + "/api/v2/list/200": {"id": "200", "space": {"id": "3000"}},
            CLICKUP + "/api/v2/team/100/space?archived=false": {"spaces": [{"id": "3000"}]},
            CLICKUP + "/api/v2/list/200/task": {"id": "realTestId"},
            CLICKUP + "/api/v2/task/realTestId": {"id": "realTestId"},
        }
        return 200, {}, json.dumps(records[url]).encode()

    def test_binding_requires_test_key_before_any_request(self):
        with self.assertRaisesRegex(ValueError, "test Stripe key"):
            Application(BINDING, {**KEYS, "stripe_api_key": "sk_live_" + "z" * 24}, Path(self.path.name) / "x")
        status, _, response = self.app.handle("GET", "/binding", b"")
        self.assertEqual((status, json.loads(response)), (200, BINDING))

    def test_real_port_never_sends_unauthorized_writes_or_reads(self):
        body = json.dumps({"name": "One paid order", "description": "A test", "assignees": [300]}).encode()
        self.assertEqual(self.app.handle("POST", "/clickup/api/v2/list/201/task", body)[0], 404)
        self.assertEqual(self.app.handle("POST", "/clickup/api/v2/list/200/task", b'{}')[0], 400)
        self.assertEqual(self.app.handle("POST", "/stripe/v1/charges", body)[0], 404)
        self.assertEqual(self.app.handle("GET", "/clickup/api/v2/task/realTestId", b"")[0], 404)
        self.assertEqual(self.app.handle("POST", "/clickup/api/v2/list/200/task", body)[0], 200)
        self.assertEqual(self.app.handle("GET", "/clickup/api/v2/task/realTestId", b"")[0], 200)
        self.assertEqual(sum(m == "POST" for m, *_ in self.requests), 1)

    def test_stripe_request_allowlist(self):
        allowed = (
            "/v1/checkout/sessions/cs_test_a1/line_items?limit=100&starting_after=li_123",
            "/v1/refunds?charge=ch_demo",
            "/v1/prices/price_abcd1234",
        )
        for path in allowed:
            self.assertTrue(stripe_allowed(path, BINDING["sourcePrice"]), path)
        for path in ("/v1/account?evil=x", "/v1/checkout/sessions/cs_live_a1", "/v1/prices/price_other",
                     "/v1/refunds?charge=ch_demo&charge=ch_other", "/v1/refunds?charge=ch_demo&limit=101",
                     "//api.evil.example/v1/account"):
            self.assertFalse(stripe_allowed(path, BINDING["sourcePrice"]), path)


if __name__ == "__main__":
    unittest.main()
