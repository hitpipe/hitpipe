"""Exercise the headless client through the private provisioning socket."""
from contextlib import closing
import json
import os
from pathlib import Path
import sqlite3
import subprocess
import sys
import tempfile
import threading
import unittest

from connect_sandbox import DEMO_ACCOUNT, DEMO_SELECTION
from provider_settings_service import Handler, Service, WorkspaceConnections

SPACE = "11111111-1111-4111-8111-111111111111"
STRIPE = "sk_test_" + "a" * 20
CLICKUP = "pk_" + "b" * 20


class ProviderFixture:
    def __init__(self, stripe, clickup):
        assert (stripe, clickup) == (STRIPE, CLICKUP)
        self.stripe_key, self.clickup_key = stripe, clickup

    def inspect(self):
        return {"state": "VERIFIED", "stripe": {"account": DEMO_ACCOUNT,
                                                "prices": [{"id": DEMO_SELECTION["sourcePrice"]}]},
                "clickup": {"workspaces": [{"id": DEMO_SELECTION["destinationWorkspace"], "lists": [
                    {"id": DEMO_SELECTION["targetList"], "assignees": [{"id": DEMO_SELECTION["assignee"]}]},
                ]}]}}

    def validate_selection(self, request):
        assert all(request[name] == value for name, value in DEMO_SELECTION.items())
        return {"sourceAccount": DEMO_ACCOUNT, **DEMO_SELECTION, "label": "Sandbox"}

    def webhook(self, url, key):
        return "we_fixture", "whsec_fixture"


class HeadlessTest(unittest.TestCase):
    def test_status_connect_and_idempotent_replay_without_browser(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            registry, socket, keys = root / "db", root / "api.sock", root / "credentials.json"
            with closing(sqlite3.connect(registry)) as db, db:
                db.execute("CREATE TABLE school_tenants(workspace_id TEXT, sandbox_slug TEXT)")
                db.execute("INSERT INTO school_tenants VALUES (?, ?)", (SPACE, "t000000000000001"))
            registry.chmod(0o600)
            for folder in (root/"data",root/"runtime",root/"ready"):
                folder.mkdir(mode=0o700)
            with Service(os.fspath(socket), Handler) as server:
                socket.chmod(0o600)
                server.slots = threading.BoundedSemaphore(1)
                server.app = WorkspaceConnections(registry, root/"data", root/"runtime", root/"ready",
                                                 "https://hitpipe.dkar-dev.ru", ProviderFixture)
                threading.Thread(target=server.serve_forever, daemon=True).start()
                try:
                    def call(command):
                        result = subprocess.run([sys.executable, "-B", "connect_sandbox.py", command,
                                                 "--socket", str(socket), "--registry", str(registry),
                                                 "--credentials", str(keys)], capture_output=True, text=True, timeout=12)
                        self.assertNotIn(STRIPE, result.stdout + result.stderr)
                        self.assertNotIn(CLICKUP, result.stdout + result.stderr)
                        return result
                    self.assertEqual(json.loads(call("status").stdout)["state"], "UNCONNECTED")
                    self.assertEqual(call("connect").returncode, 1)
                    keys.write_text(json.dumps({"stripe_api_key": STRIPE, "clickup_token": CLICKUP}))
                    keys.chmod(0o600)
                    first = call("connect")
                    self.assertEqual(first.returncode, 0, first.stderr)
                    self.assertEqual(json.loads(first.stdout), {"workspace": SPACE, "state": "CONNECTING", "demo_binding": True})
                    keys.unlink()
                    self.assertEqual(call("connect").stdout, first.stdout)
                finally:
                    server.shutdown()
