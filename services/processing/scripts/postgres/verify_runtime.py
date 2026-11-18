#!/usr/bin/env python3
"""Build an actual Processing process and qualify it on the owned PG18.7 stand."""

import argparse
import hashlib
import json
import pathlib
import uuid

import verify
import verify_lifecycle


class Run(verify_lifecycle.Run):
    def __init__(self, output, go):
        super().__init__(output, go)
        self.name = "hit111-" + uuid.uuid4().hex
        self.script_sha256 = hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()

    def command(self, label, args, *rest, **kwargs):
        args = [*args]
        if label == "compile-linux-test":
            target = next(a for a in args if a.startswith("GOARCH="))
            binary = pathlib.Path(self.binary_dir.name) / "processing"
            self.command("compile-runtime-linux", [
                "env", "GOOS=linux", target, "CGO_ENABLED=0", self.go, "-C", str(verify.MODULE),
                "build", "-ldflags", "-X main.revision=" + self.source, "-o", str(binary), "./cmd/processing",
            ])
            (self.output / "runtime-binary.json").write_text(json.dumps({
                "name": "processing", "source_sha": self.source,
                "sha256": hashlib.sha256(binary.read_bytes()).hexdigest(), "GOOS": "linux",
                "GOARCH": target.split("=", 1)[1], "CGO_ENABLED": "0",
            }, indent=2) + "\n")
        if label == "postgres-storage-tests":
            args[2:2] = ["-e", "HIT111_PROCESSING_BIN=/test/processing"]
        # The inherited HIT-103 runner sets its own label before calling its
        # parent. Set the label after that step by calling the same owned runner
        # directly here, preserving capture environment for predecessor tests.
        args = ["hitpipe.task=HIT-111" if a.startswith("hitpipe.task=HIT-") else a for a in args]
        if label == "postgres-storage-tests":
            args[2:2] = ["-e", "HIT103_CAPTURE_FILE=/tmp/hit103-captures.jsonl",
                         "-e", "HIT102_CAPTURE_FILE=/tmp/hit102-captures.jsonl"]
            args = ["-test.timeout=120s" if a == "-test.timeout=60s" else a for a in args]
        return super(verify_lifecycle.Run, self).command(label, args, *rest, **kwargs)

    def main(self):
        super().main()
        path = self.output / "summary.json"
        out = json.loads(path.read_text())
        out.update(
            task="HIT-111",
            scope="Owned PG18.7 and content. Actual standalone Linux executable, private "
            "HTTPS/mTLS, controlled IAM/Intake/Delivery, one HIT-69 gate, "
            "durable sender, kill/restart and graceful SIGTERM. Host race/vet/build separate.",
        )
        path.write_text(json.dumps(out, indent=2) + "\n")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True, type=pathlib.Path)
    parser.add_argument("--go", required=True, type=pathlib.Path)
    options = parser.parse_args()
    Run(options.output.resolve(), options.go).main()
