#!/usr/bin/env python3
"""HIT-82: сравнение чтения сессии в одной одноразовой локальной базе.

Запускать после фиксации исходников: --baseline <локальная рабочая копия>.
Внешние адреса БД не принимаются. Изоляцию, проверки и удаление базы выполняет
check_postgres_session_lookup.py; этот модуль только измеряет внутри её окружения.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess
import sys
import time
from urllib.parse import urlparse

IAM = Path(__file__).resolve().parents[1]
PACKAGE = "./internal/adapters/postgres/sessionlookup"
BENCHMARKS = "^Benchmark(ReferenceDecode|PostgresLookup)$"
BASELINE_LOOKUP_SHA256 = "9c13c8261d0752bd6c558503cae635d76f5ae47fc97d130bad5a7c4d661a8379"


def sha256(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def source_digest(directory):
    digest = hashlib.sha256()
    for path in sorted(directory.rglob("*")):
        relative = path.relative_to(directory)
        if not path.is_file() or relative.parts[0] == "evidence" or "__pycache__" in relative.parts:
            continue
        digest.update(str(relative).encode() + b"\0" + path.read_bytes() + b"\0")
    return digest.hexdigest()


def git(directory, *arguments):
    return subprocess.run(["git", *arguments], cwd=directory, text=True,
                          capture_output=True, check=True).stdout.strip()


def measure(go, environment, evidence, baseline):
    """Keep both variants and all subprocesses in the existing synthetic sandbox."""
    baseline = Path(baseline).resolve()
    baseline_iam = baseline / "services/iam" if (baseline / "services/iam").is_dir() else baseline
    if baseline_iam == IAM or not (baseline_iam / "go.mod").is_file():
        raise RuntimeError("baseline must be a different local IAM module worktree")
    if (baseline_iam / "go.mod").read_bytes() != (IAM / "go.mod").read_bytes():
        raise RuntimeError("baseline and candidate must use identical Go module dependencies")
    baseline_lookup = baseline_iam / "internal/adapters/postgres/sessionlookup/lookup.go"
    if sha256(baseline_lookup) != BASELINE_LOOKUP_SHA256:
        raise RuntimeError("baseline production lookup.go differs from accepted HIT-68")
    # This helper is intentionally unusable with an arbitrary production DSN.
    databases = set()
    for key in ("HITPIPE_IAM_TEST_DSN", "HITPIPE_IAM_TEST_ADMIN_DSN"):
        address = urlparse(environment.get(key, ""))
        if (address.scheme != "postgres" or address.hostname != "127.0.0.1"
                or not address.port or not address.path.startswith("/hit68_test_")
                or address.password is not None):
            raise RuntimeError("measurement requires the runner's disposable loopback database")
        databases.add((address.hostname, address.port, address.path))
    if len(databases) != 1:
        raise RuntimeError("measurement connections must address the same disposable database")
    evidence = Path(evidence).resolve()
    evidence.mkdir(parents=True, exist_ok=True)
    variants = {"baseline": baseline_iam, "candidate": IAM}
    before = {name: source_digest(directory) for name, directory in variants.items()}
    cpu = next((line.split(":", 1)[1].strip() for line in
                Path("/proc/cpuinfo").read_text().splitlines() if line.startswith("model name")), "unknown")
    result = {
        "task": "HIT-82", "schema_version": 1, "state": "FAILED",
        "started_at_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "host": {"platform": platform.platform(), "machine": platform.machine(),
                 "cpu_model": cpu, "logical_cpu_count": os.cpu_count(),
                 "load_average_start": list(os.getloadavg())},
        "comparison": {"repetitions_per_variant": 5,
                       "iteration_order": ["baseline,candidate", "candidate,baseline",
                                           "baseline,candidate", "candidate,baseline", "baseline,candidate"],
                       "bench_iterations": 1000, "profile_iterations": 3000,
                       "go_test_parallelism": 1, "GOMAXPROCS": environment.get("GOMAXPROCS"),
                       "GOGC": environment.get("GOGC"), "database": "same disposable synthetic fixture",
                       "warmup": "Shared benchmark/test harness; no unrecorded prewarming runs",
                       "timing_claim": "Measurements on a shared limited host, not a production throughput guarantee"},
        "variants": {}, "checks": [],
    }
    for name, directory in variants.items():
        harness = {
            str(path.relative_to(directory)): sha256(path)
            for path in sorted((directory / "internal/adapters/postgres/sessionlookup").glob("*test.go"))
            if "BenchmarkReferenceDecode" in path.read_text() or "TestHIT82LatencySamples" in path.read_text()
        }
        if not harness:
            raise RuntimeError("shared HIT-82 benchmark harness is missing from " + name)
        result["variants"][name] = {
            "source_commit": git(directory, "rev-parse", "HEAD"),
            "source_tree_sha256": before[name], "working_tree_changes": git(directory, "status", "--short", "--", "."),
            "lookup_sha256": sha256(directory / "internal/adapters/postgres/sessionlookup/lookup.go"),
            "harness_sha256": harness,
            "test_files_sha256": {
                str(path.relative_to(directory)): sha256(path)
                for path in sorted((directory / "internal/adapters/postgres/sessionlookup").glob("*test.go"))
            },
        }
    if result["variants"]["baseline"]["harness_sha256"] != result["variants"]["candidate"]["harness_sha256"]:
        raise RuntimeError("baseline and candidate must run the exact same shared benchmark harness")

    def execute(label, name, command, extra_environment=None):
        current_environment = environment.copy()
        if extra_environment:
            current_environment.update(extra_environment)
        started = time.monotonic()
        timed_out = False
        try:
            completed = subprocess.run(command, cwd=variants[name], env=current_environment,
                                       text=True, capture_output=True, timeout=300, check=False)
            stdout, stderr, exit_code = completed.stdout, completed.stderr, completed.returncode
        except subprocess.TimeoutExpired as exc:
            timed_out = True
            stdout, stderr, exit_code = exc.stdout or "", exc.stderr or "", None
            stdout = stdout.decode(errors="replace") if isinstance(stdout, bytes) else stdout
            stderr = stderr.decode(errors="replace") if isinstance(stderr, bytes) else stderr
        log = stdout + stderr
        (evidence / (label + ".stdout.log")).write_text(stdout)
        (evidence / (label + ".stderr.log")).write_text(stderr)
        (evidence / (label + ".log")).write_text(log)
        check = {"name": label, "variant": name, "command": [str(value) for value in command],
                 "exit_code": exit_code, "timed_out": timed_out,
                 "elapsed_seconds": round(time.monotonic() - started, 3),
                 "stdout": label + ".stdout.log", "stderr": label + ".stderr.log",
                 "log": label + ".log", "log_sha256": hashlib.sha256(log.encode()).hexdigest()}
        result["checks"].append(check)
        print(json.dumps(check), flush=True)
        if timed_out:
            raise RuntimeError(label + " timed out; partial output is saved")
        if exit_code:
            raise RuntimeError(label + " failed; inspect the saved log")
        return log

    try:
        for iteration in range(5):
            order = ("baseline", "candidate") if iteration % 2 == 0 else ("candidate", "baseline")
            for name in order:
                log = execute(f"bench-{iteration + 1:02d}-{name}", name,
                              [go, "test", "-p", "1", "-tags=integration", "-run", "^$",
                               "-bench", BENCHMARKS, "-benchtime=1000x", "-benchmem", "-count=1", PACKAGE])
                if "BenchmarkReferenceDecode" not in log or "BenchmarkPostgresLookup" not in log:
                    raise RuntimeError("both required benchmarks must execute without omission")
        for name in variants:
            output = evidence / name
            output.mkdir(exist_ok=True)
            log = execute("latency-" + name, name,
                          [go, "test", "-p", "1", "-tags=integration", "-run", "^TestHIT82LatencySamples$",
                           "-count=1", "-v", PACKAGE], {"HITPIPE_IAM_BENCH_OUTPUT_DIR": str(output)})
            if "--- PASS: TestHIT82LatencySamples" not in log or "--- SKIP:" in log:
                raise RuntimeError("required latency sample test did not execute")
            binary, cpu_profile, memory_profile = output / "profile.test", output / "cpu.pprof", output / "memory.pprof"
            try:
                log = execute("profile-" + name, name,
                              [go, "test", "-p", "1", "-tags=integration", "-run", "^$",
                               "-bench", BENCHMARKS, "-benchtime=3000x", "-benchmem", "-count=1",
                               "-cpuprofile", str(cpu_profile), "-memprofile", str(memory_profile),
                               "-memprofilerate=1", "-o", str(binary), PACKAGE])
                if "BenchmarkReferenceDecode" not in log or "BenchmarkPostgresLookup" not in log:
                    raise RuntimeError("profile workload omitted a required benchmark")
                execute("cpu-top-" + name, name,
                        [go, "tool", "pprof", "-top", "-nodecount=30", str(binary), str(cpu_profile)])
                execute("allocation-top-" + name, name,
                        [go, "tool", "pprof", "-top", "-alloc_space", "-nodecount=30", str(binary), str(memory_profile)])
            finally:
                binary.unlink(missing_ok=True)
            result["variants"][name]["profiles"] = {
                "cpu": str(cpu_profile.relative_to(evidence)), "cpu_sha256": sha256(cpu_profile),
                "memory": str(memory_profile.relative_to(evidence)), "memory_sha256": sha256(memory_profile),
            }
        if any(source_digest(directory) != before[name] for name, directory in variants.items()):
            raise RuntimeError("source files changed during the measurement")
        result["state"] = "PASSED"
    except (OSError, RuntimeError, subprocess.SubprocessError) as exc:
        result["failure"] = str(exc) if isinstance(exc, RuntimeError) else type(exc).__name__
        raise
    finally:
        result["host"]["load_average_finish"] = list(os.getloadavg())
        result["finished_at_utc"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        (evidence / "measurement.json").write_text(json.dumps(result, indent=2) + "\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--baseline", type=Path, required=True, help="Local baseline worktree, not a database URL")
    parser.add_argument("--go", default="go", help="Accepted Go 1.27.2 binary")
    parser.add_argument("--evidence-dir", type=Path, default=IAM / "evidence/HIT-82")
    args = parser.parse_args()
    return subprocess.run([sys.executable, str(Path(__file__).with_name("check_postgres_session_lookup.py")),
                           "--go", args.go, "--evidence-dir", str(args.evidence_dir),
                           "--measure-baseline", str(args.baseline)], check=False).returncode


if __name__ == "__main__":
    sys.exit(main())
