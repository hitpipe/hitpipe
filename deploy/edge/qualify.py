#!/usr/bin/env python3
"""Qualify the pinned edge using only synthetic peers on two isolated networks."""
import argparse
import hashlib
import json
import os
import platform
import re
import subprocess
import tarfile
import tempfile
import time
import uuid
from pathlib import Path

from test_pki import generate

ROOT = Path(__file__).resolve().parents[2]
PIN_ROOT = ROOT / "docs/labs/pr12/upstream/architecture/formal"
PIN = PIN_ROOT / "stack/edge-pin.json"
TEMPLATE = PIN_ROOT / "platform/Caddyfile.template"
TEMPLATE_SHA = "44171d205e928177b150a4c0a71ba630843771a10f1ab11fa682e7111087799c"
ARCHIVE_SHA = "727b91701a392de6ebc5027509f548bf39979e5216340d0faed8fa5e69c84f8b"
DOCKER = ["docker", "--host", "unix:///var/run/docker.sock"]
EDGE_ENV = {
    "HP_PUBLIC_HOST": "app.edge.test",
    "HP_IDP_HOST": "idp.edge.test",
    "HP_PUBLIC_CERT": "/tls/public.crt",
    "HP_PUBLIC_KEY": "/tls/public.key",
    "HP_IDP_CERT": "/tls/idp.crt",
    "HP_IDP_KEY": "/tls/idp.key",
    "HP_EDGE_CERT": "/tls/edge.crt",
    "HP_EDGE_KEY": "/tls/edge.key",
    "HP_SERVICE_CA": "/tls/service-ca.crt",
    "XDG_DATA_HOME": "/runtime/data",
    "XDG_CONFIG_HOME": "/runtime/config",
}
PHASES = ("baseline", "wrong-san", "untrusted-ca")


class TestError(RuntimeError):
    pass


def want(ok, message):
    if not ok:
        raise TestError(message)


def sha256(path):
    with path.open("rb") as handle:
        return hashlib.file_digest(handle, "sha256").hexdigest()


def write_json(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, sort_keys=True, indent=2) + "\n", encoding="utf-8")


