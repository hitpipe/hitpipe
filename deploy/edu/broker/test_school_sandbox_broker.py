import json
from pathlib import Path
import sqlite3
import stat
import tempfile
from types import SimpleNamespace
import unittest
from uuid import uuid4

from school_sandbox_broker import BINARIES, READY, Broker, Slots, read_json, write_private


ISSUER = "https://school.example.test"
TOKEN = "t" * 43
CSRF = "c" * 43


class Systemd:
    def __init__(self):
        self.status = {}
        self.started = []

    def states(self, slots):
        return {slot.slug: self.status.get(slot.slug, "inactive") for slot in slots}

    def start(self, slot, script, environment):
        self.started.append((slot, environment))
        self.status[slot.slug] = "activating"

    def stop_unready(self, slot):
        self.status[slot.slug] = "inactive"


class BrokerTest(unittest.TestCase):
    def setUp(self):
        folder = tempfile.TemporaryDirectory()
        self.addCleanup(folder.cleanup)
        root = Path(folder.name)
        self.config = SimpleNamespace(ready_root=root, registry=root / "registry.sqlite3", issuer=ISSUER,
                                      max_running=2, node=root / "node", go=root / "go",
                                      sources=root / "sources", frontend=root / "frontend",
                                      binaries=root / "binaries")
        self.config.binaries.mkdir(mode=0o700)
        for name in BINARIES:
            write_private(self.config.binaries / name, b"verified test artifact")
        self.allocations = [(f"t{number:015x}", str(uuid4())) for number in range(3)]
        with sqlite3.connect(self.config.registry) as db:
            db.execute("CREATE TABLE registry_config(singleton INTEGER, issuer TEXT)")
            db.execute("CREATE TABLE school_tenants(sandbox_slug TEXT, workspace_id TEXT)")
            db.execute("INSERT INTO registry_config VALUES (1, ?)", (ISSUER,))
            db.executemany("INSERT INTO school_tenants VALUES (?, ?)", self.allocations)
        self.config.registry.chmod(0o600)
        self.slots = Slots(root)
        self.addCleanup(self.slots.close)
        self.systemd = Systemd()
        self.response = {}
        self.probes = []
        self.broker = Broker(self.config, self.slots, self.systemd, self.probe)

    def probe(self, port, path, token):
        self.probes.append((port, path))
        self.assertEqual(token, TOKEN)
        return self.response[(port, path)]

    def publish_child(self, slot):
        state = self.config.ready_root / slot.slug
        run = state / "run.client"
        run.mkdir(mode=0o700)
        bridge = run / "bridge"
        bridge.mkdir(mode=0o700)
        user, session = str(uuid4()), str(uuid4())
        for path, payload in (
            (run / "web-ready.json", {"origin": slot.origin, "mode": "school-fixture"}),
            (bridge / "ready.json", {"workspaceId": slot.workspace_id, "sessionToken": TOKEN, "csrfToken": CSRF,
                                     "principalId": user, "sessionId": session}),
            (bridge / "browser-config.json", {"workspaceId": slot.workspace_id, "sessionToken": TOKEN,
                                              "origin": slot.origin, "schoolStripeEvents": True}),
        ):
            write_private(path, json.dumps(payload).encode())
        (state / "current").symlink_to(run)
        self.response[(slot.web_port, "/api/v1/session")] = {
            "user_id": user, "profile": {"user_id": user}, "session_id": session, "csrf_token": CSRF,
        }
        self.response[(slot.web_port, "/api/v1/workspaces")] = {
            "items": [{"workspace_id": slot.workspace_id, "role": "OWNER"}],
        }
        self.systemd.status[slot.slug] = "active"
        return state / READY

    def test_stable_distinct_slots_and_bounded_provisioning(self):
        self.broker.reconcile()
        self.broker.reconcile()
        (first, first_env), = self.systemd.started
        self.assertEqual(first_env["HITPIPE_SCHOOL_MANAGED"], 1)
        self.assertEqual(first_env["HITPIPE_SCHOOL_WORKSPACE_FILE"].read_text().strip(), first.workspace_id)
        private_first = self.publish_child(first)
        self.broker.reconcile()
        second, second_env = self.systemd.started[1]
        self.assertEqual(second_env["HITPIPE_SCHOOL_WORKSPACE_FILE"].read_text().strip(), second.workspace_id)
        for attr in ("number", "workspace_id", "internal_host", "web_port", "unit"):
            self.assertNotEqual(getattr(first, attr), getattr(second, attr))
        self.publish_child(second)
        self.broker.reconcile()
        self.assertEqual(len(self.systemd.started), 2)
        self.assertEqual(stat.S_IMODE(private_first.stat().st_mode), 0o600)
        self.assertEqual(read_json(private_first), {"workspace_id": first.workspace_id, "sandbox_slug": first.slug,
                                                    "csrf_token": CSRF, "session_token": TOKEN, "origin": first.origin})
        reopened = Slots(self.config.ready_root)
        self.addCleanup(reopened.close)
        restart = Broker(self.config, reopened, self.systemd, self.probe)
        prior = len(self.probes)
        restart.reconcile()
        self.assertEqual(len(self.probes) - prior, 4)
        self.assertEqual(len(self.systemd.started), 2)
        self.systemd.status[first.slug] = "inactive"
        restart.reconcile()
        self.assertFalse(private_first.exists())

    def test_cross_workspace_and_mismatching_csrf_are_not_published(self):
        self.broker.reconcile()
        first, _ = self.systemd.started[0]
        ready = self.publish_child(first)
        workspaces = self.response[(first.web_port, "/api/v1/workspaces")]
        workspaces["items"][0]["workspace_id"] = self.allocations[1][1]
        self.broker.reconcile()
        self.assertFalse(ready.exists())
        workspaces["items"][0]["workspace_id"] = first.workspace_id
        session = self.response[(first.web_port, "/api/v1/session")]
        session["csrf_token"] = "x" * 43
        self.broker.reconcile()
        self.assertFalse(ready.exists())
        session["csrf_token"] = CSRF
        self.broker.reconcile()
        self.assertTrue(ready.is_file())

    def test_disables_legacy_entry_even_when_child_claims_ready(self):
        self.broker.reconcile()
        slot, _ = self.systemd.started[0]
        ready = self.publish_child(slot)
        web = (ready.parent / "current").resolve() / "web-ready.json"
        payload = read_json(web)
        payload["entryUrl"] = "http://127.0.0.1/school-session/legacy"
        write_private(web, json.dumps(payload).encode())
        self.broker.reconcile()
        self.assertFalse(ready.exists())
        self.assertEqual(self.probes, [])


if __name__ == "__main__":
    unittest.main()
