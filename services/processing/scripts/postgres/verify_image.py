#!/usr/bin/env python3
"""Build, inspect, load and launch the Processing image on an owned TEST stand."""
import argparse
import hashlib
import json
import os
import pathlib
import re
import tarfile
import tempfile
import time

from verify import IMAGE, REPO, Run, now

DOCKERFILE = "services/processing/Dockerfile"
CONTENT = "/var/lib/hitpipe/processing-content"
CONTROL = "/tmp/hit145-control"
SOCKET = "/var/run/postgresql"
CONFIG = "/hit145/config"
UID = "10001:10001"


def sha(data):
    return "sha256:" + hashlib.sha256(data).hexdigest()


def image_identity(archive, revision, architecture):
    with tarfile.open(archive) as image:
        def blob(descriptor):
            digest = descriptor["digest"]
            algorithm, hexdigest = digest.split(":")
            if algorithm != "sha256" or not re.fullmatch(r"[0-9a-f]{64}", hexdigest):
                raise ValueError(digest)
            with image.extractfile("blobs/sha256/" + hexdigest) as stream:
                data = stream.read()
            if sha(data) != digest or len(data) != descriptor["size"]:
                raise AssertionError("descriptor mismatch: " + digest)
            return data

        with image.extractfile("index.json") as stream:
            index = json.load(stream)
        manifest_descriptor, = index["manifests"]
        manifest = json.loads(blob(manifest_descriptor))
        config = json.loads(blob(manifest["config"]))
        layer, = manifest["layers"]
        import io
        with tarfile.open(fileobj=io.BytesIO(blob(layer))) as rootfs:
            binary, = rootfs.getmembers()
            if binary.name.lstrip("./") != "processing" or not binary.isfile() or not binary.mode & 0o111:
                raise AssertionError("the runtime layer must contain only the executable")
            with rootfs.extractfile(binary) as stream:
                binary_sha = sha(stream.read())
    settings = config["config"]
    if (config["os"], config["architecture"], settings["User"], settings["Entrypoint"],
            settings["Cmd"], settings["Labels"]["org.opencontainers.image.revision"]) != (
                "linux", architecture, UID, ["/processing"], ["-config", "/etc/processing/processing.json"], revision):
        raise AssertionError("unexpected runtime image configuration")
    with archive.open("rb") as stream:
        archive_sha = "sha256:" + hashlib.file_digest(stream, "sha256").hexdigest()
    return {"source_sha": revision, "platform": "linux/" + architecture,
            "manifest_digest": manifest_descriptor["digest"], "manifest_media_type": manifest["mediaType"],
            "config_digest": manifest["config"]["digest"],
            "layer_digest": layer["digest"], "files": [binary.name], "binary_sha256": binary_sha,
            "archive_sha256": archive_sha, "config": settings}


