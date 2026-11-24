"""Launch and attest the private sandbox assigned to each school account."""

import argparse
import fcntl
import hashlib
import http.client
import json
import os
from contextlib import closing
from dataclasses import dataclass, replace
from hmac import compare_digest
from pathlib import Path
import re
import sqlite3
import stat
import subprocess
import sys
import tempfile
import time


UUID = re.compile(r"[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}")
SLUG = re.compile(r"t[0-9a-f]{15}")
TOKEN = re.compile(r"[A-Za-z0-9_-]{43,128}")
CSRF = re.compile(r"[A-Za-z0-9_-]{32,128}")
RUNTIME = re.compile(r"run\.[A-Za-z0-9]{6}")
FILE_LIMIT = 8192
MAX_SLOTS = 1024
START_TIMEOUT = 300
RETRY_DELAY = 20
UNIX_PATH_LIMIT = 107
READY = "gateway-ready.json"
EXECUTABLES = ("intake", "processing", "delivery-offline-receipt", "school-backend")
BINARIES = (*EXECUTABLES, "binaries.sha256")
ABSENT_STATES = {"inactive", "failed"}


def require_socket_paths(root: Path, provider_demo=False):
    # The backend binds inside the container, but Node connects through the
    # host path. Linux's pathname sockets have 108 bytes including the NUL.
    socket_name = "provider-egress.sock" if provider_demo else "processing.sock"
    longest_socket = root / "t0123456789abcd0" / "run.XXXXXX" / "bridge" / socket_name
    if len(os.fsencode(longest_socket)) > UNIX_PATH_LIMIT:
        raise ValueError("School ready root is too long for its Unix sockets; select a shorter absolute path")


def private_directory(path: Path):
    info = path.lstat()
    if not stat.S_ISDIR(info.st_mode) or stat.S_IMODE(info.st_mode) != 0o700 or info.st_uid != os.getuid():
        raise ValueError("School sandbox directory must be owner-only")


def private_file(path: Path, flags=os.O_RDONLY):
    fd = os.open(path, flags | os.O_NOFOLLOW | os.O_NONBLOCK, 0o600)
    try:
        info = os.fstat(fd)
        if (not stat.S_ISREG(info.st_mode) or stat.S_IMODE(info.st_mode) != 0o600
                or info.st_uid != os.getuid() or info.st_nlink != 1):
            raise ValueError("School sandbox control file must be owner-only")
        return fd
    except BaseException:
        os.close(fd)
        raise


def small_file(path: Path, limit=FILE_LIMIT) -> bytes:
    with os.fdopen(private_file(path), "rb") as stream:
        raw = stream.read(limit + 1)
    if len(raw) > limit:
        raise ValueError("School sandbox control file is too large")
    return raw


def unique_fields(pairs):
    result = dict(pairs)
    if len(result) != len(pairs):
        raise ValueError("Duplicate school sandbox fields")
    return result


def read_json(path: Path) -> dict:
    value = json.loads(small_file(path), object_pairs_hook=unique_fields)
    if not isinstance(value, dict):
        raise ValueError("Expected a school sandbox readiness object")
    return value


def write_private(path: Path, data: bytes):
    fd, name = tempfile.mkstemp(prefix=".broker-", dir=path.parent)
    temporary = Path(name)
    try:
        with os.fdopen(fd, "wb") as stream:
            stream.write(data)
            stream.flush()
            os.fsync(stream.fileno())
        temporary.replace(path)
        directory = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    finally:
        temporary.unlink(missing_ok=True)


def bind_private(path: Path, expected: bytes):
    try:
        existing = small_file(path)
    except FileNotFoundError:
        write_private(path, expected)
    else:
        if existing != expected:
            raise ValueError("School sandbox state differs from its configured binding")


