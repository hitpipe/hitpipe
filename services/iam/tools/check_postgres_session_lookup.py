#!/usr/bin/env python3
"""Run IAM checks in a disposable local database; never accept an external DSN."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import socket
import subprocess
import sys
import time
import uuid
from urllib.parse import quote, urlencode

IAM = Path(__file__).resolve().parents[1]
REPO = IAM.parents[1]
IMAGE = "postgres:18.7-alpine@sha256:75dcac3dfb1cd01ca3536910bbc84128759362f8b118bf8c55b38a94f68f3b73"
SQL = IAM / "internal/adapters/postgres/sessionlookup/testdata/iam.sql"
SQL_SHA256 = "604d693ff49ccde71755d2b7afccaa0e9b96f03fef2d2c0e420c08c7ab59bef7"
MIGRATIONS = IAM / "internal/adapters/postgres/migrations"


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


def main(*, task="HIT-81", schema_checks=True, authorization_checks=False):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--go", default="go", help="Path to the accepted Go 1.27.2 binary")
    parser.add_argument("--evidence-dir", type=Path, default=IAM / "evidence" / task)
    parser.add_argument("--measure-baseline", type=Path,
                        help="Optional local HIT-82 baseline worktree; never a database URL")
    args = parser.parse_args()
    # The established CI entry must run the new real-database authorization
    # checks whenever this implementation is present, including through the
    # historical schema/lookup wrappers.
    authorization_checks = authorization_checks or (IAM / "internal/application/authorizeaction").is_dir()
    profile_checks = (IAM / "internal/application/getprofile").is_dir()
    if hashlib.sha256(SQL.read_bytes()).hexdigest() != SQL_SHA256:
        parser.error("IAM fixture differs from the complete accepted SQL source")
    if schema_checks and hashlib.sha256((MIGRATIONS / "schema/001_iam.sql").read_bytes()).hexdigest() != SQL_SHA256:
        parser.error("IAM migration differs from the complete accepted SQL source")
    if shutil.disk_usage(REPO).free < 512 * 1024 * 1024:
        parser.error("less than 512 MiB of disk space is available")
    environment = os.environ.copy()
    # Never inherit another sandbox's or a live environment's IAM connections.
    for key in list(environment):
        if key.startswith("HITPIPE_IAM_"):
            del environment[key]
    environment.update(GOTOOLCHAIN="local", GOMAXPROCS="1", GOGC="50")
    version = run([args.go, "version"], env=environment).stdout.strip()
    if not version.startswith("go version go1.27.2 "):
        parser.error("the accepted Go 1.27.2 toolchain is required")
    evidence = args.evidence_dir.resolve()
    if evidence.exists() and any(evidence.iterdir()):
        parser.error("the evidence directory must be empty; use --evidence-dir for a new run")
    evidence.mkdir(parents=True, exist_ok=True)
    suffix = uuid.uuid4().hex[:12]
    prefix = task.lower().replace("-", "")
    # The shared database retains the historical prefix required by the
    # unchanged HIT-68 adapter tests; task and sandbox names identify this run.
    name, network, database = prefix + "-pg-" + suffix, prefix + "-net-" + suffix, "hit68_test_" + suffix
    result = {
        "task": "HIT-82" if args.measure_baseline else task, "schema_version": 1, "started_at_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "source_commit": run(["git", "rev-parse", "HEAD"]).stdout.strip(),
        "source_tree_sha256": source_digest(), "go_version": version,
        "platform": platform.platform(), "cpu_count": os.cpu_count(),
        "execution_limits": {"GOMAXPROCS": 1, "GOGC": 50, "go_build_parallelism": 1,
                             "postgres_memory_mib": 256, "postgres_cpus": 0.5},
        "postgres_image": IMAGE, "iam_sql_sha256": SQL_SHA256,
        "scope": ("Local protected GET /api/v1/profile over real TLS 1.3 mutual authentication, PostgreSQL and owner-only profile files; "
                  "mandatory controlled actual-response-start gate, not the HIT-69 distributed coordinator or a deployed service"
                  if profile_checks else "Local current authorization with controlled caller, owner-resource and actual-start gate ports, "
                  "real PostgreSQL authority and existing session consumer; not a live listener or distributed coordinator"
                  if authorization_checks else "Isolated IAM schema migration, role permissions, row isolation, constraints and transaction checks, "
                  "plus the existing session lookup and consumer; not live IAM, login, trusted time or deployment qualification"
                  if schema_checks else "Isolated adapter and existing consumer tests, not login, authorization, trusted time or deployment qualification"),
        "checks": [], "state": "FAILED",
    }
    network_created, container_started = False, False

    def check_command(label, command, *, data=None, env=None):
        started = time.monotonic()
        completed = run(command, env=env, data=data, check=False)
        log = completed.stdout + completed.stderr
        (evidence / (label + ".log")).write_text(log)
        command_record = ["go", *command[1:]] if command[0] == args.go else command
        check = {"name": label, "command": command_record, "exit_code": completed.returncode,
                 "elapsed_seconds": round(time.monotonic() - started, 3),
                 "log": label + ".log", "log_sha256": hashlib.sha256(log.encode()).hexdigest()}
        if data is not None:
            check["stdin_sha256"] = hashlib.sha256(data.encode()).hexdigest()
        result["checks"].append(check)
        print(json.dumps(check), flush=True)
        if completed.returncode:
            raise RuntimeError(label + " check failed; inspect the saved log")
        return completed

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
        server = run(psql + ["-Atc", "SHOW server_version"]).stdout.strip()
        if server != "18.7":
            raise RuntimeError("isolated PostgreSQL version does not match the accepted cohort")
        result["postgres_version"] = server
        address = run(["docker", "port", name, "5432/tcp"]).stdout.strip()
        if not address.startswith("127.0.0.1:") or "\n" in address:
            raise RuntimeError("test database was not bound exclusively to loopback")
        environment["HITPIPE_IAM_TEST_ADMIN_DSN"] = f"postgres://postgres@{address}/{database}?sslmode=disable"
        environment["HITPIPE_IAM_TEST_DSN"] = f"postgres://hp_hit68_lookup@{address}/{database}?sslmode=disable"
        if schema_checks:
            bootstrap = (MIGRATIONS / "bootstrap_roles.sql").read_text()
            check_command("bootstrap-roles", psql, data=bootstrap)
            logins = ""
            for login, group in (("migration", "migrator"), ("app", "app"), ("worker", "worker"), ("purge", "purge")):
                logins += (f"CREATE ROLE hp_hit81_{login} LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE "
                           f"NOBYPASSRLS IN ROLE hp_iam_{group};\n")
            check_command("test-logins", psql, data=logins)
            with socket.create_connection(("127.0.0.1", int(address.split(":")[1])), timeout=5):
                result["host_loopback_tcp_probe"] = {"host": "127.0.0.1", "state": "PASSED"}
            migration_psql = ["docker", "exec", "-i", name, "psql", "-X", "-v", "ON_ERROR_STOP=1",
                              "-h", "127.0.0.1", "-U", "hp_hit81_migration", "-d", database]
            role_check = check_command("migration-role-tcp", migration_psql + ["-Atc",
                "SET ROLE hp_iam_migrator; SELECT current_user,session_user;"])
            if role_check.stdout.splitlines()[-1:] != ["hp_iam_migrator|hp_hit81_migration"]:
                raise RuntimeError("migration login could not select the dedicated non-login owner")
            startup_check = ["docker", "exec", "-i", "--env", "PGOPTIONS=-c role=hp_iam_migrator", name,
                             *migration_psql[4:], "-Atc", "SELECT current_user,session_user;"]
            check_command("migration-startup-role", startup_check)
            # pgx PostgreSQL URI options preserve '+' literally; encode spaces as %20.
            migration_options = urlencode({"sslmode": "disable", "options": "-c role=hp_iam_migrator"}, quote_via=quote)
            environment["HITPIPE_IAM_MIGRATOR_DSN"] = f"postgres://hp_hit81_migration@{address}/{database}?{migration_options}"
            for role in ("APP", "WORKER", "PURGE"):
                environment["HITPIPE_IAM_TEST_" + role + "_DSN"] = f"postgres://hp_hit81_{role.lower()}@{address}/{database}?sslmode=disable"
            for label, expected in (("migration-install", "installed"), ("migration-repeat", "already_installed")):
                completed = check_command(label, [args.go, "run", "-p", "1", "./cmd/iam-migrate"], env=environment)
                try:
                    state = json.loads(completed.stdout)
                except (ValueError, TypeError):
                    raise RuntimeError(label + " did not emit the documented migration receipt") from None
                if (not isinstance(state, dict) or state.get("state") != expected or state.get("version") != 1 or
                        state.get("sql_sha256") != SQL_SHA256):
                    raise RuntimeError(label + " did not confirm the exact expected migration state")
        else:
            bootstrap = "\n".join(
                f"CREATE ROLE {role} NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;"
                for role in ("hp_iam_app", "hp_iam_worker", "hp_iam_purge")
            )
            bootstrap += "\n" + SQL.read_text() + "\n"
            check_command("legacy-schema-fixture", psql, data=bootstrap)
        reader_role = "CREATE ROLE hp_hit68_lookup LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS IN ROLE hp_iam_app;\n"
        reader_role += "ALTER ROLE hp_hit68_lookup SET default_transaction_read_only=on;\n"
        check_command("read-only-lookup", psql, data=reader_role)
        if args.measure_baseline:
            environment["HITPIPE_IAM_BENCH_OUTPUT_DIR"] = str(evidence / "race-samples")
        commands = [
            ("unit", [args.go, "test", "-p", "1", "-count=1", "./..."]),
            ("postgres-race", [args.go, "test", "-p", "1", "-tags=integration", "-race", "-count=1", "-v", "./..."]),
            ("vet", [args.go, "vet", "-p", "1", "-tags=integration", "./..."]),
            ("build", [args.go, "build", "-p", "1", "./..."]),
        ]
        for label, command in commands:
            completed = check_command(label, command, env=environment)
            log = completed.stdout + completed.stderr
            if label == "postgres-race" and ("--- SKIP: TestPostgres" in log or "--- PASS: TestPostgres" not in log):
                raise RuntimeError("required real PostgreSQL tests did not run")
            if schema_checks and label == "postgres-race" and ("--- SKIP: TestIAMSchema" in log or "--- PASS: TestIAMSchema" not in log):
                raise RuntimeError("required IAM schema and permission tests did not run")
            if authorization_checks and label == "postgres-race" and ("--- SKIP: TestPostgresAuthorization" in log or "--- PASS: TestPostgresAuthorization" not in log):
                raise RuntimeError("required current authorization PostgreSQL tests did not run")
            if profile_checks and label == "postgres-race":
                for required in ("TestPostgresProfile", "TestProfileHTTPSPostgresAndOwnerFiles"):
                    if "--- SKIP: " + required in log or "--- PASS: " + required not in log:
                        raise RuntimeError("required protected profile composition tests did not run: " + required)
        if args.measure_baseline:
            from measure_session_lookup import measure
            measure(args.go, environment, evidence, args.measure_baseline)
            result["measurement"] = "measurement.json"
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
        if container_started:
            diagnostic_command = ["docker", "logs", "--tail", "30", name]
            diagnostic = run(diagnostic_command, check=False)
            log = diagnostic.stdout + diagnostic.stderr
            (evidence / "postgres-failure.log").write_text(log)
            result["failure_diagnostics"] = {
                "command": diagnostic_command, "exit_code": diagnostic.returncode,
                "log": "postgres-failure.log", "log_sha256": hashlib.sha256(log.encode()).hexdigest(),
            }
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
