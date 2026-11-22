#!/usr/bin/env python3
"""Run the existing image and recovery scenarios on one immutable TEST image."""
import argparse
import hashlib
import json
import pathlib
import sys
import tempfile

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1] / "postgres"))
from verify_image import CONFIG, CONTENT, CONTROL, UID, ImageRun


class RecoveryImageRun(ImageRun):
    task = "HIT-155"
    test_summary = "HIT-145 image restart and HIT-128 two-cluster recovery on the same inspected OCI image"

    def __init__(self, output, go, fixture):
        super().__init__(output, go, fixture)
        self.script_sha256 = hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()

    def checks(self):
        go = ["env", "GOTOOLCHAIN=local", "GOWORK=off", self.go, "-C", "services/processing"]
        self.command("go-race", [*go, "test", "-race", "-count=1", "./..."])
        self.command("go-vet", [*go, "vet", "./..."])
        self.command("build-purge", ["env", "GOOS=linux", "GOARCH=" + self.identity["platform"].split("/")[1],
                                    "CGO_ENABLED=0", *go, "build", "-trimpath", "-o",
                                    str(self.fixture / "processing-purge"), "./cmd/processing-purge"])
        super().checks()
        self.check_recovery()

    def check_recovery(self):
        status = "recovery-status"
        runner = (f"/test/storage.test -test.v -test.count=1 -test.timeout=110s "
                  f"-test.run=^TestProcessingOCIRecoveryOwnedStand$ >{CONTROL}/recovery.stdout 2>{CONTROL}/recovery.stderr; "
                  f'code=$?; printf "%s\\n" "$code" >{CONTROL}/{status}.tmp && mv {CONTROL}/{status}.tmp {CONTROL}/{status}')
        self.docker("recovery-driver", "exec", "--detach", "--user", "0", "--env", "HIT78_ISOLATED_PG=1",
                    "--env", "HIT128_RECOVERY_STAND=1", "--env", "HIT155_SOURCE_SHA=" + self.source,
                    "--env", "HIT126_PURGE_BIN=/test/processing-purge", "--env", "TMPDIR=" + CONFIG + "/fixture",
                    self.container, "sh", "-c", runner)
        try:
            for step in range(1, 13):
                request = json.loads(self.await_file(f"recovery-{step}.json", status))
                if set(request) != {"operation", "args"}:
                    raise ValueError("unexpected recovery control fields")
                args = request["args"]
                if step % 2:
                    if (request["operation"] != "start" or not isinstance(args, list)
                            or len(args) not in (2, 4) or args[0] != "-config"
                            or (len(args) == 4 and args[2] != "-stand-config")):
                        raise ValueError("unexpected recovery start request")
                    paths = {"config": args[1]}
                    if len(args) == 4:
                        paths["stand"] = args[3]
                    # The Go recovery fixture owns the disposable cluster setup as
                    # root. Make its private volumes usable by the actual OCI UID.
                    self.docker("recovery-volume-ownership", "exec", "--user", "0", self.container,
                                "chown", "-R", UID, CONFIG + "/fixture", CONTENT)
                    self.start_app("recovery-" + str(step // 2 + 1), paths)
                else:
                    if request != {"operation": "kill", "args": None}:
                        raise ValueError("unexpected recovery stop request")
                    self.stop_app(kill=True)
                self.docker("recovery-ack", "exec", self.container, "touch", f"{CONTROL}/recovery-ack-{step}")
            result = self.await_file(status, status)
            if result.strip() != b"0":
                raise RuntimeError("OCI recovery fixture failed")
        finally:
            stdout = self.docker("recovery-stdout", "exec", self.container, "cat", CONTROL + "/recovery.stdout",
                                 required=False)
            self.docker("recovery-stderr", "exec", self.container, "cat", CONTROL + "/recovery.stderr", required=False)
            if self.app_name:
                self.docker("recovery-incomplete-logs", "logs", self.app_name, required=False)
                self.docker("recovery-forced-cleanup", "rm", "-f", "-v", self.app_name, required=False)
                self.app_name = None
        if b"RECOVERY PASS:" not in stdout.stdout:
            raise RuntimeError("OCI recovery fixture did not verify the restored state")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--go", required=True, type=pathlib.Path, help="absolute path to Go 1.27.2")
    parser.add_argument("--output", required=True, type=pathlib.Path, help="new directory outside the checkout")
    args = parser.parse_args()
    if not args.go.is_absolute():
        parser.error("--go must be absolute")
    with tempfile.TemporaryDirectory(prefix="hit155-") as scratch:
        fixture = pathlib.Path(scratch)
        fixture.chmod(0o755)
        RecoveryImageRun(args.output.resolve(), str(args.go), fixture).run()
