"""Run the current local Platform gate checks and keep genuine, bounded evidence."""

import argparse
import hashlib
import json
import os
import platform
import shutil
import subprocess
import sys
import time
from datetime import datetime, timezone
from pathlib import Path


MODULE = Path(__file__).resolve().parents[1]
REPO = MODULE.parents[1]


def utc():
    return datetime.now(timezone.utc).isoformat()


def sources():
    paths = [MODULE / "go.mod", Path(__file__).resolve()]
    paths += sorted((MODULE / "startgate").glob("*.go"))
    assert len(paths) >= 4, "Gate source and tests are required"
    return {
        str(path.relative_to(REPO)): hashlib.sha256(path.read_bytes()).hexdigest()
        for path in paths
    }


def run(args):
    out = args.evidence_dir.resolve()
    out.mkdir(parents=True, exist_ok=True)
    if out == MODULE or out in (MODULE / "startgate").parents:
        raise ValueError("Evidence directory must not replace module sources")
    go = shutil.which(args.go_bin)
    if go is None:
        raise ValueError("Go 1.27.2 executable is required")
    env = os.environ.copy()
    env.update(GOTOOLCHAIN="local", GOWORK="off", CGO_ENABLED="1")
    env.setdefault("GOMAXPROCS", "2")
    env.setdefault("GOGC", "50")
    version = subprocess.check_output([go, "version"], env=env, text=True).strip()
    if version.split()[2] != "go1.27.2":
        raise ValueError("Use the accepted Go 1.27.2 build cohort")
    before = sources()
    proof = {
        "schema_version": 1,
        "task": "HIT-69",
        "started_at": utc(),
        "source_commit": subprocess.check_output(
            ["git", "rev-parse", "HEAD"], cwd=REPO, text=True
        ).strip(),
        "source_files_sha256": before,
        "go_version": version,
        "platform": platform.platform(),
        "cpu_count": os.cpu_count(),
        "GOMAXPROCS": env["GOMAXPROCS"],
        "GOTOOLCHAIN": "local",
        "scope": "Controlled in-process starts, snapshots and barriers; no live service, IAM authority or distributed fencing qualification",
        "checks": [],
        "state": "RUNNING",
    }
    try:
        commands = [
            ("unit", ["test", "-p", "1", "-count=1", "-v", "./..."]),
            ("race", ["test", "-p", "1", "-race", "-count=10", "./..."]),
            ("vet", ["vet", "-p", "1", "./..."]),
            ("build", ["build", "-p", "1", "./..."]),
        ]
        for name, command in commands:
            started = time.monotonic()
            target = out / (name + ".log")
            with target.open("wb") as log:
                result = subprocess.run(
                    [go, *command], cwd=MODULE, env=env, stdout=log,
                    stderr=subprocess.STDOUT, timeout=300, check=False,
                )
            proof["checks"].append({
                "name": name, "command": ["go", *command],
                "exit_code": result.returncode,
                "elapsed_seconds": round(time.monotonic() - started, 3),
                "log": target.name,
                "log_sha256": hashlib.sha256(target.read_bytes()).hexdigest(),
            })
            print(json.dumps(proof["checks"][-1]), flush=True)
            if result.returncode:
                raise RuntimeError(name + " failed; inspect " + str(target))
        if sources() != before:
            raise RuntimeError("Sources changed during verification; result is not qualified")
        proof["state"] = "PASSED"
    finally:
        if proof["state"] != "PASSED":
            proof["state"] = "FAILED"
        proof["finished_at"] = utc()
        (out / "verification.json").write_text(
            json.dumps(proof, ensure_ascii=False, indent=2) + "\n"
        )
    print(json.dumps({"state": proof["state"], "evidence_dir": str(out)}))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--evidence-dir", type=Path, required=True)
    parser.add_argument("--go-bin", default="go")
    parsed = parser.parse_args()
    try:
        run(parsed)
    except (ValueError, RuntimeError, OSError, subprocess.SubprocessError) as error:
        print(str(error), file=sys.stderr)
        raise SystemExit(1)
