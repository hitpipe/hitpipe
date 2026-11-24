#!/usr/bin/env python3
"""Egress for one network-isolated HitPipe workspace with Sandbox providers."""
from argparse import ArgumentParser
from contextlib import closing
import hashlib
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
import threading
import time
from urllib.error import HTTPError
from urllib.parse import parse_qs, urlsplit
from urllib.request import HTTPRedirectHandler, ProxyHandler, Request, build_opener

STRIPE = "https://api.stripe.com"
CLICKUP = "https://api.clickup.com"
VERSION = "2026-09-30.endive"
BODY_LIMIT = 32 << 10
REPLY_LIMIT = 1 << 20
OBJECT = r"[a-zA-Z0-9]+"
CHECKOUT = r"cs_test_[a-zA-Z0-9]+"
CLICKUP_TASK = r"[a-zA-Z0-9_-]{3,90}"
BINDING = {"sourceAccount", "sourcePrice", "destinationWorkspace", "targetList", "assignee", "label"}


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


def send(url, token, *, method="GET", body=None, stripe=False):
    headers = {"Authorization": token, "Accept": "application/json"}
    if stripe:
        headers["Stripe-Version"] = VERSION
    if body is not None:
        headers["Content-Type"] = "application/json"
    request = Request(url, method=method, data=body, headers=headers)
    try:
        response = build_opener(ProxyHandler({}), NoRedirect).open(request, timeout=7)
    except HTTPError as err:
        response = err
    with closing(response):
        result = response.read(REPLY_LIMIT + 1)
        if len(result) > REPLY_LIMIT or 300 <= response.code < 400:
            raise ValueError("invalid provider response")
        return response.code, dict(response.headers.items()), result


def private_json(path):
    fd = os.open(path, os.O_NOFOLLOW | os.O_RDONLY)
    with os.fdopen(fd, "rb") as stream:
        info = os.fstat(stream.fileno())
        if (not stat.S_ISREG(info.st_mode) or stat.S_IMODE(info.st_mode) != 0o600
                or info.st_uid != os.getuid() or info.st_size > 8192):
            raise ValueError("expected owner-only binding or secret file")
        return json.load(stream)


def json_data(body):
    return json.dumps(body, ensure_ascii=False, separators=(",", ":")).encode()


def no_query(path):
    parts = urlsplit(path)
    if parts.netloc or parts.scheme or parts.fragment or parts.path != path:
        return None
    return parts.path


def stripe_allowed(path, price):
    parts = urlsplit(path)
    if parts.netloc or parts.scheme or parts.fragment or "%" in parts.path:
        return False
    query = parse_qs(parts.query, strict_parsing=True, max_num_fields=4)
    if any(len(values) != 1 for values in query.values()):
        return False
    match = parts.path
    if match == "/v1/account" or match == f"/v1/prices/{price}":
        return not query
    if re.fullmatch(rf"/v1/(checkout/sessions/{CHECKOUT}|payment_intents/pi_{OBJECT}|charges/ch_{OBJECT})", match):
        return not query
    if not (re.fullmatch(rf"/v1/checkout/sessions/{CHECKOUT}/line_items", match)
            or match in {"/v1/refunds", "/v1/disputes"}):
        return False
    charge_required = match in {"/v1/refunds", "/v1/disputes"}
    if charge_required and not re.fullmatch(rf"ch_{OBJECT}", query.get("charge", [""])[0]):
        return False
    expected = {"limit", "starting_after"} | ({"charge"} if charge_required else set())
    return (
        set(query) <= expected
        and re.fullmatch(r"[1-9][0-9]{0,2}", query.get("limit", ["20"])[0]) is not None
        and int(query.get("limit", ["20"])[0]) <= 100
        and re.fullmatch(r"[a-zA-Z0-9_]{4,120}", query.get("starting_after", ["item_none"])[0]) is not None
    )


