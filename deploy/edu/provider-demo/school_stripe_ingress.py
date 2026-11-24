"""Relay one public Stripe webhook route to the dedicated school workspace."""

import argparse
from contextlib import closing, contextmanager
from dataclasses import dataclass
from http.client import HTTPConnection, HTTPException
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from ipaddress import IPv4Address, IPv4Network
import json
import os
from pathlib import Path
import re
import socket
import stat
import struct
from threading import BoundedSemaphore, Timer


BRIDGE = "172.18.0.1"
PUBLIC_PATH = "/webhooks/stripe/demo"
SOCKET_NAME = "stripe-webhook.sock"
PRIVATE_HOST = "school-stripe-ingress"
MAX_BODY = 256 * 1024  # Intake normalizesignal.MaxBodyBytes.
MAX_SIGNATURE = 2048
MAX_METADATA = 8192
REQUEST_TIMEOUT = 12
INTAKE_TIMEOUT = 8
UUID = re.compile(r"[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}")
STRIPE_ACCOUNT = re.compile(r"acct_[a-zA-Z0-9]{1,64}")
HOST = re.compile(r"(?=.{1,253}\Z)(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}")
INTAKE_FAILURES = {400, 401, 403, 404, 409, 412, 413, 429, 503}


class Rejected(Exception):
    def __init__(self, status):
        self.status = status


def private_file(directory, name):
    flags = os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK
    with os.fdopen(os.open(name, flags, dir_fd=directory), "rb") as source:
        info = os.fstat(source.fileno())
        if (not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or info.st_nlink != 1
                or stat.S_IMODE(info.st_mode) != 0o600 or not 0 < info.st_size <= MAX_METADATA):
            raise ValueError("Unexpected Intake readiness file ownership, mode or size")
        data = source.read(MAX_METADATA + 1)
        if len(data) != info.st_size:
            raise ValueError("Intake readiness file changed while reading")
        return data


def unique_object(pairs):
    obj = dict(pairs)
    if len(obj) != len(pairs):
        raise ValueError("Duplicate Intake readiness fields")
    return obj


@dataclass(frozen=True)
class IntakeBinding:
    workspace: str
    share: Path
    source_account: str

    def __post_init__(self):
        if not UUID.fullmatch(self.workspace) or not STRIPE_ACCOUNT.fullmatch(self.source_account):
            raise ValueError("A canonical workspace UUID and Stripe account ID are required")
        if not self.share.is_absolute() or ".." in self.share.parts:
            raise ValueError("An absolute private workspace share is required")

    @contextmanager
    def destination(self):
        # Pin one directory generation through a broker's atomic current-link
        # rotation so the readiness manifest and socket cannot mix generations.
        directory = os.open(self.share, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        try:
            info = os.fstat(directory)
            if info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) != 0o700:
                raise ValueError("Intake readiness share must be owner-only")
            ready = json.loads(private_file(directory, "ready.json"), object_pairs_hook=unique_object)
            if not isinstance(ready, dict) or ready.get("workspaceId") != self.workspace:
                raise ValueError("Intake readiness belongs to another workspace")
            demo = ready.get("providerDemo")
            if not isinstance(demo, dict) or demo.get("sourceAccount") != self.source_account:
                raise ValueError("Intake readiness belongs to another Stripe account")
            entry = os.stat(SOCKET_NAME, dir_fd=directory, follow_symlinks=False)
            if (not stat.S_ISSOCK(entry.st_mode) or entry.st_uid != os.getuid()
                    or stat.S_IMODE(entry.st_mode) != 0o600):
                raise ValueError("Intake relay must be an owner-only socket in the selected share")
            yield f"/proc/self/fd/{directory}/{SOCKET_NAME}"
        finally:
            os.close(directory)

    def deliver(self, signature, content_type, body):
        with self.destination() as socket_path, socket.socket(socket.AF_UNIX) as transport:
            transport.settimeout(INTAKE_TIMEOUT)
            transport.connect(socket_path)
            credentials = transport.getsockopt(socket.SOL_SOCKET, socket.SO_PEERCRED, struct.calcsize("3i"))
            pid, uid, _gid = struct.unpack("3i", credentials)
            if pid <= 0 or uid != os.getuid():
                raise ValueError("Intake relay is outside the configured owner boundary")
            with closing(HTTPConnection(PRIVATE_HOST, timeout=INTAKE_TIMEOUT)) as client:
                client.sock = transport
                client.request("POST", PUBLIC_PATH, body=body, headers={
                    "Content-Type": content_type,
                    "Stripe-Signature": signature,
                    "Connection": "close",
                })
                with closing(client.getresponse()) as result:
                    if result.status == 204 or result.status in INTAKE_FAILURES:
                        return result.status
                    return 503


