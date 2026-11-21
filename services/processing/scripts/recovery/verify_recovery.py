#!/usr/bin/env python3
"""Qualify Processing recovery in two private PostgreSQL clusters."""
import argparse
import hashlib
import json
import pathlib
import tempfile
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1] / "postgres"))
import verify


class Run(verify.Run):
    def __init__(self, output, go, binaries):
        super().__init__(output)
        self.name = self.name.replace("hit76-", "hit128-", 1)
        self.db = "hit78"
        self.go = str(go.resolve())
        self.binaries = pathlib.Path(binaries)
        self.script_sha256 = hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()

    def command(self, label, argv, *args, **kwargs):
        if label == "container-create":
            argv = ["hitpipe.task=HIT-128" if a == "hitpipe.task=HIT-76" else a for a in argv]
            argv[2:2] = ["--mount", f"type=bind,source={self.binaries},target=/test,readonly"]
        return super().command(label, argv, *args, **kwargs)

    def checks(self):
        go = ["env", "GOTOOLCHAIN=local", "GOWORK=off", self.go]
        version = self.command("go-version", [*go, "version"]).stdout.split()
        if version[2] != b"go1.27.2":
            raise RuntimeError("the runner requires Go 1.27.2")
        go += ["-C", str(verify.MODULE)]
        self.command("go-race", [*go, "test", "-race", "-count=1", "./..."])
        self.command("go-vet", [*go, "vet", "./..."])
        arch = self.command("container-arch", ["docker", "exec", self.container, "uname", "-m"])
        target = {"aarch64": "arm64", "x86_64": "amd64"}[arch.stdout.decode().strip()]
        linux = ["env", "GOOS=linux", "GOARCH=" + target, "CGO_ENABLED=0", *go]
        hashes = {}
        for name in ("processing", "processing-purge"):
            path = self.binaries / name
            self.command("build-" + name, [*linux, "build", "-trimpath", "-ldflags",
                         "-X main.revision=" + self.source, "-o", str(path), "./cmd/" + name])
            hashes[name] = hashlib.sha256(path.read_bytes()).hexdigest()
        path = self.binaries / "storage.test"
        self.command("build-recovery-tests",
                     [*linux, "test", "-c", "-o", str(path), "./internal/adapters/postgres"])
        hashes[path.name] = hashlib.sha256(path.read_bytes()).hexdigest()
        (self.output / "binaries.json").write_text(json.dumps({
            "source_sha": self.source, "platform": "linux/" + target, "sha256": hashes,
        }, indent=2) + "\n")
        self.command("recovery-process-tests", [
            "docker", "exec", "-e", "HIT78_ISOLATED_PG=1", "-e", "HIT128_RECOVERY_STAND=1",
            "-e", "HIT111_PROCESSING_BIN=/test/processing", "-e", "HIT126_PURGE_BIN=/test/processing-purge",
            self.container, "/test/storage.test", "-test.v", "-test.count=1", "-test.timeout=110s",
            "-test.run=^TestProcessingRecoveryOwnedStand$",
        ])
        self.command("predecessor-process-tests", [
            "docker", "exec", "-e", "HIT78_ISOLATED_PG=1", "-e", "HIT111_PROCESSING_BIN=/test/processing",
            "-e", "HIT126_PURGE_BIN=/test/processing-purge", self.container, "/test/storage.test",
            "-test.v", "-test.count=1", "-test.timeout=145s",
            "-test.run=^TestProcessing(Executable|CaseWorker|Recheck|CaseTimeouts|Purge|Diagnostics)OwnedStand$",
        ])

    def main(self):
        if self.tree_status:
            raise RuntimeError("use a clean checkout and an output directory outside it")
        super().main()
        path = self.output / "summary.json"
        summary = json.loads(path.read_text())
        summary.update(task="HIT-128", scope="Two real PostgreSQL 18.7 clusters inside one disposable, "
                       "network-none stand. Isolated Linux Processing and purge, controlled mTLS neighbors. "
                       "Selective in-memory SQL recovery only; no expiring file copy or operational RTO.")
        path.write_text(json.dumps(summary, indent=2) + "\n")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=pathlib.Path, required=True)
    parser.add_argument("--go", type=pathlib.Path, required=True)
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="hit128-build-") as binaries:
        Run(args.output.resolve(), args.go, binaries).main()