class Qualification:
    def __init__(self, tmp, evidence, archive):
        self.tmp, self.evidence, self.archive = tmp, evidence, archive
        self.id = uuid.uuid4().hex[:14]
        self.prefix = "hit141-" + self.id
        self.label = "hitpipe.edge-qualification=" + self.id
        self.front = self.prefix + "-front"
        self.back = self.prefix + "-back"
        self.image = self.prefix + ":local"
        self.creds = tmp / "pki"
        self.commands = []
        self.log = {
            "task": "HIT-141",
            "state": "FAILED",
            "scope": "Isolated synthetic topology, route/header and transport checks; no live endpoints or data.",
            "phases": {},
            "topology": {},
        }
        self.docker_used = False
        self.diagnostics = {}
        self.child_env = dict(os.environ)
        self.child_env.update(DOCKER_CONFIG=str(tmp / "docker-config"), DOCKER_HOST="")
        for key in ("DOCKER_CONTEXT", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH"):
            self.child_env.pop(key, None)

    def run(self, label, argv, *, cwd=ROOT, env=None, timeout=120, allowed=(0,)):
        # Record stages/codes, not subprocess stdout or request headers.
        try:
            p = subprocess.run(argv, cwd=cwd, env=self.child_env if env is None else env,
                               stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=timeout, check=False)
        except (OSError, subprocess.TimeoutExpired) as exc:
            self.commands.append({"stage": label, "error": type(exc).__name__})
            raise TestError(label + " did not finish") from None
        step = {"stage": label, "exit": p.returncode}
        if not label.startswith("wait for"):
            # All arguments are test commands, paths, or isolated fixture
            # addresses. Keys and HTTP request bodies never appear in argv.
            step["argv"] = argv
        self.commands.append(step)
        if p.returncode not in allowed:
            raise TestError(label + " failed (exit " + str(p.returncode) + ")")
        return p

    def docker(self, label, *args, **kwargs):
        return self.run(label, DOCKER + list(args), **kwargs)

    def prepare(self):
        want(platform.system() == "Linux" and platform.machine() == "x86_64", "pinned archive requires Linux amd64")
        want(os.getuid() != 0, "run as the unprivileged owner of the isolated lab directory")
        edge_pin = json.loads(PIN.read_text(encoding="utf-8"))
        want(edge_pin["version"] == "2.11.7" and edge_pin["plugins"] == [] and edge_pin["sha256"] == ARCHIVE_SHA,
             "substituted architectural binary pin")
        want(sha256(TEMPLATE) == TEMPLATE_SHA, "the checked-in mirror differs from the accepted Caddyfile")
        want(sha256(self.archive) == ARCHIVE_SHA, "official Caddy archive digest differs")
        self.log.update(
            source_commit=self.run("git exact source", ["git", "rev-parse", "HEAD"]).stdout.decode().strip(),
            template={"archive_mirror": str(TEMPLATE.relative_to(ROOT)), "sha256": TEMPLATE_SHA},
            caddy={"archive": edge_pin["official_archive"], "archive_sha256": ARCHIVE_SHA},
        )
        rootfs = self.tmp / "rootfs"
        rootfs.mkdir(mode=0o700)
        with tarfile.open(self.archive, "r:gz") as tar:
            members = [member for member in tar.getmembers() if member.name == "caddy"]
            want(len(members) == 1 and members[0].isfile() and 0 < members[0].size < 128 << 20,
                 "unexpected layout of official archive")
            with tar.extractfile(members[0]) as binary:
                (rootfs / "caddy").write_bytes(binary.read())
        (rootfs / "caddy").chmod(0o755)
        self.log["caddy"]["binary_sha256"] = sha256(rootfs / "caddy")
        env = dict(self.child_env, CGO_ENABLED="0", GO111MODULE="off", GOTOOLCHAIN="local",
                   GOENV="off", GOWORK="off", GOPROXY="off", GOSUMDB="off",
                   GOCACHE=str(self.tmp / "go-cache"))
        go = self.run("go version", ["go", "version"], env=env).stdout.decode().strip()
        want(go == "go version go1.27.2 linux/amd64", "qualification requires pinned Go 1.27.2")
        self.log["toolchain"] = go
        for tool in ("fixture", "probe"):
            pkg = "./deploy/edge/" + tool
            self.run(tool + " vet", ["go", "vet", pkg], env=env)
            self.run(tool + " race tests", ["go", "test", "-race", "-count=1", pkg],
                     env=dict(env, CGO_ENABLED="1"))
            self.run(tool + " isolated static build", ["go", "build", "-trimpath", "-o", str(rootfs / tool), pkg], env=env)
        self.creds.mkdir(mode=0o700)
        generate(self.creds)
        root_tar = self.tmp / "rootfs.tar"
        with tarfile.open(root_tar, "w") as tar:
            for tool in ("caddy", "fixture", "probe"):
                tar.add(rootfs / tool, arcname=tool)
        self.docker_used = True
        self.docker("import scratch image without registry", "image", "import", "--change",
                    "LABEL " + self.label, str(root_tar), self.image)
        for network in (self.front, self.back):
            self.docker("create internal bridge", "network", "create", "--driver", "bridge",
                        "--internal", "--label", self.label, network)
            inspected = json.loads(self.docker("inspect bridge", "network", "inspect", network).stdout)[0]
            want(inspected["Internal"] is True and inspected["Driver"] == "bridge", "test network isn't isolated")
        self.log["topology"]["networks"] = {"front": "internal bridge", "private": "internal bridge"}

    def container_options(self, network):
        return ["--network", network, "--label", self.label, "--read-only", "--cap-drop", "ALL",
                "--security-opt", "no-new-privileges", "--pids-limit", "64",
                "--user", f"{os.getuid()}:{os.getgid()}"]

    @staticmethod
    def mount(source, target):
        return ["--mount", f"type=bind,src={source},dst={target},readonly"]

    def caddy_options(self, network):
        opts = self.container_options(network)
        opts += ["--tmpfs", f"/runtime:rw,nosuid,nodev,noexec,mode=0700,uid={os.getuid()},gid={os.getgid()}"]
        opts += self.mount(TEMPLATE, "/Caddyfile") + self.mount(self.creds / "edge", "/tls")
        for key, val in EDGE_ENV.items():
            opts += ["--env", key + "=" + val]
        return opts

    def assess_pin(self):
        # No public or private network required for local version/adapt/validate.
        opts = self.caddy_options("none") + ["--entrypoint", "/caddy", self.image]
        p = self.docker("caddy version", "run", "--rm", *opts, "version")
        version = p.stdout.decode().strip()
        want(re.fullmatch(r"v2\.11\.7(?: [^\n]*)?", version), "Caddy executable is not v2.11.7")
        self.log["caddy"]["version"] = version
        m = self.docker("caddy list-modules --skip-standard", "run", "--rm", *opts,
                        "list-modules", "--skip-standard").stdout.decode()
        want(re.search(r"(?im)^non-standard modules:\s*0\s*$", m) and
             re.search(r"(?im)^unknown modules:\s*0\s*$", m), "unexpected extension modules in pinned executable")
        self.log["caddy"]["non_standard_modules"] = 0
        self.log["caddy"]["unknown_modules"] = 0
        adapted = self.docker("caddy adapt", "run", "--rm", *opts, "adapt", "--config",
                              "/Caddyfile", "--adapter", "caddyfile").stdout
        want(json.loads(adapted)["admin"]["disabled"] is True, "Caddy admin listener was not disabled")
        self.log["template"]["adapted_sha256"] = hashlib.sha256(adapted).hexdigest()
        self.log["template"]["admin_disabled"] = True
        self.docker("caddy validate", "run", "--rm", *opts, "validate", "--config",
                    "/Caddyfile", "--adapter", "caddyfile")

    def inspect(self, name, memberships, key):
        value = json.loads(self.docker("inspect actual container topology", "inspect", name).stdout)[0]
        host = value["HostConfig"]
        networks = value["NetworkSettings"]["Networks"]
        want(set(networks) == set(memberships) and not host["PortBindings"] and
             not host["PublishAllPorts"] and not host["Privileged"] and host["ReadonlyRootfs"],
             "container topology or sandbox differed for " + key)
        self.log["topology"][key] = {
            "attachments": sorted("front" if n == self.front else "private" for n in networks),
            "published_ports": 0, "privileged": False,
        }
        return networks

    def wait_fixture(self, name):
        for _ in range(80):
            p = self.docker("wait for synthetic listener", "logs", name, allowed=(0, 1))
            if b"fixture ready" in p.stdout or b"fixture ready" in p.stderr:
                return
            time.sleep(0.1)
        raise TestError("synthetic listener never became ready")

    def start_fixture(self, role, variant=None):
        name = self.prefix + "-" + role
        opts = self.container_options(self.back) + ["--name", name, "--network-alias", role]
        args = ["-role", role]
        if role != "private":
            opts += self.mount(self.creds / (variant or role), "/tls")
            args += ["-cert", "/tls/server.crt", "-key", "/tls/server.key", "-ca", "/tls/service-ca.crt"]
        self.docker("start synthetic " + role, "run", "-d", *opts, "--entrypoint",
                    "/fixture", self.image, *args)
        self.wait_fixture(name)
        return self.inspect(name, (self.back,), variant or role)[self.back]["IPAddress"]

    def start_edge(self, phase):
        name = self.prefix + "-edge"
        opts = self.caddy_options(self.front) + ["--name", name, "--network-alias", "edge",
                "--sysctl", "net.ipv4.ip_unprivileged_port_start=0"]
        self.docker("start unmodified Caddyfile " + phase, "run", "-d", *opts, "--entrypoint",
                    "/caddy", self.image, "run", "--config", "/Caddyfile", "--adapter", "caddyfile")
        self.docker("attach Caddy to private bridge", "network", "connect", self.back, name)
        self.inspect(name, (self.front, self.back), "caddy-" + phase)
        for _ in range(80):
            p = self.docker("wait for Caddy listener", "logs", name, allowed=(0, 1))
            if b"serving initial configuration" in p.stdout or b"serving initial configuration" in p.stderr:
                return
            time.sleep(0.1)
        raise TestError("Caddy listener never became ready")

    def probe(self, phase, network, key, private, iam):
        name = self.prefix + "-probe-" + key
        opts = self.container_options(network) + ["--name", name]
        args = ["-phase", phase, "-private-ip", private, "-iam-ip", iam]
        if network == self.front:
            opts += self.mount(self.creds / "client", "/tls")
            args += ["-edge", "edge:443", "-ca", "/tls/public-ca.crt"]
        self.docker("create probe " + key, "create", *opts, "--entrypoint", "/probe", self.image, *args)
        self.inspect(name, (network,), "probe-" + key)
        p = self.docker("run probe " + key, "start", "-a", name, allowed=(0, 1, 2))
        try:
            report = json.loads(p.stdout)
        except ValueError:
            raise TestError("probe " + key + " did not produce structured evidence") from None
        self.log["phases"][key] = report
        want(report.get("phase") == phase and report.get("passed") is True and p.returncode == 0,
             "probe " + key + " assertions did not pass")
        self.docker("remove probe " + key, "rm", name)

    def negative_cause(self, phase):
        # The direct backend TCP positive control plus BFF continuity excludes
        # a routing or fixture outage. Confirm the particular x509 diagnostic
        # without saving Caddy's request metadata or the synthetic values.
        p = self.docker("classify Caddy upstream failure", "logs", self.prefix + "-edge")
        log = p.stdout + p.stderr
        patterns = {
            "wrong-san": rb"x509: certificate is valid for [^,\r\n]+, not iam",
            "untrusted-ca": rb"x509: certificate signed by unknown authority",
        }
        self.diagnostics[phase] = bool(re.search(patterns[phase], log))
        want(self.diagnostics[phase], "missing expected x509 failure for " + phase)

    def execute(self):
        self.prepare()
        self.assess_pin()
        private = self.start_fixture("private")
        for role in ("bff", "intake", "keycloak"):
            self.start_fixture(role)
        iam = self.start_fixture("iam")
        self.probe("private-positive", self.back, "private-baseline", private, iam)
        for phase in PHASES:
            if phase != "baseline":
                self.docker("replace only IAM synthetic listener", "rm", "-f", self.prefix + "-iam")
                iam = self.start_fixture("iam", "iam-" + phase)
                self.probe("private-positive", self.back, "private-" + phase, private, iam)
            self.start_edge(phase)
            self.probe(phase, self.front, phase, private, iam)
            if phase != "baseline":
                self.negative_cause(phase)
            self.docker("remove phase Caddy and cached outbound connections", "rm", "-f", self.prefix + "-edge")
        self.log["rejected_tls_diagnostics"] = self.diagnostics
        self.log["state"] = "PASSED"

    def matching(self, kind):
        cmd = {"containers": ("ps", "-aq"), "networks": ("network", "ls", "-q"),
               "images": ("image", "ls", "-q")}[kind]
        return set(self.docker("inventory experiment " + kind, *cmd, "--filter",
                               "label=" + self.label).stdout.decode().split())

    def cleanup(self):
        if not self.docker_used:
            return
        for kind, command in (("containers", ("rm", "-f")), ("networks", ("network", "rm")), ("images", ("image", "rm"))):
            ids = sorted(self.matching(kind))
            if ids:
                self.docker("cleanup labelled " + kind, *command, *ids)
        counts = {kind: len(self.matching(kind)) for kind in ("containers", "networks", "images")}
        self.log["cleanup_remaining"] = counts
        want(not any(counts.values()), "experiment resources were not removed")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--isolated-test", required=True, action="store_true",
                        help="confirm this is a disposable TEST Docker host, never a live node")
    parser.add_argument("--archive", required=True, type=Path, help="exact official Caddy archive")
    parser.add_argument("--evidence-dir", required=True, type=Path)
    args = parser.parse_args()
    os.umask(0o077)
    out = args.evidence_dir.resolve()
    out.mkdir(mode=0o700, parents=True, exist_ok=True)
    want(not any(out.iterdir()), "evidence directory must be empty")
    with tempfile.TemporaryDirectory(prefix="hit141-edge-") as tmp:
        q = Qualification(Path(tmp), out, args.archive.resolve())
        try:
            q.execute()
        except (TestError, OSError, ValueError, KeyError, RuntimeError) as exc:
            q.log["failure"] = str(exc) if isinstance(exc, TestError) else type(exc).__name__
        finally:
            try:
                q.cleanup()
            except TestError as exc:
                q.log.update(state="FAILED", cleanup_failure=str(exc))
            q.log["commands"] = q.commands
            write_json(out / "verification.json", q.log)
        print("HIT-141 " + q.log["state"])
        if q.log["state"] != "PASSED":
            print(q.log.get("failure") or q.log.get("cleanup_failure") or "test did not complete")
            return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