def one_header(headers, name):
    values = headers.get_all(name, [])
    if len(values) > 1 or (values and not re.fullmatch(r"[ -~]*", values[0])):
        raise Rejected(400)
    return values[0] if values else None


def make_server(address, connectors, public_host, intake):
    if not HOST.fullmatch(public_host):
        raise ValueError("A canonical public hostname is required")

    class IngressServer(ThreadingHTTPServer):
        capacity = BoundedSemaphore(4)

        def verify_request(self, _connection, client):
            return client[0] in connectors

        def process_request(self, connection, client):
            if not self.capacity.acquire(blocking=False):
                self.shutdown_request(connection)
                return
            try:
                super().process_request(connection, client)
            except BaseException:
                self.capacity.release()
                raise

        def process_request_thread(self, connection, client):
            try:
                super().process_request_thread(connection, client)
            finally:
                self.capacity.release()

        def handle_error(self, _request, _client):
            pass

    class Handler(BaseHTTPRequestHandler):
        def setup(self):
            self.request.settimeout(REQUEST_TIMEOUT)
            super().setup()

        def handle(self):
            # An absolute deadline applies even when the peer trickles bytes.
            def abort():
                try:
                    self.request.shutdown(socket.SHUT_RDWR)
                except OSError:
                    pass

            deadline = Timer(REQUEST_TIMEOUT, abort)
            deadline.daemon = True
            deadline.start()
            try:
                super().handle()
            finally:
                deadline.cancel()

        def log_message(self, *_args):
            pass

        def respond(self, status):
            self.send_response_only(status)
            self.send_header("Cache-Control", "no-store")
            if status == 405:
                self.send_header("Allow", "POST")
            if status != 204:
                self.send_header("Content-Length", "0")
            self.end_headers()

        def handle_route(self):
            try:
                if one_header(self.headers, "Host") != public_host:
                    raise Rejected(421)
                if self.requestline.split()[1] != PUBLIC_PATH or self.path != PUBLIC_PATH:
                    raise Rejected(404)
                if self.command != "POST":
                    raise Rejected(405)
                if any(self.headers.get_all(name) for name in ("Transfer-Encoding", "Content-Encoding", "Expect")):
                    raise Rejected(400)
                length = one_header(self.headers, "Content-Length")
                if length is None:
                    raise Rejected(411)
                if not re.fullmatch(r"[0-9]{1,8}", length) or not (size := int(length)):
                    raise Rejected(400)
                if size > MAX_BODY:
                    raise Rejected(413)
                content_type = one_header(self.headers, "Content-Type")
                if not content_type or content_type.partition(";")[0].strip().lower() != "application/json":
                    raise Rejected(400)
                signature = one_header(self.headers, "Stripe-Signature")
                if not signature:
                    raise Rejected(401)
                if len(signature) > MAX_SIGNATURE:
                    raise Rejected(413)
                # A connector may comma-fold duplicate field lines. Rotation
                # can add v1 signatures, but Stripe supplies one timestamp.
                if sum(part.partition("=")[0].strip() == "t" for part in signature.split(",")) != 1:
                    raise Rejected(401)
                body = self.rfile.read(size)
                if len(body) != size:
                    raise Rejected(400)
                status = intake.deliver(signature, content_type, body)
            except Rejected as error:
                status = error.status
            except (OSError, HTTPException, ValueError):
                status = 503
            self.respond(status)

        do_POST = do_GET = do_HEAD = do_DELETE = do_PUT = do_PATCH = do_OPTIONS = do_TRACE = do_CONNECT = handle_route

    return IngressServer(address, Handler)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", required=True)
    parser.add_argument("--connector-ip", required=True, action="append", type=IPv4Address)
    parser.add_argument("--workspace", required=True)
    parser.add_argument("--share", required=True, type=Path)
    parser.add_argument("--source-account", required=True)
    parser.add_argument("--port", default=19481, type=int)
    args = parser.parse_args()
    bridge = IPv4Network("172.18.0.0/16")
    reserved = {IPv4Address(BRIDGE), bridge.network_address, bridge.broadcast_address}
    if any(ip not in bridge or ip in reserved for ip in args.connector_ip) or not 1024 <= args.port <= 65535:
        parser.error("A private bridge connector and unprivileged listener are required")
    try:
        binding = IntakeBinding(args.workspace, args.share, args.source_account)
        server = make_server((BRIDGE, args.port), set(map(str, args.connector_ip)), args.host, binding)
    except ValueError as error:
        parser.error(str(error))
    with server:
        server.serve_forever()


if __name__ == "__main__":
    main()
