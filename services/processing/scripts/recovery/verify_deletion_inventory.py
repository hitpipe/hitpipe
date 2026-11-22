#!/usr/bin/env python3
"""Qualify read-only HIT-154 integration and Processing recovery regressions."""
import argparse
import hashlib
import json
import pathlib
import tempfile

import verify_recovery

IAM_SOURCE = "c715a079ab57d4c9e7b56862f7f2584c792b74b0"
IAM_SQL = "604d693ff49ccde71755d2b7afccaa0e9b96f03fef2d2c0e420c08c7ab59bef7"


class Run(verify_recovery.Run):
    def __init__(self, output, go, binaries, iam):
        super().__init__(output, go, binaries)
        self.name = self.name.replace("hit128-", "hit148-", 1)
        self.iam = iam
        self.module = iam / "services/iam"
        self.migrations = self.module / "internal/adapters/postgres/migrations"
        self.script_sha256 = hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()

    def command(self, label, argv, *args, **kwargs):
        if label == "container-create":
            argv = ["hitpipe.task=HIT-148" if a == "hitpipe.task=HIT-76" else a for a in argv]
            argv[2:2] = ["--mount", f"type=bind,source={self.migrations},target=/iam-migration,readonly"]
        return super().command(label, argv, *args, **kwargs)

    def checks(self):
        version = self.command("iam-sha", ["git", "-C", str(self.iam), "rev-parse", "HEAD"])
        clean = self.command("iam-clean", ["git", "-C", str(self.iam), "status", "--porcelain"])
        if version.stdout.decode().strip() != IAM_SOURCE or clean.stdout:
            raise RuntimeError("use the exact clean HIT-154 checkout")
        sql = self.migrations / "schema/001_iam.sql"
        if hashlib.sha256(sql.read_bytes()).hexdigest() != IAM_SQL:
            raise RuntimeError("HIT-154 IAM schema differs from accepted bytes")
        super().checks()
        binaries = json.loads((self.output / "binaries.json").read_text())
        cpu = binaries["platform"].split("/")[1]
        go = ["env", "GOOS=linux", "GOARCH="+cpu, "CGO_ENABLED=0", "GOTOOLCHAIN=local",
              "GOWORK=off", self.go, "-C", str(self.module)]
        pkg = self.module / "internal/adapters/composition/casedeletion"
        fixture = pkg / "fixture_integration_test.go"
        probe = verify_recovery.verify.REPO / "tools/hit148/inventory-probe.go"
        with tempfile.TemporaryDirectory(prefix="hit148-overlay-") as temp:
            patched = pathlib.Path(temp) / fixture.name
            overlay = pathlib.Path(temp) / "overlay.json"
            text = fixture.read_text()
            for old, new in [
                ('c.Host != "127.0.0.1"', 'c.Host != "/var/run/postgresql"'),
                ('"SHOW server_version"', '"SHOW server_version_num"'),
                ('version != "18.7"', 'version != "180007"'),
                ('type labTLS struct {', 'type labTLS struct {\n\tcaDER []byte'),
                ('ca: ca, publish:', 'ca: ca, caDER: der, publish:'),
            ]:
                if text.count(old) != 1:
                    raise RuntimeError("unexpected pinned test fixture")
                text = text.replace(old, new)
            patched.write_text(text)
            overlay.write_text(json.dumps({"Replace": {
                str(fixture): str(patched),
                str(pkg / "processing_inventory_test.go"): str(probe),
            }}))
            self.command("build-iam-deletion-interop", [*go, "test", "-c", "-tags", "integration,hit148_probe",
                         "-overlay", str(overlay), "-o", str(self.binaries / "iam-deletion.test"),
                         "./internal/adapters/composition/casedeletion"])
            self.command("build-iam-migrations", [*go, "build", "-trimpath",
                         "-o", str(self.binaries / "iam-migrate"), "./cmd/iam-migrate"])
            binaries.update(iam_source=IAM_SOURCE, iam_schema_sha256=IAM_SQL, overlay={
                "source_fixture_sha256": self.sha(fixture),
                "patched_fixture_sha256": self.sha(patched),
                "test_sha256": self.sha(probe),
            })
        for name in ("iam-deletion.test", "iam-migrate"):
            binaries["sha256"][name] = self.sha(self.binaries / name)
        (self.output / "binaries.json").write_text(json.dumps(binaries, indent=2) + "\n")

        db = "hit68_test_hit148"
        self.sql("create-iam-db", "CREATE DATABASE "+db+"; REVOKE ALL ON DATABASE "+db+" FROM PUBLIC;",
                 role="postgres", database="postgres")
        self.sql("iam-bootstrap-roles", "\\i /iam-migration/bootstrap_roles.sql\n", role="postgres", database=db)
        self.sql("iam-test-users",
                 "CREATE ROLE hp_hit81_app LOGIN INHERIT; CREATE ROLE hit148_migrator LOGIN INHERIT;"
                 "GRANT hp_iam_app TO hp_hit81_app; GRANT hp_iam_migrator TO hit148_migrator;"
                 "GRANT CONNECT ON DATABASE "+db+" TO hp_hit81_app,hit148_migrator;",
                 role="postgres", database=db)
        dsn = "host=/var/run/postgresql dbname="+db+" sslmode=disable user="
        self.command("iam-migration", ["docker", "exec", "-e",
                     "HITPIPE_IAM_MIGRATOR_DSN="+dsn+"hit148_migrator options='-c role=hp_iam_migrator'",
                     self.container, "/test/iam-migrate"])
        self.command("real-iam-inventory", ["docker", "exec", "-e", "HITPIPE_IAM_TEST_ADMIN_DSN="+dsn+"postgres",
                     "-e", "HITPIPE_IAM_TEST_APP_DSN="+dsn+"hp_hit81_app", self.container,
                     "/test/iam-deletion.test", "-test.v", "-test.count=1", "-test.timeout=50s",
                     "-test.run=^Test(ProcessingConsumesRealHIT154Inventory|PostgresCaseDeletionQualification)$"])

    @staticmethod
    def sha(file):
        return hashlib.sha256(file.read_bytes()).hexdigest()

    def main(self):
        super().main()
        path = self.output / "summary.json"
        summary = json.loads(path.read_text())
        summary.update(task="HIT-148", iam_source=IAM_SOURCE,
                       scope="Pinned IAM mTLS, PostgreSQL owner confirmation, signed inventory. "
                             "Processing observes only; no case deletion or serving transition.")
        path.write_text(json.dumps(summary, indent=2) + "\n")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("output", "go", "iam"):
        parser.add_argument("--"+name, type=pathlib.Path, required=True)
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="hit148-build-") as binaries:
        Run(args.output.resolve(), args.go, pathlib.Path(binaries), args.iam.resolve()).main()
