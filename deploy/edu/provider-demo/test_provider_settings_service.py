"""In-app provider setup keeps credentials and webhook ownership per workspace."""
import json
from pathlib import Path
import sqlite3
import tempfile
import unittest
from unittest.mock import patch

from provider_connections import ProviderError
from provider_settings_service import WorkspaceConnections

A = "11111111-1111-4111-8111-111111111111"
B = "22222222-2222-4222-8222-222222222222"
RED = "t000000000000001"
BLUE = "t000000000000002"


class FakeProviders:
    calls = []

    def __init__(self, stripe_key, clickup_key):
        self.stripe_key = stripe_key
        self.clickup_key = clickup_key

    def validate_selection(self, fields):
        return {"sourceAccount": "acct_" + self.stripe_key,
                **{k: fields[k] for k in ("sourcePrice", "destinationWorkspace", "targetList", "assignee")},
                "label": "Verified product"}

    def webhook(self, url, idempotency):
        self.calls.append((self.stripe_key, url, idempotency))
        return "we_" + self.stripe_key, "whsec_" + self.stripe_key

    def inspect(self):
        return {"state": "VERIFIED", "stripe": {"account": "acct_" + self.stripe_key, "prices": []},
                "clickup": {"workspaces": []}}


class SettingsTests(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        self.settings = root / "settings"
        self.runtime = root / "runtime"
        self.ready = root / "ready"
        self.settings.mkdir(mode=0o700)
        self.runtime.mkdir(mode=0o700)
        self.ready.mkdir(mode=0o700)
        registry = root / "tenants.sqlite3"
        with sqlite3.connect(registry) as db:
            db.execute("CREATE TABLE school_tenants (workspace_id TEXT PRIMARY KEY, sandbox_slug TEXT)")
            db.executemany("INSERT INTO school_tenants VALUES (?,?)", [(A, RED), (B, BLUE)])
        self.service = WorkspaceConnections(registry, self.settings, self.runtime, self.ready,
                                            "https://hitpipe.dkar-dev.ru", FakeProviders)
        FakeProviders.calls.clear()

    @staticmethod
    def request(key):
        return {"action": "connect", "stripeApiKey": key, "clickupToken": "click_" + key,
                "sourcePrice": "price_1", "destinationWorkspace": "200", "targetList": "300", "assignee": "400"}

    def test_workspace_connections_keep_distinct_credentials_and_endpoints(self):
        a = self.service.request(A, "POST", self.request("sboxAlpha"))
        b = self.service.request(B, "POST", self.request("sboxBravo"))
        self.assertEqual(a["state"], "CONNECTING")
        self.assertNotEqual(a["webhookUrl"], b["webhookUrl"])
        self.assertEqual([row[1].rsplit("/", 1)[1] for row in FakeProviders.calls], [A, B])
        for workspace, key in ((A, "sboxAlpha"), (B, "sboxBravo")):
            folder = self.settings / workspace
            private = folder / "credentials.json"
            self.assertEqual(private.stat().st_mode & 0o777, 0o600)
            self.assertEqual(json.loads(private.read_text())["stripe_api_key"], key)
            self.assertNotIn("click_" + key, json.dumps(self.service.request(workspace, "GET")))
        self.service.request(A, "POST", self.request("sboxAlpha"))
        self.assertEqual(len(FakeProviders.calls), 2)
        with self.assertRaises(ProviderError):
            self.service.request(A, "POST", {**self.request("sboxAlpha"), "targetList": "500"})

    def test_error_reuses_stripe_idempotency_key(self):
        def failing(vendor, url, key):
            FakeProviders.calls.append((vendor.stripe_key, url, key))
            raise ProviderError("timed out", status=503)

        with patch.object(FakeProviders, "webhook", failing):
            with self.assertRaises(ProviderError):
                self.service.request(A, "POST", self.request("sboxAlpha"))
        self.assertEqual(self.service.request(A, "GET")["state"], "UNAVAILABLE")
        self.assertFalse((self.settings / A / "credentials.json").exists())
        self.service.request(A, "POST", self.request("sboxAlpha"))
        self.assertEqual(FakeProviders.calls[0], FakeProviders.calls[1])

    def test_unregistered_workspace_cannot_create_settings(self):
        with self.assertRaises(ProviderError):
            self.service.request("00000000-0000-0000-0000-000000000000", "POST", self.request("sboxAlpha"))
        self.assertFalse(FakeProviders.calls)
        self.assertFalse(list(self.settings.iterdir()))


if __name__ == "__main__":
    unittest.main()
