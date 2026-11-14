#!/usr/bin/env python3
"""Run HIT-68 checks in a disposable local database; never accept an external DSN."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import sys
import time
import uuid

IAM = Path(__file__).resolve().parents[1]
REPO = IAM.parents[1]
IMAGE = "postgres:18.7-alpine@sha256:75dcac3dfb1cd01ca3536910bbc84128759362f8b118bf8c55b38a94f68f3b73"
SQL = IAM / "internal/adapters/postgres/sessionlookup/testdata/iam.sql"
SQL_SHA256 = "604d693ff49ccde71755d2b7afccaa0e9b96f03fef2d2c0e420c08c7ab59bef7"


def run(args, *, env=None, data=None, check=True):
    return subprocess.run(args, cwd=IAM, input=data, text=True, capture_output=True,
                          env=env, check=check)


def source_digest():
    digest = hashlib.sha256()
    for path in sorted(IAM.rglob("*")):
        relative = path.relative_to(IAM)
        if not path.is_file() or relative.parts[0] == "evidence" or "__pycache__" in relative.parts:
            continue
        digest.update(str(relative).encode() + b"\0" + path.read_bytes() + b"\0")
    return digest.hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--go", default="go", help="Path to the accepted Go 1.27.2 binary")
    parser.add_argument("--evidence-dir", type=Path, default=IAM / "evidence/security-postgres-18.7")
    args = parser.parse_args()
    if hashlib.sha256(SQL.read_bytes()).hexdigest() != SQL_SHA256:
        parser.error("IAM fixture differs from the complete accepted SQL source")
    if shutil.disk_usage(REPO).free < 512 * 1024 * 1024:
        parser.error("less than 512 MiB of disk space is available")
    environment = os.environ.copy()
    environment.update(GOTOOLCHAIN="local", GOMAXPROCS="1", GOGC="50")
    version = run([args.go, "version"], env=environment).stdout.strip()
    if not version.startswith("go version go1.27.2 "):
        parser.error("the accepted Go 1.27.2 toolchain is required")
    evidence = args.evidence_dir.resolve()
    evidence.mkdir(parents=True, exist_ok=True)
    suffix = uuid.uuid4().hex[:12]
    name, network, database = "hit68-pg-" + suffix, "hit68-net-" + suffix, "hit68_test_" + suffix
    result = {
        "task": "HIT-68", "maintenance": "HP-CCR-POSTGRES-20261114", "schema_version": 1, "started_at_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "source_commit": run(["git", "rev-parse", "HEAD"]).stdout.strip(),
        "source_tree_sha256": source_digest(), "go_version": version,
        "platform": platform.platform(), "cpu_count": os.cpu_count(),
        "execution_limits": {"GOMAXPROCS": 1, "GOGC": 50, "go_build_parallelism": 1,
                             "postgres_memory_mib": 256, "postgres_cpus": 0.5},
        "postgres_image": IMAGE, "iam_sql_sha256": SQL_SHA256,
        "scope": "Isolated adapter and existing consumer tests, not login, authorization, trusted time or deployment qualification",
        "checks": [], "state": "FAILED",
    }
    network_created, container_started = False, False
    try:
        if run(["docker", "image", "inspect", IMAGE], check=False).returncode:
            run(["docker", "pull", IMAGE])
        # A dedicated bridge allows an exclusively loopback-published host port.
        # Docker internal networks suppress publishing, even with 127.0.0.1.
        run(["docker", "network", "create", network])
        network_created = True
        run(["docker", "run", "--detach", "--name", name, "--network", network,
             "--publish", "127.0.0.1::5432", "--memory", "256m", "--cpus", "0.5", "--pids-limit", "128",
             "--tmpfs", "/var/lib/postgresql:rw,nosuid,nodev,size=192m", "--shm-size", "32m",
             "--env", "POSTGRES_HOST_AUTH_METHOD=trust", "--env", "POSTGRES_DB=" + database,
             IMAGE, "postgres", "-c", "shared_buffers=24MB", "-c", "max_connections=20",
             "-c", "log_statement=none", "-c", "log_min_error_statement=panic",
             "-c", "log_parameter_max_length=0", "-c", "log_parameter_max_length_on_error=0"])
        container_started = True
        for _ in range(60):
            ready = run(["docker", "exec", name, "pg_isready", "-h", "127.0.0.1", "-U", "postgres", "-d", database], check=False)
            if ready.returncode == 0:
                break
            time.sleep(0.5)
        else:
            raise RuntimeError("isolated PostgreSQL did not become ready")
        psql = ["docker", "exec", "-i", name, "psql", "-X", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", database]
        bootstrap = "\n".join(
            f"CREATE ROLE {role} NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;"
            for role in ("hp_iam_app", "hp_iam_worker", "hp_iam_purge")
        )
        bootstrap += "\n" + SQL.read_text() + "\n"
        bootstrap += "CREATE ROLE hp_hit68_lookup LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS IN ROLE hp_iam_app;\n"
        bootstrap += "ALTER ROLE hp_hit68_lookup SET default_transaction_read_only=on;\n"
        run(psql, data=bootstrap)
        server = run(psql + ["-Atc", "SHOW server_version"]).stdout.strip()
        if server != "18.7":
            raise RuntimeError("isolated PostgreSQL version does not match the accepted cohort")
        result["postgres_version"] = server
        address = run(["docker", "port", name, "5432/tcp"]).stdout.strip()
        if not address.startswith("127.0.0.1:") or "\n" in address:
            raise RuntimeError("test database was not bound exclusively to loopback")
        environment["HITPIPE_IAM_TEST_ADMIN_DSN"] = f"postgres://postgres@{address}/{database}?sslmode=disable"
        environment["HITPIPE_IAM_TEST_DSN"] = f"postgres://hp_hit68_lookup@{address}/{database}?sslmode=disable"
        commands = [
            ("unit", [args.go, "test", "-p", "1", "-count=1", "./..."]),
            ("postgres-race", [args.go, "test", "-p", "1", "-tags=integration", "-race", "-count=1", "-v", "./..."]),
            ("vet", [args.go, "vet", "-p", "1", "-tags=integration", "./..."]),
            ("build", [args.go, "build", "-p", "1", "./..."]),
        ]
        for label, command in commands:
            started = time.monotonic()
            completed = run(command, env=environment, check=False)
            log = completed.stdout + completed.stderr
            (evidence / (label + ".log")).write_text(log)
            check = {"name": label, "command": ["go", *command[1:]], "exit_code": completed.returncode,
                     "elapsed_seconds": round(time.monotonic() - started, 3),
                     "log": label + ".log", "log_sha256": hashlib.sha256(log.encode()).hexdigest()}
            result["checks"].append(check)
            print(json.dumps(check), flush=True)
            if completed.returncode:
                raise RuntimeError(label + " check failed; inspect the saved log")
            if label == "postgres-race" and ("--- SKIP: TestPostgres" in log or "--- PASS: TestPostgres" not in log):
                raise RuntimeError("required real PostgreSQL tests did not run")
        if source_digest() != result["source_tree_sha256"]:
            raise RuntimeError("source files changed during the checks")
        result["state"] = "PASSED"
    except (OSError, RuntimeError, subprocess.SubprocessError) as exc:
        # Do not print arbitrary subprocess output or connection configuration.
        result["failure"] = str(exc) if isinstance(exc, RuntimeError) else type(exc).__name__
        if isinstance(exc, subprocess.CalledProcessError):
            # These commands contain only this script's synthetic sandbox values.
            # DSNs are environment-only and arbitrary external DSNs are never accepted.
            result["failed_step"] = exc.cmd[:5]
            result["setup_stderr"] = (exc.stderr or "")[-4000:]
            print(json.dumps({"step": result["failed_step"], "stderr": result["setup_stderr"]}), file=sys.stderr)
        print(result["failure"], file=sys.stderr)
    finally:
        if container_started:
            result["test_container_removed"] = run(["docker", "rm", "--force", name], check=False).returncode == 0
        if network_created:
            result["test_network_removed"] = run(["docker", "network", "rm", network], check=False).returncode == 0
        if result.get("test_container_removed") is False or result.get("test_network_removed") is False:
            result["state"] = "FAILED"
            result["failure"] = "test sandbox cleanup failed"
        result["finished_at_utc"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        (evidence / "verification.json").write_text(json.dumps(result, indent=2) + "\n")
    return 0 if result["state"] == "PASSED" else 1


if __name__ == "__main__":
    sys.exit(main())