@dataclass(frozen=True)
class ProviderProfile:
    workspace_id: str
    state_root: Path
    binaries: Path
    binding: dict
    credentials: Path
    webhook_secret: Path

    def signing_key(self):
        key = small_file(self.webhook_secret).strip()
        if not re.fullmatch(rb"whsec_[A-Za-z0-9_-]{16,240}", key):
            raise ValueError("Invalid private Stripe Sandbox signing secret")
        return key + b"\n"


def profile_path(value) -> Path:
    if not isinstance(value, (str, Path)):
        raise ValueError("Expected a canonical absolute provider profile path")
    path = Path(value)
    if not path.is_absolute() or path.resolve(strict=True) != path:
        raise ValueError("Expected a canonical absolute provider profile path")
    return path


def load_provider_profile(path: Path, ready_root: Path, default_binaries: Path):
    path = profile_path(path)
    private_directory(path.parent)
    spec = read_json(path)
    if set(spec) != {"workspace_id", "state_root", "binaries", "binding", "credentials", "webhook_secret"}:
        raise ValueError("Unexpected school provider profile fields")
    workspace = spec["workspace_id"]
    if not isinstance(workspace, str) or not UUID.fullmatch(workspace):
        raise ValueError("Expected one canonical school workspace ID")
    state_root = profile_path(spec["state_root"])
    private_directory(state_root)
    if state_root.is_relative_to(ready_root) or ready_root.is_relative_to(state_root):
        raise ValueError("School provider demo state and original state must have separate roots")
    require_socket_paths(state_root, provider_demo=True)
    binaries = profile_path(spec["binaries"])
    verify_binaries(binaries)
    if (binaries / "school-backend").samefile(default_binaries / "school-backend"):
        raise ValueError("Expected a dedicated school provider demo binary release")
    config, credentials, secret = (profile_path(spec[key]) for key in ("binding", "credentials", "webhook_secret"))
    for control in (config, credentials, secret):
        private_directory(control.parent)
    os.close(private_file(credentials))
    profile = ProviderProfile(workspace, state_root, binaries, load_provider_binding(config), credentials, secret)
    profile.signing_key()
    return profile


def load_provider_binding(config):
    binding = read_json(config)
    patterns = {
        "sourceAccount": r"acct_[A-Za-z0-9]{8,64}",
        "sourcePrice": r"price_[A-Za-z0-9]{8,64}",
        "destinationWorkspace": r"[1-9][0-9]{0,19}",
        "targetList": r"[1-9][0-9]{0,19}",
        "assignee": r"[1-9][0-9]{0,19}",
    }
    if (set(binding) != set(patterns) | {"label"} or not all(isinstance(value, str) for value in binding.values())
            or not all(re.fullmatch(pattern, binding[key]) for key, pattern in patterns.items())):
        raise ValueError("Invalid school provider demo binding")
    label = binding["label"]
    if not label or label != label.strip() or len(label) > 120 or not label.isprintable():
        raise ValueError("Invalid school provider demo service label")
    return binding


@dataclass(frozen=True)
class Slot:
    number: int
    slug: str
    workspace_id: str
    started_at: float
    retry_at: float
    published: bool
    provider_demo: bool = False

    @property
    def web_port(self):
        return 22000 + 2 * self.number

    @property
    def internal_host(self):
        subnet, address = divmod(self.number - 1, 254)
        return f"127.77.{subnet}.{address + 1}"

    @property
    def origin(self):
        return f"http://127.0.0.1:{self.web_port}"

    @property
    def unit(self):
        return f"hitpipe-school-tenant-{self.slug}.service"

    @property
    def description(self):
        mode = "provider demo" if self.provider_demo else "sandbox"
        return f"HitPipe school {mode} {self.slug}"


