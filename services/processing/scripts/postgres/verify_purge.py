#!/usr/bin/env python3
"""Qualify the HIT-126 command on the existing isolated Processing stand."""

import argparse
import hashlib
import json
import pathlib
import uuid

import verify
import verify_runtime


class Run(verify_runtime.Run):
    def __init__(self, output, go):
        super().__init__(output, go)
        self.name = "hit126-" + uuid.uuid4().hex
        self.script_sha256 = hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()

    def command(self, label, args, *rest, **kwargs):
        args = [*args]
        if label == "compile-linux-test":
            arch = next(arg for arg in args if arg.startswith("GOARCH="))
            binary = pathlib.Path(self.binary_dir.name) / "processing-purge"
            self.command("compile-purge-linux", [
                "env", "GOOS=linux", arch, "CGO_ENABLED=0", self.go, "-C", str(verify.MODULE),
                "build", "-o", str(binary), "./cmd/processing-purge",
            ])
            (self.output / "purge-binary.json").write_text(json.dumps({
                "name": "processing-purge", "source_sha": self.source,
                "sha256": hashlib.sha256(binary.read_bytes()).hexdigest(), "GOOS": "linux",
                "GOARCH": arch.split("=", 1)[1], "CGO_ENABLED": "0",
            }, indent=2) + "\n")
        if label == "postgres-storage-tests":
            args[2:2] = ["-e", "HIT126_PURGE_BIN=/test/processing-purge"]
        if label == "container-create":
            # verify_runtime sets a predecessor label before passing to the
            # shared runner, so the created name still isolates this run.
            args = [self.name if a.startswith("hit111-") else a for a in args]
        return super().command(label, args, *rest, **kwargs)

    def main(self):
        super().main()
        p = self.output / "summary.json"
        out = json.loads(p.read_text())
        out.update(task="HIT-126", scope="Processing purge binary and predecessor regression on disposable PostgreSQL 18.7 plus durable files; host race/vet/build.")
        p.write_text(json.dumps(out, indent=2) + "\n")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True, type=pathlib.Path)
    parser.add_argument("--go", required=True, type=pathlib.Path)
    options = parser.parse_args()
    Run(options.output.resolve(), options.go).main()
