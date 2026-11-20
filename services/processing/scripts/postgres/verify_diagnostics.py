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
        if label == "go-race":
            args.remove("-v")
        return super().command(label, args, *rest, **kwargs)

    def checks(self):
        super().checks()
        # Capture the new evidence from the same compiled source and pinned
        # database, using a separately recorded run of the complete HIT-127 tests.
        self.command("diagnostic-postgres-tests", [
            "docker", "exec", "-e", "HIT78_ISOLATED_PG=1",
            "-e", "HIT111_PROCESSING_BIN=/test/processing",
            "-e", "HIT126_PURGE_BIN=/test/processing-purge",
            "-e", "HIT127_CAPTURE_FILE=/tmp/hit127-diagnostics.jsonl",
            "-e", "HIT127_HTTP_CAPTURE_FILE=/tmp/hit127-http.jsonl",
            self.container, "/test/storage.test", "-test.v", "-test.count=1",
            "-test.timeout=45s",
            "-test.run=^Test(DiagnosticSampleBoundedAndScoped|ProcessingDiagnosticsOwnedStand|ProcessingPurgeOwnedStand)$",
        ])
        self.command("diagnostic-race-tests", [
            self.go, "-C", str(pathlib.Path(__file__).resolve().parents[2]),
            "test", "-race", "-count=1", "-v", "-run=^TestDiagnosticsBoundsAndOpaqueIDs$", "./internal/runtime",
        ])
        for label, name in (("diagnostic-runtime-capture", "hit127-diagnostics.jsonl"),
                            ("diagnostic-http-capture", "hit127-http.jsonl")):
            self.command(label, ["docker", "exec", self.container, "cat", f"/tmp/{name}"])

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