class ImageRun(Run):
    def __init__(self, output, go, fixture):
        super().__init__(output)
        self.go = go
        self.fixture = fixture
        self.name = self.name.replace("hit76-", "hit145-", 1)
        self.db = "hit78"  # Existing isolated Go fixtures use this database name.
        self.tag = self.name + ":test"
        self.volumes = {target: f"{self.name}-{name}" for target, name in
                        ((SOCKET, "socket"), (CONFIG, "config"), (CONTENT, "content"))}
        self.owned_volumes = []
        self.app_name = None
        self.identity = {}
        self.script_sha256 = hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()

    def docker(self, label, *args, required=True):
        return self.command(label, ["docker", *args], required=required)

    def volume(self, target, readonly=False):
        mount = f"type=volume,source={self.volumes[target]},target={target}"
        return ["--mount", mount + (",readonly" if readonly else "")]

    def command(self, label, args, stdin=None, required=True):
        if label == "container-create":
            for volume in self.volumes.values():
                self.docker("volume-create", "volume", "create", "--label", "hitpipe.task=HIT-145", volume)
                self.owned_volumes.append(volume)
            args = list(args)
            index = args.index("/var/run/postgresql:rw") - 1
            args[index:index + 2] = [*self.volume(SOCKET), *self.volume(CONFIG), *self.volume(CONTENT),
                                    "--mount", f"type=bind,source={self.fixture},target=/test,readonly"]
            args[args.index("hitpipe.task=HIT-76")] = "hitpipe.task=HIT-145"
        return super().command(label, args, stdin=stdin, required=required)

    def run(self):
        if self.tree_status:
            raise RuntimeError("run from a clean checkout and write evidence outside the checkout")
        if os.environ.get("DOCKER_HOST"):
            raise RuntimeError("unset DOCKER_HOST")
        context = json.loads(self.docker("build-context", "context", "inspect").stdout)[0]
        if not context["Endpoints"]["docker"]["Host"].startswith("unix://"):
            raise RuntimeError("only the local Unix-socket context is supported")
        architecture = self.docker("engine-arch", "info", "--format", "{{.Architecture}}").stdout.decode().strip()
        architecture = {"aarch64": "arm64", "x86_64": "amd64"}.get(architecture, architecture)
        if architecture not in {"arm64", "amd64"}:
            raise RuntimeError("unsupported test host architecture: " + architecture)
        toolchain = ["env", "GOTOOLCHAIN=local", "GOWORK=off", self.go]
        if self.command("go-version", [*toolchain, "version"]).stdout.split()[2] != b"go1.27.2":
            raise RuntimeError("the test driver requires Go 1.27.2")
        archive = self.output / "processing-image.tar"
        meta = self.output / "build-metadata.json"
        try:
            self.docker("buildx-version", "buildx", "version")
            self.docker("build-image", "buildx", "build", "--load", "--platform", "linux/" + architecture,
                        "--provenance=false", "--sbom=false", "--build-arg", "SOURCE_SHA=" + self.source,
                        "--build-arg", "SOURCE_DATE_EPOCH=0", "--metadata-file", str(meta),
                        "--file", DOCKERFILE, "--tag", self.tag, ".")
            self.docker("save-image", "image", "save", "--output", str(archive), self.tag)
            self.identity = image_identity(archive, self.source, architecture)
            metadata = json.loads(meta.read_text())
            if metadata["containerimage.digest"] != self.identity["manifest_digest"]:
                raise AssertionError("build and archived manifests differ")
            (self.output / "image-identity.json").write_text(json.dumps(self.identity, indent=2) + "\n")
            self.docker("remove-built-tag", "image", "rm", self.tag)
            self.docker("load-image", "image", "load", "--input", str(archive))
            launched_id = self.docker("loaded-config-id", "image", "inspect", "--format", "{{.Id}}", self.tag)
            if launched_id.stdout.decode().strip() != self.identity["config_digest"]:
                raise AssertionError("the imported image configuration differs")
            self.command("build-test-driver",
                         ["env", "GOOS=linux", "GOARCH=" + architecture, "CGO_ENABLED=0",
                          *toolchain, "-C", "services/processing", "test", "-c",
                          "-o", str(self.fixture / "storage.test"), "./internal/adapters/postgres"])
            self.main()
        finally:
            for volume in reversed(self.owned_volumes):
                self.docker("volume-cleanup", "volume", "rm", volume)
                if self.docker("volume-cleanup-probe", "volume", "inspect", volume, required=False).returncode == 0:
                    raise AssertionError("owned volume still exists: " + volume)
            self.docker("image-cleanup", "image", "rm", self.tag, required=False)
        runtime_summary = {"status": "PASS", "image": self.identity, "postgres": IMAGE,
                           "started_from": "image loaded from the inspected archive",
                           "tests": "CLOSED, forbidden Health, mTLS, READY, restart, durable idempotence",
                           "finished_at": now()}
        (self.output / "summary.json").write_text(json.dumps(runtime_summary, indent=2) + "\n")

    def await_file(self, name):
        for _ in range(120):
            found = self.docker("await-" + name, "exec", self.container, "test", "-f",
                                CONTROL + "/" + name, required=False)
            if found.returncode == 0:
                return self.docker(name, "exec", self.container, "cat", CONTROL + "/" + name).stdout
            if name != "status":
                done = self.docker("early-driver-status", "exec", self.container, "test", "-f",
                                   CONTROL + "/status", required=False)
                if done.returncode == 0:
                    break
            time.sleep(0.15)
        raise RuntimeError("fixture did not publish " + name)

    def start_app(self, number, paths):
        for path in paths.values():
            if not re.fullmatch(CONFIG + r"/fixture/[a-zA-Z0-9._/-]+", path) or ".." in pathlib.PurePath(path).parts:
                raise ValueError("unexpected fixture path")
        self.app_name = f"{self.name}-processing-{number}"
        self.docker("processing-start-" + number, "run", "--detach", "--name", self.app_name,
                    "--pull=never", "--label", "hitpipe.task=HIT-145", "--network", "container:" + self.container,
                    "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
                    *self.volume(CONFIG, readonly=True), *self.volume(SOCKET, readonly=True), *self.volume(CONTENT),
                    self.tag, "-config", paths["config"], "-stand-config", paths["stand"])
        fields = ("Config.User", "HostConfig.ReadonlyRootfs", "HostConfig.PidMode", "HostConfig.Privileged",
                  "HostConfig.NetworkMode", "HostConfig.CapDrop", "HostConfig.SecurityOpt", "Mounts", "Image")
        template = "\n".join("{{json ." + field + "}}" for field in fields)
        raw = self.docker("processing-isolation-" + number, "inspect", "--format", template, self.app_name).stdout
        user, readonly, pid, privileged, network, caps, sec, mounts, image = map(json.loads, raw.splitlines())
        actual = {mount["Destination"]: (mount["Type"], mount["Name"], mount["RW"]) for mount in mounts}
        wanted = {target: ("volume", volume, target == CONTENT) for target, volume in self.volumes.items()}
        if (user != UID or not readonly or pid == "host" or privileged or network != "container:" + self.container
                or caps != ["ALL"] or sec != ["no-new-privileges:true"] or actual != wanted
                or image != self.identity["config_digest"]):
            raise AssertionError("running container does not match the required isolation")
        top = self.docker("processing-real-uid-" + number, "top", self.app_name, "-eo", "uid,pid,comm")
        _, process = top.stdout.splitlines()
        if process.split()[0] != b"10001":
            raise AssertionError("the actual process UID is not 10001")

    def stop_app(self):
        name = self.app_name
        self.docker("processing-stop", "stop", "-t", "7", name)
        self.docker("processing-logs", "logs", name)
        state = self.docker("processing-exit", "inspect", "--format", "{{json .State}}", name).stdout
        if json.loads(state)["ExitCode"] != 0:
            raise AssertionError("Processing did not exit normally")
        self.docker("processing-cleanup", "rm", "-v", name)
        self.app_name = None

    def checks(self):
        self.docker("stand-directories", "exec", "--user", "0", self.container, "sh", "-c",
                    f"mkdir -p {CONTROL} {CONFIG}/fixture && chown {UID} {CONTROL} {CONFIG}/fixture {CONTENT}"
                    f" && chmod 0700 {CONTROL} {CONFIG}/fixture {CONTENT}")
        runner = (f"/test/storage.test -test.v -test.count=1 -test.timeout=90s "
                  f"-test.run=^TestProcessingOCIImageOwnedStand$ >{CONTROL}/driver.stdout 2>{CONTROL}/driver.stderr; "
                  f'status=$?; printf "%s\\n" "$status" >{CONTROL}/status.tmp && mv {CONTROL}/status.tmp {CONTROL}/status')
        self.docker("stand-driver", "exec", "--detach", "--user", UID, "--env", "HIT78_ISOLATED_PG=1",
                    "--env", "HIT145_SOURCE_SHA=" + self.source, "--env", "TMPDIR=" + CONFIG + "/fixture",
                    self.container, "sh", "-c", runner)
        try:
            paths = json.loads(self.await_file("ready.json"))
            if set(paths) != {"config", "stand"}:
                raise AssertionError("incorrect stand inputs")
            self.start_app("1", paths)
            self.await_file("restart.json")
            self.stop_app()
            self.start_app("2", paths)
            self.docker("stand-restarted", "exec", "--user", UID, self.container,
                        "touch", CONTROL + "/second.json")
            status = self.await_file("status")
            stdout = self.docker("driver-stdout", "exec", self.container, "cat", CONTROL + "/driver.stdout")
            if status.strip() != b"0" or b"RESTART PASS:" not in stdout.stdout:
                raise AssertionError("the image restart test failed")
            self.stop_app()
        finally:
            self.docker("driver-stderr", "exec", self.container, "cat", CONTROL + "/driver.stderr",
                        required=False)
            if self.app_name:
                self.docker("driver-incomplete", "exec", self.container, "cat", CONTROL + "/driver.stdout",
                            required=False)
                self.docker("processing-incomplete", "logs", self.app_name, required=False)
                self.docker("processing-forced-cleanup", "rm", "-f", "-v", self.app_name, required=False)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--go", required=True, type=pathlib.Path, help="absolute path to Go 1.27.2")
    parser.add_argument("--output", required=True, type=pathlib.Path, help="new directory outside the checkout")
    args = parser.parse_args()
    if not args.go.is_absolute():
        parser.error("--go must be absolute")
    with tempfile.TemporaryDirectory(prefix="hit145-") as scratch:
        fixture = pathlib.Path(scratch)
        fixture.chmod(0o755)
        ImageRun(args.output.resolve(), str(args.go), fixture).run()
