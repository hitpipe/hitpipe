#!/usr/bin/env python3
"""Prepare unchanged HIT-68 production code with identical HIT-82 test harness."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess

REPO = Path(__file__).resolve().parents[3]
BASE = "ce771d37505e7770003d0796ae932556d06b7bab"
LOOKUP = Path("services/iam/internal/adapters/postgres/sessionlookup")
EXPECTED = "9c13c8261d0752bd6c558503cae635d76f5ae47fc97d130bad5a7c4d661a8379"
HARNESS = ("reference_decode_test.go", "lookup_benchmark_test.go", "lookup_benchmark_integration_test.go")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True, help="New local worktree directory")
    args = parser.parse_args()
    output = args.output.resolve()
    if output.exists():
        parser.error("output must not already exist; existing files are never overwritten")
    subprocess.run(["git", "worktree", "add", "--detach", str(output), BASE], cwd=REPO, check=True)
    if hashlib.sha256((output / LOOKUP / "lookup.go").read_bytes()).hexdigest() != EXPECTED:
        raise RuntimeError("baseline production code is not the accepted HIT-68 implementation")
    hashes = {}
    for name in HARNESS:
        shutil.copyfile(REPO / LOOKUP / name, output / LOOKUP / name)
        hashes[name] = hashlib.sha256((output / LOOKUP / name).read_bytes()).hexdigest()
    # The original production Lookup never calls this shim. Only the auxiliary
    # decoder benchmark uses the old algorithm extracted verbatim in the oracle.
    (output / LOOKUP / "baseline_decode_test.go").write_text(
        'package sessionlookup\n\nimport "crypto/sha256"\n\n'
        'func decodeReference(reference string) ([sha256.Size]byte, error) {\n'
        '\treturn legacyDecodeReference(reference)\n}\n')
    print(json.dumps({"baseline_commit": BASE, "production_lookup_sha256": EXPECTED,
                      "test_overlay_sha256": hashes, "output": str(output)}, indent=2))


if __name__ == "__main__":
    main()