def registry_allocations(path: Path, issuer: str):
    private_directory(path.parent)
    try:
        os.close(private_file(path))
    except FileNotFoundError:
        return []
    with closing(sqlite3.connect(path.as_uri() + "?mode=ro", uri=True, timeout=3)) as db:
        if db.execute("SELECT issuer FROM registry_config WHERE singleton = 1").fetchone() != (issuer,):
            raise ValueError("Unexpected issuer for school sandbox registry")
        rows = db.execute("SELECT sandbox_slug, workspace_id FROM school_tenants ORDER BY sandbox_slug LIMIT ?",
                          (MAX_SLOTS + 1,)).fetchall()
    if len(rows) > MAX_SLOTS or any(not SLUG.fullmatch(slug) or not UUID.fullmatch(workspace) for slug, workspace in rows):
        raise ValueError("Invalid allocation in school sandbox registry")
    return rows


class Slots:
    def __init__(self, root: Path):
        path = root / "broker-slots.sqlite3"
        os.close(private_file(path, os.O_RDWR | os.O_CREAT))
        self.db = sqlite3.connect(path, timeout=3)
        self.db.execute("PRAGMA synchronous = EXTRA")
        with self.db:
            self.db.execute(f"""
                CREATE TABLE IF NOT EXISTS slots (
                    number INTEGER PRIMARY KEY CHECK (number BETWEEN 1 AND {MAX_SLOTS}),
                    slug TEXT UNIQUE NOT NULL, workspace_id TEXT UNIQUE NOT NULL,
                    started_at REAL NOT NULL DEFAULT 0, retry_at REAL NOT NULL DEFAULT 0,
                    published INTEGER NOT NULL DEFAULT 0 CHECK (published IN (0, 1))
                )
            """)

    def remember(self, allocations):
        with self.db:
            for slug, workspace in allocations:
                row = self.db.execute("SELECT workspace_id FROM slots WHERE slug = ?", (slug,)).fetchone()
                if row is None:
                    self.db.execute("INSERT INTO slots (slug, workspace_id) VALUES (?, ?)", (slug, workspace))
                elif row != (workspace,):
                    raise ValueError("A school sandbox slot cannot change workspaces")
        return [Slot(*row) for row in self.db.execute("SELECT * FROM slots ORDER BY number")]

    def attempt(self, slot: Slot, now: float):
        with self.db:
            self.db.execute("UPDATE slots SET started_at = ?, retry_at = ?, published = 0 WHERE number = ?",
                            (now, now + RETRY_DELAY, slot.number))

    def publish(self, slot: Slot):
        with self.db:
            self.db.execute("UPDATE slots SET published = 1 WHERE number = ?", (slot.number,))

    def close(self):
        self.db.close()


class UserSystemd:
    def states(self, slots):
        if not slots:
            return {}
        reply = subprocess.run([
            "systemctl", "--user", "show", "--property=Id,Description,Transient,LoadState,ActiveState",
            *(slot.unit for slot in slots),
        ], capture_output=True, text=True, timeout=10)
        units = {}
        for block in reply.stdout.strip().split("\n\n"):
            properties = dict(line.split("=", 1) for line in block.splitlines() if "=" in line)
            if properties.get("Id"):
                units[properties["Id"]] = properties
        states = {}
        for slot in slots:
            properties = units.get(slot.unit)
            if properties is None:
                raise RuntimeError("Unable to distinguish inactive school sandbox units")
            if properties.get("LoadState") == "not-found":
                states[slot.slug] = "inactive"
            elif properties.get("Transient") != "yes" or properties.get("Description") != slot.description:
                states[slot.slug] = "unmanaged"
            else:
                states[slot.slug] = properties["ActiveState"]
        return states

    def start(self, slot: Slot, script: Path, environment: dict):
        subprocess.run([
            "systemd-run", "--user", "--quiet", "--collect", f"--unit={slot.unit}",
            f"--description={slot.description}", "--property=Type=exec", "--property=KillMode=mixed",
            "--property=TimeoutStopSec=80",
            *(f"--setenv={key}={value}" for key, value in environment.items()),
            str(script),
        ], capture_output=True, check=True, timeout=12)

    def stop_unready(self, slot: Slot):
        if self.states([slot]).get(slot.slug) in {"active", "activating"}:
            subprocess.run(["systemctl", "--user", "--no-block", "stop", slot.unit],
                           capture_output=True, check=True, timeout=10)


