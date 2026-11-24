#!/usr/bin/env python3
"""Private provisioning API for verified HitPipe workspace owners."""
from argparse import ArgumentParser
from contextlib import closing, contextmanager
import fcntl
from hashlib import sha256
from http.server import BaseHTTPRequestHandler
import json
import os
from pathlib import Path
import re
import socket
from socketserver import ThreadingMixIn, UnixStreamServer
import sqlite3
import stat
import struct
import tempfile
import threading
import time

from provider_connections import Connections, ProviderError
from provider_egress import private_json

WORKSPACE = r"[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}"
SLUG = re.compile(r"t[0-9a-f]{15}\Z")
FIELDS = {"action", "stripeApiKey", "clickupToken"}
CHOICE = {"sourcePrice", "destinationWorkspace", "targetList", "assignee"}
MAX_BODY = 8192


def private_dir(path):
    result = path.lstat()
    if (not stat.S_ISDIR(result.st_mode) or result.st_uid != os.getuid()
            or stat.S_IMODE(result.st_mode) != 0o700):
        raise ValueError("Ожидается каталог, доступный только серверу HitPipe.")


def atomic(path, value):
    with tempfile.NamedTemporaryFile(dir=path.parent, delete=False) as output:
        temp = Path(output.name)
        try:
            output.write(value)
            output.flush()
            os.fsync(output.fileno())
            temp.replace(path)
            with_directory = os.open(path.parent, os.O_DIRECTORY | os.O_RDONLY | os.O_NOFOLLOW)
            try:
                os.fsync(with_directory)
            finally:
                os.close(with_directory)
        finally:
            temp.unlink(missing_ok=True)


def json_bytes(data):
    return json.dumps(data, ensure_ascii=False, separators=(",", ":")).encode()


class WorkspaceConnections:
    def __init__(self, registry, config_root, runtime_root, ready_root, public_origin, connections=Connections):
        self.registry = registry
        self.config_root = config_root
        self.runtime_root = runtime_root
        self.ready_root = ready_root
        self.origin = public_origin
        self.connections = connections

    def lookup(self, workspace):
        with closing(sqlite3.connect(self.registry.as_uri() + "?mode=ro", uri=True, timeout=3)) as db:
            result = db.execute("SELECT sandbox_slug FROM school_tenants WHERE workspace_id=?", (workspace,)).fetchall()
        if len(result) != 1 or not SLUG.fullmatch(result[0][0]):
            raise ProviderError("Рабочая область ещё не создана или недоступна.", status=404)
        return result[0][0]

    def directory(self, workspace, create=False):
        path = self.config_root / workspace
        if create:
            path.mkdir(mode=0o700, exist_ok=True)
        if path.exists():
            private_dir(path)
        return path

    def status(self, workspace, slug):
        path = self.directory(workspace)
        webhook_url = self.origin + "/webhooks/stripe/" + workspace
        try:
            record = private_json(path / "state.json")
        except FileNotFoundError:
            return {"state": "UNCONNECTED", "webhookUrl": webhook_url}
        binding = record["binding"]
        if record["workspace"] != workspace or record["webhookUrl"] != webhook_url:
            raise ValueError("Состояние подключения не принадлежит рабочей области.")
        provider_status = record["state"]
        if provider_status == "CONNECTING" and "stripeEndpoint" not in record:
            return {"state": "UNAVAILABLE", "webhookUrl": webhook_url,
                    "message": "Сохранён запрос настройки Stripe. Повторите его с тем же тарифом и списком."}
        bridge = self.runtime_root / ("d" + slug[1:]) / "current" / "bridge"
        ready = False
        try:
            owner = private_json(self.ready_root / slug / "gateway-ready.json")
            runtime = private_json(bridge / "ready.json")
            ready = (owner["workspace_id"] == runtime["workspaceId"] == workspace
                     and runtime.get("providerDemo") == binding
                     and (bridge / "stripe-webhook.sock").is_socket())
        except (OSError, ValueError, KeyError):
            pass
        state = "CONNECTED" if ready else "CONNECTING"
        message = ("Подключения проверены. Рабочая область готова." if ready
                   else "Провайдеры подключены. HitPipe запускает отдельную рабочую область.")
        return {"state": state, "binding": binding, "webhookUrl": webhook_url, "message": message}

    @contextmanager
    def lock(self, path):
        fd = os.open(path / ".connect.lock", os.O_CREAT | os.O_WRONLY | os.O_NOFOLLOW, 0o600)
        with os.fdopen(fd, "wb"):
            try:
                fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except BlockingIOError:
                raise ProviderError("Подключение уже выполняется. Обновите состояние.", status=409) from None
            yield

    def connect(self, workspace, slug, vendor, selection):
        binding = vendor.validate_selection(selection)
        path = self.directory(workspace, create=True)
        webhook_url = self.origin + "/webhooks/stripe/" + workspace
        with self.lock(path):
            try:
                saved = private_json(path / "state.json")
            except FileNotFoundError:
                saved = None
            if saved is not None and "stripeEndpoint" in saved:
                if saved["binding"] != binding:
                    raise ProviderError("У рабочей области уже есть подключение. Остановите связанную операцию перед изменением.", status=409)
                return self.status(workspace, slug)
            if saved is None:
                request_id = sha256(json_bytes([workspace, binding, webhook_url])).hexdigest()
                saved = {"state": "CONNECTING", "workspace": workspace, "binding": binding,
                         "request": request_id, "requested_at": int(time.time()), "webhookUrl": webhook_url}
                atomic(path / "state.json", json_bytes(saved))
            elif saved["binding"] != binding or saved["workspace"] != workspace or saved["webhookUrl"] != webhook_url:
                raise ProviderError("Дождитесь завершения настройки выбранной связки Stripe и ClickUp.", status=409)
            elif time.time() - saved["requested_at"] > 23 * 3600:
                raise ProviderError("Нужно восстановить незавершённый вебхук Stripe перед повторным созданием.", status=409)

            endpoint, signing_secret = vendor.webhook(webhook_url, "hitpipe-workspace-" + saved["request"])
            atomic(path / "provider-binding.json", json_bytes(binding))
            atomic(path / "credentials.json", json_bytes({
                "stripe_api_key": vendor.stripe_key, "clickup_token": vendor.clickup_key,
            }))
            atomic(path / "provider-webhook.secret", (signing_secret + "\n").encode())
            saved["stripeEndpoint"] = endpoint
            atomic(path / "state.json", json_bytes(saved))
        return self.status(workspace, slug)

    def request(self, workspace, method, data=None):
        slug = self.lookup(workspace)
        if method == "GET":
            return self.status(workspace, slug)
        if not isinstance(data, dict):
            raise ProviderError("Неподдерживаемые данные подключения.")
        action = data.get("action")
        if action == "inspect" and set(data) == FIELDS:
            vendor = self.connections(data["stripeApiKey"], data["clickupToken"])
            return vendor.inspect()
        if action == "connect" and set(data) == FIELDS | CHOICE:
            vendor = self.connections(data["stripeApiKey"], data["clickupToken"])
            return self.connect(workspace, slug, vendor, data)
        raise ProviderError("Неподдерживаемая операция подключения.")


