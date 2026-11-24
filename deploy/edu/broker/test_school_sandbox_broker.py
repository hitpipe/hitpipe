import hashlib
import json
from pathlib import Path
import sqlite3
import stat
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch
from uuid import uuid4

from school_sandbox_broker import (
    BINARIES, EXECUTABLES, READY, Broker, Slots, UserSystemd, load_provider_profile,
    read_json, require_socket_paths, write_private,
)


ISSUER = "https://school.example.test"
TOKEN = "t" * 43
CSRF = "c" * 43
BINDING = {
    "sourceAccount": "acct_abcdefgh",
    "sourcePrice": "price_abcdefgh",
    "destinationWorkspace": "1234",
    "targetList": "2345",
    "assignee": "3456",
    "label": "Учебная услуга",
}


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

    def publish_child(self, slot, state=None, binding=None):
        manifest = self.config.ready_root / slot.slug / READY
        state = state or manifest.parent
        run = state / "run.client"
        run.mkdir(mode=0o700)
        bridge = run / "bridge"
        bridge.mkdir(mode=0o700)
        user, session = str(uuid4()), str(uuid4())
        raw = {"workspaceId": slot.workspace_id, "sessionToken": TOKEN, "csrfToken": CSRF,
               "principalId": user, "sessionId": session}
        if binding is not None:
            raw["providerDemo"] = binding
        for path, payload in (
            (run / "web-ready.json", {"origin": slot.origin, "mode": "school-fixture"}),
            (bridge / "ready.json", raw),
            (bridge / "browser-config.json", {"workspaceId": slot.workspace_id, "sessionToken": TOKEN,
                                              "origin": slot.origin, "schoolStripeEvents": binding is None,
                                              "schoolAi": binding is None}),
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
        return manifest

    def set_provider_profile(self, workspace):
        folder = tempfile.TemporaryDirectory()
        self.addCleanup(folder.cleanup)
        root = Path(folder.name)
        for path in (root / "state", root / "bin"):
            path.mkdir(mode=0o700)
        artifact = b"verified provider demo test artifact"
        for name in EXECUTABLES:
            file = root / "bin" / name
            write_private(file, artifact)
            file.chmod(0o700)
        receipt = "".join(f"{hashlib.sha256(artifact).hexdigest()}  {name}\n" for name in EXECUTABLES)
        write_private(root / "bin/binaries.sha256", receipt.encode("ascii"))
        write_private(root / "binding", json.dumps(BINDING).encode())
        write_private(root / "credentials", b"test")
        write_private(root / "secret", b"whsec_abcdefghijklmnop")
        profile = root / "profile.json"
        write_private(profile, json.dumps({
            "workspace_id": workspace, "state_root": str(root / "state"), "binaries": str(root / "bin"),
            "binding": str(root / "binding"), "credentials": str(root / "credentials"),
            "webhook_secret": str(root / "secret"),
        }).encode())
        self.config.provider_profile = load_provider_profile(profile, self.config.ready_root, self.config.binaries)

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

    def test_socket_path_limit_counts_bytes_before_provisioning(self):
        require_socket_paths(Path("/home/deploy/.hp-school/private"))
        with self.assertRaisesRegex(ValueError, "ready root is too long"):
            require_socket_paths(Path("/tmp") / ("а" * 27))
        marginal = Path("/tmp") / ("x" * 51)
        require_socket_paths(marginal)
        with self.assertRaisesRegex(ValueError, "ready root is too long"):
            require_socket_paths(marginal, provider_demo=True)

    def test_provider_profile_substitutes_one_instance_preserving_original_state(self):
        first, second, *_ = self.slots.remember(self.allocations)
        original = self.broker.sandbox(first)
        self.broker.prepare(first, original)
        write_private(original / "workspace-bound-id", b"original test database binding")
        self.set_provider_profile(first.workspace_id)

        self.broker.reconcile()
        selected, real = self.systemd.started[0]
        self.assertEqual(selected.description, f"HitPipe school provider demo {first.slug}")
        self.assertEqual(selected.unit, first.unit)
        state = real["HITPIPE_SCHOOL_STATE"]
        self.assertEqual(state, self.config.provider_profile.state_root / ("d" + first.slug[1:]))
        self.assertEqual(real["HITPIPE_SCHOOL_INSTANCE"], state.name)
        self.assertEqual(real["HITPIPE_PROVIDER_DEMO_CREDENTIALS"], self.config.provider_profile.credentials)
        self.assertEqual(real["HITPIPE_SCHOOL_AI"], 0)
        self.assertEqual(real["HITPIPE_SCHOOL_STRIPE_EVENTS"], 0)
        self.assertEqual(read_json(state / "provider-demo.json"), BINDING)
        self.assertEqual((state / "workspace-id").read_text().strip(), first.workspace_id)
        self.assertTrue((state / "bin/school-backend").samefile(self.config.provider_profile.binaries / "school-backend"))
        self.assertTrue((original / "bin/school-backend").samefile(self.config.binaries / "school-backend"))
        self.assertEqual((original / "workspace-bound-id").read_bytes(), b"original test database binding")
        self.assertFalse((original / "provider-demo.json").exists())

        ready = self.publish_child(selected, state, BINDING)
        self.broker.reconcile()
        self.assertEqual(ready.parent, original)
        self.assertEqual(read_json(ready)["sandbox_slug"], first.slug)
        self.assertFalse((state / READY).exists())
        ordinary, normal = self.systemd.started[1]
        self.assertEqual(ordinary.description, second.description)
        self.assertEqual(normal["HITPIPE_SCHOOL_INSTANCE"], second.slug)
        self.assertEqual(normal["HITPIPE_SCHOOL_STRIPE_EVENTS"], 1)
        self.assertNotIn("HITPIPE_PROVIDER_DEMO_CREDENTIALS", normal)
        self.assertTrue((normal["HITPIPE_SCHOOL_STATE"] / "bin/school-backend")
                        .samefile(self.config.binaries / "school-backend"))

    def test_selected_manifest_rejects_wrong_provider_and_synthetic_payment(self):
        self.set_provider_profile(self.allocations[0][1])
        self.broker.reconcile()
        slot, environment = self.systemd.started[0]
        state = environment["HITPIPE_SCHOOL_STATE"]
        ready = self.publish_child(slot, state, dict(BINDING, sourcePrice="price_wrongvalue"))
        self.broker.reconcile()
        self.assertFalse(ready.exists())
        bridge = (state / "current").resolve() / "bridge"
        raw = read_json(bridge / "ready.json")
        raw["providerDemo"] = BINDING
        write_private(bridge / "ready.json", json.dumps(raw).encode())
        browser = read_json(bridge / "browser-config.json")
        browser["schoolStripeEvents"] = True
        write_private(bridge / "browser-config.json", json.dumps(browser).encode())
        self.broker.reconcile()
        self.assertFalse(ready.exists())
        self.assertEqual(self.probes, [])

    def test_provider_profile_refuses_nonempty_unbound_state(self):
        first, *_ = self.slots.remember(self.allocations)
        self.set_provider_profile(first.workspace_id)
        demo = self.config.provider_profile.state_root / ("d" + first.slug[1:])
        demo.mkdir(mode=0o700)
        write_private(demo / "workspace-bound-id", b"existing database binding")
        with self.assertRaisesRegex(ValueError, "fresh school provider demo state"):
            self.broker.prepare(first, self.broker.sandbox(first))
        self.assertFalse((demo / "provider-demo.json").exists())

    def test_an_active_unit_in_the_other_mode_cannot_be_attested_or_stopped(self):
        original, *_ = self.slots.remember(self.allocations)
        self.set_provider_profile(original.workspace_id)
        self.broker.reconcile()
        demo_slot, _ = self.systemd.started[0]
        reply = SimpleNamespace(stdout=f"Id={original.unit}\nDescription={original.description}\n"
                                      "LoadState=loaded\nActiveState=active\nTransient=yes\n")
        with patch("school_sandbox_broker.subprocess.run", return_value=reply) as systemctl:
            systemd = UserSystemd()
            self.assertEqual(systemd.states([demo_slot]), {original.slug: "unmanaged"})
            systemd.stop_unready(demo_slot)
            self.assertTrue(all(call.args[0][2] == "show" for call in systemctl.call_args_list))


if __name__ == "__main__":
    unittest.main()