class Application:
    def __init__(self, binding, secrets, journal, transport=send):
        if set(binding) != BINDING or not all(isinstance(x, str) and x for x in binding.values()):
            raise ValueError("provider demo fields differ")
        if not (re.fullmatch(r"acct_[a-zA-Z0-9]{8,64}", binding["sourceAccount"])
                and re.fullmatch(r"price_[a-zA-Z0-9]{8,64}", binding["sourcePrice"])
                and all(re.fullmatch(r"[1-9][0-9]{0,19}", binding[k])
                        for k in ("destinationWorkspace", "targetList", "assignee"))):
            raise ValueError("invalid provider binding identifiers")
        self.binding = binding
        stripe = secrets["stripe_api_key"]
        self.clickup = secrets["clickup_token"]
        if not re.fullmatch(r"(?:rk|sk)_test_[a-zA-Z0-9]{16,220}", stripe) or not re.fullmatch(
                r"pk_[a-zA-Z0-9_\-]{10,240}", self.clickup):
            raise ValueError("a test Stripe key and dedicated ClickUp credential are required")
        self.stripe = f"Bearer {stripe}"
        self.send = transport
        self.journal = journal
        self.verified_until = 0
        self.lock = threading.Lock()
        with closing(self.database()) as db:
            db.execute("""CREATE TABLE IF NOT EXISTS created (
                id TEXT PRIMARY KEY, body_digest TEXT NOT NULL, created_at INTEGER NOT NULL, reads INTEGER NOT NULL DEFAULT 0
            )""")
            db.commit()

    def database(self):
        return sqlite3.connect(self.journal, timeout=2)

    def request_json(self, path, provider):
        if provider == "stripe":
            code, _, content = self.send(STRIPE + path, self.stripe, stripe=True)
        else:
            code, _, content = self.send(CLICKUP + path, self.clickup)
        if code != 200:
            raise ValueError(f"{provider} binding verification returned HTTP {code}")
        return json.loads(content)

    def confirm_binding(self, now):
        with self.lock:
            if now < self.verified_until:
                return
            b = self.binding
            account = self.request_json("/v1/account", "stripe")
            price = self.request_json("/v1/prices/" + b["sourcePrice"], "stripe")
            user = self.request_json("/api/v2/user", "clickup")
            target = self.request_json("/api/v2/list/" + b["targetList"], "clickup")
            spaces = self.request_json(f"/api/v2/team/{b['destinationWorkspace']}/space?archived=false", "clickup")
            if (account.get("id") != b["sourceAccount"] or price.get("id") != b["sourcePrice"]
                    or price.get("livemode") is not False or price.get("active") is not True
                    or price.get("type") != "one_time" or str(user.get("user", {}).get("id")) != b["assignee"]
                    or str(target.get("id")) != b["targetList"] or target.get("archived") is True
                    or str(target.get("space", {}).get("id")) not in {str(space.get("id")) for space in spaces.get("spaces", [])}):
                raise ValueError("external provider binding differs")
            self.verified_until = now + 45

    def clickup_action(self, method, path, body):
        target = self.binding["targetList"]
        assignee = int(self.binding["assignee"])
        if method == "POST" and path == f"/api/v2/list/{target}/task":
            try:
                parsed = json.loads(body)
                if (set(parsed) != {"name", "description", "assignees"} or parsed["assignees"] != [assignee]
                        or not isinstance(parsed["name"], str) or not 0 < len(parsed["name"]) <= 512
                        or not isinstance(parsed["description"], str) or not 0 < len(parsed["description"]) <= 8192):
                    raise ValueError
            except (ValueError, TypeError):
                return 400, {}, b""
            status, headers, raw = self.send(CLICKUP + path, self.clickup, method="POST", body=body)
            if 200 <= status < 300:
                identifier = json.loads(raw).get("id", "")
                if not isinstance(identifier, str) or not re.fullmatch(CLICKUP_TASK, identifier):
                    return 502, {}, b""
                with closing(self.database()) as db:
                    db.execute("INSERT INTO created (id,body_digest,created_at) VALUES (?,?,?)",
                               (identifier, hashlib.sha256(body).hexdigest(), int(time.time())))
                    db.commit()
            return status, headers, raw
        match = re.fullmatch(rf"/api/v2/task/({CLICKUP_TASK})", path) if method == "GET" else None
        if not match:
            return 404, {}, b""
        task = match.group(1)
        with closing(self.database()) as db:
            if not db.execute("SELECT 1 FROM created WHERE id=?", (task,)).fetchone():
                return 404, {}, b""
        status, headers, raw = self.send(CLICKUP + path, self.clickup)
        if status == 200:
            with closing(self.database()) as db:
                db.execute("UPDATE created SET reads=reads+1 WHERE id=?", (task,))
                db.commit()
        return status, headers, raw

    def handle(self, method, path, body):
        self.confirm_binding(time.monotonic())
        if method == "GET" and path == "/binding":
            return 200, {}, json_data(self.binding)
        if method == "GET" and path.startswith("/stripe/") and stripe_allowed(path[7:], self.binding["sourcePrice"]):
            return self.send(STRIPE + path[7:], self.stripe, stripe=True)
        if path.startswith("/clickup/") and no_query(path):
            return self.clickup_action(method, path[8:], body)
        return 404, {}, b""


