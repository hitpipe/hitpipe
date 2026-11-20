#!/usr/bin/env python3
"""Qualify runtime diagnostics on Processing's disposable PostgreSQL stand."""

import argparse
import hashlib
import json
import pathlib
import uuid

import verify_purge


class Run(verify_purge.Run):
    def __init__(self, output, go):
        super().__init__(output, go)
        self.name = "hit127-" + uuid.uuid4().hex
        self.script_sha256 = hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()

    def command(self, label, args, *rest, **kwargs):
        args = [*args]
        if label == "postgres-storage-tests":
            args[2:2] = ["-e", "HIT127_CAPTURE_FILE=/tmp/hit127-diagnostics.jsonl"]
        return super().command(label, args, *rest, **kwargs)

    def checks(self):
        super().checks()
        result = self.command("diagnostic-runtime-capture", [
            "docker", "exec", self.container, "cat", "/tmp/hit127-diagnostics.jsonl",
        ])
        (self.output / "diagnostic-runtime.jsonl").write_bytes(result.stdout)

    def main(self):
        super().main()
        path = self.output / "summary.json"
        summary = json.loads(path.read_text())
        summary.update(task="HIT-127", scope="Actual Processing executable, TLS/mTLS HTTP, queue diagnostics, PostgreSQL 18.7 and owned file faults, restart and stderr purge diagnostics on a disposable stand; host race/vet/build.")
        path.write_text(json.dumps(summary, indent=2) + "\n")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True, type=pathlib.Path)
    parser.add_argument("--go", required=True, type=pathlib.Path)
    options = parser.parse_args()
    Run(options.output.resolve(), options.go).main()