def get_fixture(port: int, path: str, token: str):
    with closing(http.client.HTTPConnection("127.0.0.1", port, timeout=4)) as client:
        client.request("GET", path, headers={"Accept": "application/json", "Cookie": "__Host-hp_session=" + token})
        response = client.getresponse()
        if response.status != 200 or response.getheader("Content-Type", "").split(";", 1)[0] != "application/json":
            raise ValueError("School sandbox did not admit the private fixture session")
        raw = response.read(FILE_LIMIT + 1)
    if len(raw) > FILE_LIMIT:
        raise ValueError("School sandbox readiness response is too large")
    value = json.loads(raw, object_pairs_hook=unique_fields)
    if not isinstance(value, dict):
        raise ValueError("Invalid school sandbox readiness response")
    return value


def verify_binaries(path: Path):
    private_directory(path)
    with (path / "binaries.sha256").open(encoding="ascii") as source:
        records = re.findall(r"^([a-f0-9]{64}) [ *]([a-z-]+)$", source.read(FILE_LIMIT), flags=re.MULTILINE)
    signatures = {name: signature for signature, name in records}
    if len(records) != len(EXECUTABLES) or set(signatures) != set(EXECUTABLES):
        raise ValueError("Invalid school sandbox binary receipt")
    for name in EXECUTABLES:
        executable = path / name
        info = executable.lstat()
        if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or not os.access(executable, os.X_OK):
            raise ValueError("Expected a verified school sandbox executable")
        with executable.open("rb") as source:
            digest = hashlib.file_digest(source, "sha256").hexdigest()
        if digest != signatures[name]:
            raise ValueError("School sandbox executable differs from its receipt")