class Handler(BaseHTTPRequestHandler):
    def setup(self):
        self.request.settimeout(12)
        super().setup()

    def log_message(self, *_):
        pass

    def serve_request(self):
        try:
            credentials = self.request.getsockopt(socket.SOL_SOCKET, socket.SO_PEERCRED, struct.calcsize("3i"))
            _, uid, _ = struct.unpack("3i", credentials)
            size = int(self.headers.get("Content-Length", "0"))
            if (uid != os.getuid() or self.headers.get("Transfer-Encoding") is not None or not 0 <= size <= BODY_LIMIT
                    or len(self.headers.get_all("Content-Length", [])) > 1 or self.command == "GET" and size):
                self.reply(400, {}, b"")
                return
            body = self.rfile.read(size)
            if len(body) != size or len(self.path) > 2048:
                self.reply(400, {}, b"")
                return
            status, headers, result = self.server.app.handle(self.command, self.path, body)
        except (OSError, ValueError, KeyError, TypeError, AttributeError, sqlite3.Error):
            # An ambiguous create must stay ambiguous to Delivery. Never retry here.
            status, headers, result = 502, {}, b""
        self.reply(status, headers, result)

    do_POST = do_GET = serve_request

    def reply(self, status, headers, result):
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        for name in ("Stripe-Version", "Retry-After"):
            value = next((v for k, v in headers.items() if k.lower() == name.lower()), None)
            if isinstance(value, str) and re.fullmatch(r"[a-zA-Z0-9. -]{1,60}", value):
                self.send_header(name, value)
        self.send_header("Content-Length", str(len(result)))
        self.end_headers()
        try:
            self.wfile.write(result)
        except (BrokenPipeError, ConnectionResetError):
            pass


class Server(ThreadingMixIn, UnixStreamServer):
    daemon_threads = True


def main():
    os.umask(0o077)
    parser = ArgumentParser(description=__doc__)
    parser.add_argument("--binding", type=Path, required=True)
    parser.add_argument("--credentials", type=Path, required=True)
    parser.add_argument("--journal", type=Path, required=True)
    parser.add_argument("--socket", type=Path, required=True)
    args = parser.parse_args()
    if args.socket.name != "provider-egress.sock" or args.socket.parent.stat().st_mode & 0o777 != 0o700:
        raise SystemExit("Expected owner-only runtime bridge")
    app = Application(private_json(args.binding), private_json(args.credentials), args.journal)
    app.confirm_binding(time.monotonic())
    with Server(str(args.socket), Handler) as server:
        server.app = app
        os.chmod(args.socket, 0o600)
        try:
            server.serve_forever()
        finally:
            args.socket.unlink(missing_ok=True)


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, KeyError, sqlite3.Error) as error:
        raise SystemExit(f"Provider egress startup failed ({type(error).__name__})") from None
