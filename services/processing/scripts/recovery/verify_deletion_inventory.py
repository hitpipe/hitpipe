#!/usr/bin/env python3
"""Qualify HIT-154 application, restoration and Processing regressions."""
import argparse
import hashlib
import json
import pathlib
import tempfile

import verify_recovery

IAM_SOURCE = "c715a079ab57d4c9e7b56862f7f2584c792b74b0"
IAM_SQL = "604d693ff49ccde71755d2b7afccaa0e9b96f03fef2d2c0e420c08c7ab59bef7"


class Run(verify_recovery.Run):
    def __init__(self, output, go, binaries, iam, focused):
        super().__init__(output, go, binaries)
        self.name = self.name.replace("hit128-", "hit148-", 1)
        self.iam = iam
        self.focused = focused
        self.module = iam / "services/iam"
        self.migrations = self.module / "internal/adapters/postgres/migrations"
        self.script_sha256 = hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()

    def command(self, label, argv, *args, **kwargs):
        if self.focused and label in {"go-race", "go-vet", "recovery-process-tests", "predecessor-process-tests"}:
            return None
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
        tool_dir = verify_recovery.verify.REPO / "tools/hit148"
        probes = {f"processing_{name.replace('-', '_')}_test.go": tool_dir / f"{name}.go"
                  for name in ("inventory-probe", "deletion-flow", "deletion-runtime")}
        with tempfile.TemporaryDirectory(prefix="hit148-overlay-") as temp:
            patched = pathlib.Path(temp) / fixture.name
            overlay = pathlib.Path(temp) / "overlay.json"
            text = fixture.read_text()
            for old, new in [
                ('c.Host != "127.0.0.1"', 'c.Host != "/var/run/postgresql"'),
                ('"SHOW server_version"', '"SHOW server_version_num"'),
                ('version != "18.7"', 'version != "180007"'),
                ('type labTLS struct {', 'type labTLS struct {\n\tcaDER []byte\n\tissue func(int64, string, x509.ExtKeyUsage) tls.Certificate'),
                ('ca: ca, publish:', 'ca: ca, caDER: der, issue: issue, publish:'),
                ('cert.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}',
                 'cert.IPAddresses = []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("127.0.0.2"), net.ParseIP("127.0.0.4")}\n\t\t\tif dns != "" { cert.DNSNames = []string{dns} }'),
            ]:
                if text.count(old) != 1:
                    raise RuntimeError("unexpected pinned test fixture")
                text = text.replace(old, new)
            patched.write_text(text)
            overlay.write_text(json.dumps({"Replace": {
                str(fixture): str(patched),
                **{str(pkg / test): str(path) for test, path in probes.items()},
            }}))
            modfile = pathlib.Path(temp) / "iam-stand.mod"
            modfile.write_text((self.module / "go.mod").read_text())
            module_path = "github.com/hitpipe/hitpipe/services/"
            module_source = verify_recovery.verify.MODULE
            sums = (self.module / "go.sum").read_text().splitlines()
            sums += (module_source / "go.sum").read_text().splitlines()
            modfile.with_suffix(".sum").write_text("\n".join(sorted(set(sums)))+"\n")
            self.command("compose-iam-processing-test-module", [*go, "mod", "edit", "-modfile="+str(modfile),
                         "-require="+module_path+"processing@v0.0.0",
                         "-replace="+module_path+"processing="+str(module_source),
                         "-replace="+module_path+"platform="+str(self.iam / "services/platform")])
            self.command("build-iam-deletion-interop", [*go, "test", "-c", "-tags", "integration,hit148_probe",
                         "-modfile="+str(modfile), "-mod=mod", "-overlay", str(overlay),
                         "-o", str(self.binaries / "iam-deletion.test"), "./internal/adapters/composition/casedeletion"])
            self.command("build-iam-migrations", [*go, "build", "-trimpath",
                         "-o", str(self.binaries / "iam-migrate"), "./cmd/iam-migrate"])
            binaries.update(iam_source=IAM_SOURCE, iam_schema_sha256=IAM_SQL, overlay={
                "source_fixture_sha256": self.sha(fixture),
                "patched_fixture_sha256": self.sha(patched),
                "test_sha256": {file.name: self.sha(file) for file in probes.values()},
                "test_mod_sha256": self.sha(modfile),
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
        for fixture_db in ("hit148_source", "hit148_backup", "hit148restore"):
            self.sql("create-"+fixture_db, f"CREATE DATABASE {fixture_db} OWNER hp_processing_owner;"
                     f"REVOKE ALL ON DATABASE {fixture_db} FROM PUBLIC;"
                     f"GRANT CONNECT ON DATABASE {fixture_db} TO hp_processing_migrator,hp_processing_app,"
                     "hp_processing_worker,hp_processing_purge;", role="postgres", database="postgres")
            self.sql("migrate-"+fixture_db, "\\i /migrations/apply.sql\n", role="hp_processing_migrator", database=fixture_db)
        self.command("real-iam-inventory", ["docker", "exec", "-e", "HITPIPE_IAM_TEST_ADMIN_DSN="+dsn+"postgres",
                     "-e", "HITPIPE_IAM_TEST_APP_DSN="+dsn+"hp_hit81_app", "-e", "HIT148_PROCESSING_SOURCE="+self.source,
                     self.container,
                     "/test/iam-deletion.test", "-test.v", "-test.count=1", "-test.timeout=50s",
                     "-test.run=^TestProcessingAppliesAndRestoresRealHIT154$" if self.focused else
                     "-test.run=^Test(ProcessingConsumesRealHIT154Inventory|ProcessingAppliesAndRestoresRealHIT154|PostgresCaseDeletionQualification)$"])

    @staticmethod
    def sha(file):
        return hashlib.sha256(file.read_bytes()).hexdigest()

    def main(self):
        super().main()
        path = self.output / "summary.json"
        summary = json.loads(path.read_text())
        summary.update(task="HIT-148", iam_source=IAM_SOURCE,
                       scope="Pinned IAM and real Processing application, interrupted receipt, restore replay and actual opening.",
                       focused=self.focused)
        path.write_text(json.dumps(summary, indent=2) + "\n")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("output", "go", "iam"):
        parser.add_argument("--"+name, type=pathlib.Path, required=True)
    parser.add_argument("--focused", action="store_true", help="run only the new addressable deletion scenario")
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="hit148-build-") as binaries:
        Run(args.output.resolve(), args.go, pathlib.Path(binaries), args.iam.resolve(), args.focused).main()