class Handler(BaseHTTPRequestHandler):
    def setup(self):
        self.request.settimeout(45)
        super().setup()

    def log_message(self, *_):
        pass

    def invoke(self):
        peer = self.request.getsockopt(socket.SOL_SOCKET, socket.SO_PEERCRED, struct.calcsize("3i"))
        _, uid, _ = struct.unpack("3i", peer)
        route = re.fullmatch(rf"/workspaces/({WORKSPACE})/provider-settings", self.path)
        if uid != os.getuid() or route is None:
            self.send_json(404, {"message": "Маршрут подключения недоступен."})
            return
        if not self.server.slots.acquire(blocking=False):
            self.send_json(503, {"message": "Сейчас сервер проверяет другие подключения. Повторите позже."})
            return
        try:
            data = None
            if self.command == "POST":
                lengths = self.headers.get_all("Content-Length", [])
                if (len(lengths) != 1 or not lengths[0].isdigit() or not 0 < int(lengths[0]) <= MAX_BODY
                        or self.headers.get_content_type() != "application/json"
                        or self.headers.get("Transfer-Encoding")):
                    raise ProviderError("Ожидается небольшой JSON-запрос подключения.", status=400)
                data = json.loads(self.rfile.read(int(lengths[0])))
            result = self.server.app.request(route[1], self.command, data)
        except ProviderError as error:
            self.send_json(error.status, {"state": "UNAVAILABLE", "message": str(error)})
        except (ValueError, OSError, KeyError, TypeError, AttributeError, sqlite3.Error):
            self.send_json(503, {"state": "UNAVAILABLE", "message": "Проверка подключения временно недоступна."})
        else:
            self.send_json(200, result)
        finally:
            self.server.slots.release()

    do_GET = do_POST = invoke

    def send_json(self, status, doc):
        body = json_bytes(doc)
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        try:
            self.wfile.write(body)
        except (BrokenPipeError, ConnectionResetError):
            pass


class Service(ThreadingMixIn, UnixStreamServer):
    daemon_threads = True


def main():
    os.umask(0o077)
    parser = ArgumentParser(description=__doc__)
    for flag in ("socket", "registry", "config-root", "runtime-root", "ready-root"):
        parser.add_argument("--" + flag, type=Path, required=True)
    parser.add_argument("--public-origin", default="https://hitpipe.dkar-dev.ru")
    args = parser.parse_args()
    if args.public_origin != "https://hitpipe.dkar-dev.ru":
        raise SystemExit("Ожидается канонический HTTPS-адрес HitPipe.")
    for folder in (args.socket.parent, args.config_root, args.runtime_root, args.ready_root):
        folder.mkdir(mode=0o700, exist_ok=True)
        private_dir(folder)
    if args.socket.is_socket() and args.socket.lstat().st_uid == os.getuid():
        with socket.socket(socket.AF_UNIX) as client:
            if client.connect_ex(str(args.socket)) == 0:
                raise SystemExit("Служба подключения уже запущена.")
        args.socket.unlink()
    with Service(str(args.socket), Handler) as server:
        os.chmod(args.socket, 0o600)
        server.slots = threading.BoundedSemaphore(4)
        server.app = WorkspaceConnections(args.registry, args.config_root, args.runtime_root, args.ready_root,
                                         args.public_origin)
        server.serve_forever()


if __name__ == "__main__":
    main()
