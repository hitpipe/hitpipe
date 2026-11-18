#!/usr/bin/env python3
"""HIT-103 lifecycle checks against the pinned, disposable PostgreSQL 18.7 stand."""

import argparse
import hashlib
import json
import pathlib
import uuid

import verify_drafts


class Run(verify_drafts.Run):
    def __init__(self, output, go):
        super().__init__(output, go)
        self.name = "hit103-" + uuid.uuid4().hex
        self.script_sha256 = hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()

    def command(self, label, args, *rest, **kwargs):
        args = ["hitpipe.task=HIT-103" if arg.startswith("hitpipe.task=HIT-") else arg for arg in args]
        if label == "postgres-storage-tests":
            args[2:2] = ["-e", "HIT103_CAPTURE_FILE=/tmp/hit103-captures.jsonl",
                         "-e", "HIT102_CAPTURE_FILE=/tmp/hit102-captures.jsonl"]
            args = ["-test.timeout=240s" if x == "-test.timeout=60s" else x for x in args]
        return super().command(label, args, *rest, **kwargs)

    def checks(self):
        super().checks()
        self.command("activation-api-captures", ["docker", "exec", self.container, "cat", "/tmp/hit102-captures.jsonl"])
        capture = self.command("lifecycle-api-captures", [
            "docker", "exec", self.container, "cat", "/tmp/hit103-captures.jsonl",
        ])
        (self.output / "lifecycle-api-captures.jsonl").write_bytes(capture.stdout)
        pipeline = self.command("integrated-lifecycle-pipeline", [
            "docker", "exec", self.container, "cat", "/tmp/activation-pipeline.json",
        ])
        (self.output / "activation-pipeline.json").write_bytes(pipeline.stdout)

    def main(self):
        super().main()
        path = self.output / "summary.json"
        result = json.loads(path.read_bytes())
        result.update(
            task="HIT-103",
            scope="Existing private HTTPS/mTLS, owned PostgreSQL 18.7 and durable Processing files. "
            "Controlled queue, bindings, grants and gate. Host Go race/vet/build and separate Linux CGO=0 DB binary.",
        )
        path.write_text(json.dumps(result, indent=2) + "\n")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True, type=pathlib.Path)
    parser.add_argument("--go", required=True, type=pathlib.Path)
    options = parser.parse_args()
    Run(options.output.resolve(), options.go).main()
