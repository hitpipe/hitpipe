#!/usr/bin/env python3
"""Reproduce the Processing/IAM startup boundary on the owned local stand."""
import argparse
import hashlib
import json
import pathlib
import tempfile
import uuid

import verify

IAM_HEAD = "e1be0c1a0be577a3626a3e120707b3aeeb579c6f"
IAM_SCHEMA = "604d693ff49ccde71755d2b7afccaa0e9b96f03fef2d2c0e420c08c7ab59bef7"


class Run(verify.Run):
    def __init__(self, output, go, iam, scratch):
        super().__init__(output)
        self.name = "hit146-" + uuid.uuid4().hex
        self.db = "hit78"
        self.go, self.iam, self.scratch = str(go), iam, scratch
        self.iam_module = self.iam / "services/iam"
        self.migrations = self.iam_module / "internal/adapters/postgres/migrations"
        self.script_sha256 = hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()

    def command(self, label, args, *rest, **kwargs):
        if label == "container-create":
            args = list(args)
            args[2:2] = [
                "--mount", f"type=bind,source={self.scratch},target=/test,readonly",
                "--mount", f"type=bind,source={self.migrations},target=/iam-migration,readonly",
            ]
            args[args.index("hitpipe.task=HIT-76")] = "hitpipe.task=HIT-146"
        return super().command(label, args, *rest, **kwargs)

    def build(self, architecture):
        if self.command("iam-head", ["git", "-C", str(self.iam), "rev-parse", "HEAD"]).stdout.decode().strip() != IAM_HEAD:
            raise RuntimeError("use the pinned IAM checkout")
        if self.command("iam-clean", ["git", "-C", str(self.iam), "status", "--porcelain"]).stdout:
            raise RuntimeError("use a clean IAM checkout")
        if hashlib.sha256((self.migrations / "schema/001_iam.sql").read_bytes()).hexdigest() != IAM_SCHEMA:
            raise RuntimeError("IAM migration differs from the pinned formal SQL")
        go = ["env", "GOTOOLCHAIN=local", "GOWORK=off", self.go]
        if self.command("go-version", [*go, "version"]).stdout.split()[2] != b"go1.27.2":
            raise RuntimeError("Go 1.27.2 required")
        processing = [*go, "-C", str(verify.MODULE)]
        self.command("go-race", [*processing, "test", "-race", "-count=1", "./..."])
        self.command("go-vet", [*processing, "vet", "./..."])
        linux = ["env", "GOOS=linux", "GOARCH=" + architecture, "CGO_ENABLED=0", *go]
        binaries = self.scratch
        self.command("build-processing", [*linux, "-C", str(verify.MODULE), "build", "-trimpath",
            "-ldflags", "-X main.revision=" + self.source, "-o", str(binaries / "processing"), "./cmd/processing"])
        self.command("build-processing-tests", [*linux, "-C", str(verify.MODULE), "test", "-c",
            "-o", str(binaries / "storage.test"), "./internal/adapters/postgres"])
        # Only the fixture main is overlaid. The IAM checkout and all imported
        # IAM implementation files remain the pinned originals.
        probe = verify.REPO / "tools/hit146/iam-process.go"
        overlay = binaries / "iam.probe.overlay.json"
        overlay.write_text(json.dumps({"Replace": {
            str(self.iam_module / "cmd/iam-contractcheck/main.go"): str(probe),
        }}) + "\n")
        self.command("build-iam-probe", [*linux, "-C", str(self.iam_module), "build", "-trimpath", "-tags", "hit146_iam_fixture",
            "-overlay", str(overlay), "-o", str(binaries / "iam-probe"), "./cmd/iam-contractcheck"])
        self.command("build-iam-migrate", [*linux, "-C", str(self.iam_module), "build", "-trimpath",
            "-o", str(binaries / "iam-migrate"), "./cmd/iam-migrate"])
        (self.output / "binaries.json").write_text(json.dumps({
            "processing_sha": self.source, "iam_sha": IAM_HEAD, "platform": "linux/" + architecture,
            "fixture_main_sha256": hashlib.sha256(probe.read_bytes()).hexdigest(),
            "binaries": {file.name: hashlib.sha256(file.read_bytes()).hexdigest()
                for file in binaries.iterdir() if file != overlay},
        }, indent=2) + "\n")

    def checks(self):
        arch = self.command("container-arch", ["docker", "exec", self.container, "uname", "-m"]).stdout.decode().strip()
        self.build({"aarch64": "arm64", "x86_64": "amd64"}[arch])
        self.sql("iam-database", "CREATE DATABASE hit146_iam; REVOKE ALL ON DATABASE hit146_iam FROM PUBLIC;",
                 role="postgres", database="postgres")
        self.sql("iam-roles", "\\i /iam-migration/bootstrap_roles.sql\n", role="postgres", database="hit146_iam")
        self.sql("iam-logins",
                 "CREATE ROLE hp_hit146_iam_app LOGIN INHERIT; CREATE ROLE hp_hit146_iam_migrate LOGIN INHERIT;"
                 "GRANT hp_iam_app TO hp_hit146_iam_app; GRANT hp_iam_migrator TO hp_hit146_iam_migrate;"
                 "GRANT CONNECT ON DATABASE hit146_iam TO hp_hit146_iam_app,hp_hit146_iam_migrate;",
                 role="postgres", database="hit146_iam")
        self.command("iam-migration", ["docker", "exec", "-e",
            "HITPIPE_IAM_MIGRATOR_DSN=host=/var/run/postgresql user=hp_hit146_iam_migrate"
            " dbname=hit146_iam sslmode=disable options='-c role=hp_iam_migrator'",
            self.container, "/test/iam-migrate"])
        self.command("real-iam-boundary", ["docker", "exec", "-e", "HIT78_ISOLATED_PG=1",
            "-e", "HIT111_PROCESSING_BIN=/test/processing", "-e", "HIT146_IAM_BIN=/test/iam-probe",
            self.container, "/test/storage.test", "-test.v", "-test.count=1", "-test.timeout=50s",
            "-test.run=^TestRealIAMRegistrationBoundary$"])
        self.command("processing-predecessor", ["docker", "exec", "-e", "HIT78_ISOLATED_PG=1",
            "-e", "HIT111_PROCESSING_BIN=/test/processing", self.container, "/test/storage.test",
            "-test.v", "-test.count=1", "-test.timeout=50s", "-test.run=^TestProcessingExecutableOwnedStand$"])

    def main(self):
        if self.tree_status:
            raise RuntimeError("use a clean Processing checkout")
        super().main()
        path = self.output / "summary.json"
        result = json.loads(path.read_text())
        result.update(status="PASS", iam_source_sha=IAM_HEAD,
                      scope="Real IAM startup, two authenticated control ACKs and missing-ACK deadline; controlled HIT-111 regression.")
        path.write_text(json.dumps(result, indent=2) + "\n")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    for argument in ("go", "iam", "output"):
        parser.add_argument("--" + argument, type=pathlib.Path, required=True)
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="hit146-build-") as scratch:
        Run(args.output.resolve(), args.go.resolve(), args.iam.resolve(), pathlib.Path(scratch)).main()