class Broker:
    def __init__(self, args, slots, systemd, probe=get_fixture):
        self.config = args
        self.slots = slots
        self.systemd = systemd
        self.probe = probe
        self.attested = {}
        self.script = Path(__file__).resolve().parents[1] / "serve.sh"
        self.backend = self.script.parent / "backend/run-school-backend.sh"

    def sandbox(self, slot: Slot):
        path = self.config.ready_root / slot.slug
        path.mkdir(mode=0o700, exist_ok=True)
        private_directory(path)
        return path

    def provider(self, slot: Slot):
        configured = getattr(self.config, "provider_profile", None)
        if configured and configured.workspace_id == slot.workspace_id:
            return configured
        directory = getattr(self.config, "provider_settings_root", None)
        if directory is None:
            return None
        entry = directory / slot.workspace_id
        try:
            private_directory(entry)
            state = read_json(entry / "state.json")
        except FileNotFoundError:
            return None
        if state.get("workspace") != slot.workspace_id:
            raise ValueError("Provider settings belong to another workspace")
        endpoint = state.get("stripeEndpoint")
        if endpoint is None:
            return None
        if not isinstance(endpoint, str) or not re.fullmatch(r"we_[a-zA-Z0-9]{8,80}", endpoint):
            raise ValueError("Provider settings have no exact Stripe endpoint")
        binding = load_provider_binding(entry / "provider-binding.json")
        if state.get("binding") != binding:
            raise ValueError("Provider settings binding differs")
        credentials = profile_path(entry / "credentials.json")
        secret = profile_path(entry / "provider-webhook.secret")
        os.close(private_file(credentials))
        result = ProviderProfile(slot.workspace_id, self.config.provider_runtime_root,
                                 self.config.provider_binaries, binding, credentials, secret)
        result.signing_key()
        return result

    def instance(self, slot: Slot, state: Path, profile):
        if profile is None:
            return slot.slug, state
        name = "d" + slot.slug[1:]
        return name, profile.state_root / name

    def prepare(self, slot: Slot, state: Path):
        profile = self.provider(slot)
        instance, state = self.instance(slot, state, profile)
        state.mkdir(mode=0o700, exist_ok=True)
        private_directory(state)
        marker = state / "provider-demo.json"
        if profile:
            binding = json.dumps(profile.binding, sort_keys=True, ensure_ascii=True).encode("ascii") + b"\n"
            if not os.path.lexists(marker) and any(state.iterdir()):
                raise ValueError("Expected a fresh school provider demo state directory")
            bind_private(marker, binding)
            bind_private(state / "provider-webhook.secret", profile.signing_key())
        elif os.path.lexists(marker):
            raise ValueError("The default sandbox may not have a provider demo binding")
        config = state / "workspace-id"
        bind_private(config, slot.workspace_id.encode("ascii") + b"\n")
        target = state / "bin"
        target.mkdir(mode=0o700, exist_ok=True)
        private_directory(target)
        binaries = profile.binaries if profile else self.config.binaries
        for name in BINARIES:
            original = binaries / name
            staged = target / name
            try:
                os.link(original, staged, follow_symlinks=False)
            except FileExistsError:
                before, after = original.lstat(), staged.lstat()
                if not os.path.samestat(before, after) or not stat.S_ISREG(after.st_mode):
                    raise ValueError("Sandbox binaries differ from the staged release") from None
        environment = {
            "HITPIPE_SCHOOL_STATE": state, "HITPIPE_SCHOOL_WORKSPACE_FILE": config,
            "HITPIPE_SCHOOL_INSTANCE": instance, "HITPIPE_SCHOOL_INTERNAL_HOST": slot.internal_host,
            "HITPIPE_BROWSER_PORT": slot.web_port, "HITPIPE_SCHOOL_ENTRY_PORT": slot.web_port + 1,
            "HITPIPE_SCHOOL_FRONTEND": self.config.frontend, "HITPIPE_NODE": self.config.node,
            "HITPIPE_GO": self.config.go, "HITPIPE_SCHOOL_BACKEND": self.backend,
            "HITPIPE_DEMO_SOURCE": self.config.sources, "HITPIPE_SCHOOL_REUSE_BINARIES": 1,
            "HITPIPE_SCHOOL_AI": int(profile is None), "HITPIPE_SCHOOL_STRIPE_EVENTS": int(profile is None),
            "HITPIPE_SCHOOL_MANAGED": 1,
        }
        if profile:
            environment["HITPIPE_PROVIDER_DEMO_CREDENTIALS"] = profile.credentials
        return environment

    def revoke(self, slot: Slot, state: Path):
        (state / READY).unlink(missing_ok=True)
        self.attested.pop(slot.slug, None)

    def attest(self, slot: Slot, state: Path):
        profile = self.provider(slot)
        _, runtime_state = self.instance(slot, state, profile)
        private_directory(runtime_state)
        current = runtime_state / "current"
        if not current.is_symlink():
            raise ValueError("School sandbox has no current runtime")
        run = current.resolve(strict=True)
        if run.parent != runtime_state or not RUNTIME.fullmatch(run.name):
            raise ValueError("School sandbox runtime escaped its state")
        private_directory(run)
        if self.attested.get(slot.slug) == run and (state / READY).is_file():
            return
        self.revoke(slot, state)
        web = read_json(run / "web-ready.json")
        bridge = run / "bridge"
        private_directory(bridge)
        raw = read_json(bridge / "ready.json")
        browser = read_json(bridge / "browser-config.json")
        token, csrf = raw["sessionToken"], raw["csrfToken"]
        synthetic_payment = profile is None
        if (web.get("mode") != "school-fixture" or web.get("origin") != slot.origin
                or "entryUrl" in web or "proofUrl" in web
                or raw.get("workspaceId") != slot.workspace_id
                or browser.get("origin") != slot.origin or browser.get("workspaceId") != slot.workspace_id
                or browser.get("sessionToken") != token or browser.get("schoolStripeEvents") is not synthetic_payment
                or not TOKEN.fullmatch(token) or not CSRF.fullmatch(csrf)):
            raise ValueError("Child readiness differs from the assigned school sandbox")
        if profile is not None:
            if raw.get("providerDemo") != profile.binding or browser.get("schoolAi") is not False:
                raise ValueError("Child readiness differs from the school's pinned provider binding")
        elif "providerDemo" in raw:
            raise ValueError("Unexpected provider demo in the default school sandbox")
        session = self.probe(slot.web_port, "/api/v1/session", token)
        profile = session["profile"]
        if (not UUID.fullmatch(raw["principalId"]) or not UUID.fullmatch(raw["sessionId"])
                or session["user_id"] != raw["principalId"] or profile["user_id"] != raw["principalId"]
                or session["session_id"] != raw["sessionId"] or not compare_digest(session["csrf_token"], csrf)):
            raise ValueError("Active fixture session differs from child readiness")
        workspaces = self.probe(slot.web_port, "/api/v1/workspaces", token)["items"]
        if (not isinstance(workspaces, list) or len(workspaces) != 1
                or workspaces[0].get("workspace_id") != slot.workspace_id or workspaces[0].get("role") != "OWNER"):
            raise ValueError("Active fixture session does not own the assigned school workspace")
        if current.resolve(strict=True) != run:
            raise ValueError("School sandbox runtime changed while checking readiness")
        write_private(state / READY, json.dumps({
            "sandbox_slug": slot.slug, "workspace_id": slot.workspace_id, "origin": slot.origin,
            "session_token": token, "csrf_token": csrf,
        }, separators=(",", ":")).encode("ascii") + b"\n")
        self.slots.publish(slot)
        self.attested[slot.slug] = run

    def reconcile(self):
        allocations = registry_allocations(self.config.registry, self.config.issuer)
        allocated = {slug for slug, _ in allocations}
        slots = []
        invalid = set()
        for slot in self.slots.remember(allocations):
            try:
                slots.append(replace(slot, provider_demo=True) if self.provider(slot) else slot)
            except (OSError, ValueError, KeyError, TypeError):
                invalid.add(slot.slug)
                slots.append(slot)
        states = self.systemd.states(slots)
        now = time.time()
        running = 0
        provisioning = False
        candidates = []
        for slot in slots:
            status = states[slot.slug]
            if status not in ABSENT_STATES:
                running += 1
            try:
                state = self.sandbox(slot)
                if slot.slug in invalid:
                    self.revoke(slot, state)
                    continue
                if status == "unmanaged" and slot.provider_demo and getattr(self.config, "provider_settings_root", None):
                    prior = replace(slot, provider_demo=False)
                    if self.systemd.states([prior]).get(slot.slug) in {"active", "activating"}:
                        self.revoke(slot, state)
                        self.systemd.stop_unready(prior)
                        continue
                if status in ABSENT_STATES:
                    self.revoke(slot, state)
                    if slot.slug in allocated and now >= slot.retry_at:
                        candidates.append((slot, state))
                    continue
                if status == "unmanaged" or slot.slug not in allocated:
                    self.revoke(slot, state)
                    continue
                if status == "active":
                    try:
                        self.attest(slot, state)
                        continue
                    except (OSError, ValueError, KeyError, TypeError, AttributeError, http.client.HTTPException):
                        self.revoke(slot, state)
                else:
                    self.revoke(slot, state)
            except (OSError, ValueError) as exc:
                print(f"School sandbox {slot.slug} unavailable ({type(exc).__name__})", file=sys.stderr)
            if not slot.published:
                provisioning = True
                if status in {"active", "activating"} and now >= slot.started_at + START_TIMEOUT:
                    self.systemd.stop_unready(slot)
        if running < self.config.max_running and not provisioning and candidates:
            slot, state = min(candidates, key=lambda candidate: (not candidate[0].provider_demo, candidate[0].started_at))
            self.slots.attempt(slot, now)
            try:
                self.systemd.start(slot, self.script, self.prepare(slot, state))
            except (OSError, ValueError, subprocess.SubprocessError) as exc:
                print(f"School sandbox {slot.slug} could not start ({type(exc).__name__})", file=sys.stderr)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("registry", "ready-root", "binaries", "frontend", "node", "go", "sources"):
        parser.add_argument(f"--{name}", type=Path, required=True)
    parser.add_argument("--issuer", required=True)
    parser.add_argument("--max-running", type=int, choices=range(1, 5), default=2)
    parser.add_argument("--provider-profile", type=Path, help="Owner-only profile for one registered workspace")
    parser.add_argument("--provider-settings-root", type=Path)
    parser.add_argument("--provider-runtime-root", type=Path)
    parser.add_argument("--provider-binaries", type=Path)
    parser.add_argument("--once", action="store_true")
    config = parser.parse_args()
    paths = (config.registry, config.ready_root, config.binaries, config.frontend, config.node, config.go, config.sources)
    if not all(path.is_absolute() for path in paths):
        parser.error("File arguments must be absolute")
    try:
        require_socket_paths(config.ready_root)
    except ValueError as exc:
        parser.error(str(exc))
    if (not (config.frontend / ".next/BUILD_ID").is_file() or not config.sources.is_dir()
            or not all(path.is_file() and os.access(path, os.X_OK) for path in (config.node, config.go))):
        parser.error("Expected the built frontend, pinned source archive and Node/Go executables")
    os.umask(0o077)
    config.ready_root.mkdir(mode=0o700, exist_ok=True)
    private_directory(config.ready_root)
    if config.ready_root.resolve() != config.ready_root:
        parser.error("Ready root must be canonical")
    verify_binaries(config.binaries)
    dynamic = (config.provider_settings_root, config.provider_runtime_root, config.provider_binaries)
    if any(dynamic):
        try:
            if not all(dynamic) or config.provider_profile is not None:
                raise ValueError("One provider setup mode is required")
            for directory in dynamic:
                private_directory(profile_path(directory))
            if config.provider_runtime_root.is_relative_to(config.ready_root) or config.ready_root.is_relative_to(config.provider_runtime_root):
                raise ValueError("Provider runtime and school sandboxes must remain separate")
            require_socket_paths(config.provider_runtime_root, provider_demo=True)
            verify_binaries(config.provider_binaries)
            if (config.provider_binaries / "school-backend").samefile(config.binaries / "school-backend"):
                raise ValueError("A separate external provider build is required")
        except (OSError, ValueError):
            parser.error("The self-service provider paths or release are invalid")
    if config.provider_profile is not None:
        try:
            config.provider_profile = load_provider_profile(config.provider_profile, config.ready_root, config.binaries)
        except (OSError, ValueError):
            parser.error("The owner-only provider profile, state root or release is invalid")
    with os.fdopen(private_file(config.ready_root / ".broker.lock", os.O_RDWR | os.O_CREAT), "wb") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            parser.error("A school sandbox broker already owns this ready root")
        with closing(Slots(config.ready_root)) as slots:
            broker = Broker(config, slots, UserSystemd())
            while True:
                try:
                    broker.reconcile()
                except (OSError, ValueError, RuntimeError, sqlite3.Error, subprocess.SubprocessError) as exc:
                    print(f"School sandbox reconciliation failed ({type(exc).__name__})", file=sys.stderr)
                    if config.once:
                        return 1
                if config.once:
                    return 0
                time.sleep(2)


if __name__ == "__main__":
    raise SystemExit(main())
