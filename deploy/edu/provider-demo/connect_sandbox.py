#!/usr/bin/env python3
"""Connect the designated Sandbox providers through the private HitPipe API."""

from argparse import ArgumentParser
from contextlib import closing
from getpass import getpass
from http.client import HTTPConnection, HTTPException
import json
import os
from pathlib import Path
import re
import socket
import sqlite3
import stat
import sys

from provider_settings_service import WORKSPACE

ROOT = Path.home() / ".hp-school"
DEMO_ACCOUNT = "acct_1UfGTDE1OQW8MuKn"
DEMO_SELECTION = {
    "sourcePrice": "price_1UfQv0E1OQW8MuKntAOpA58g",
    "destinationWorkspace": "90121565569",
    "targetList": "1500770000002514",
    "assignee": "290457043",
}
KEY_FILE = Path.home() / ".config/hitpipe-provider-demo/credentials.json"


def private(path, kind):
    info = path.lstat()
    if not kind(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise ValueError("Ожидается закрытый файл службы, принадлежащий текущему пользователю.")


def registered_workspace(registry, requested):
    private(registry, stat.S_ISREG)
    with closing(sqlite3.connect(registry.as_uri() + "?mode=ro", uri=True, timeout=2)) as db:
        if requested is None:
            candidates = db.execute("SELECT workspace_id FROM school_tenants LIMIT 2").fetchall()
        else:
            candidates = db.execute("SELECT workspace_id FROM school_tenants WHERE workspace_id=? LIMIT 2",
                                    (requested,)).fetchall()
    if len(candidates) != 1 or not re.fullmatch(WORKSPACE, candidates[0][0]):
        raise ValueError("Нужна одна зарегистрированная область. При нескольких укажите её UUID через --workspace.")
    return candidates[0][0]


class SettingsConnection(HTTPConnection):
    def __init__(self, path):
        super().__init__("hitpipe.local", timeout=48)
        self.path = path

    def connect(self):
        connection = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        try:
            connection.settimeout(self.timeout)
            connection.connect(os.fspath(self.path))
        except OSError:
            connection.close()
            raise
        self.sock = connection


def request(path, workspace, data=None):
    private(path, stat.S_ISSOCK)
    route = f"/workspaces/{workspace}/provider-settings"
    headers = {"Content-Type": "application/json"} if data is not None else {}
    body = json.dumps(data, separators=(",", ":")).encode() if data is not None else None
    with closing(SettingsConnection(path)) as connection:
        connection.request("POST" if data is not None else "GET", route, body=body, headers=headers)
        response = connection.getresponse()
        raw = response.read(1024 * 1024 + 1)
        if response.status != 200:
            raise ValueError(f"Служба настроек вернула HTTP {response.status}.")
    if len(raw) > 1024 * 1024:
        raise ValueError("Ответ службы настроек превысил ограничение.")
    result = json.loads(raw)
    if not isinstance(result, dict):
        raise ValueError("Неизвестный ответ службы настроек.")
    return result


def credentials(path, prompt):
    if prompt:
        if not sys.stdin.isatty() or not sys.stderr.isatty():
            raise ValueError("Скрытый ввод доступен в интерактивном терминале VPS.")
        stripe = getpass("Ключ Stripe Sandbox: ").strip()
        clickup = getpass("Токен ClickUp: ").strip()
    else:
        private(path, stat.S_ISREG)
        with path.open(encoding="utf-8") as stream:
            raw = stream.read(16_385)
        if len(raw) > 16_384:
            raise ValueError("Файл подключения превысил ограничение.")
        keys = json.loads(raw)
        stripe, clickup = keys.get("stripe_api_key"), keys.get("clickup_token")
    if not isinstance(stripe, str) or not re.fullmatch(r"(?:sk|rk)_test_[a-zA-Z0-9]{16,220}", stripe):
        raise ValueError("Требуется ключ выбранного Stripe Sandbox.")
    if not isinstance(clickup, str) or not re.fullmatch(r"pk_[a-zA-Z0-9_\-]{10,240}", clickup):
        raise ValueError("Требуется токен ClickUp для тестового списка.")
    return {"stripeApiKey": stripe, "clickupToken": clickup}


def matching_binding(binding):
    return (isinstance(binding, dict) and binding.get("sourceAccount") == DEMO_ACCOUNT
            and all(binding.get(key) == value for key, value in DEMO_SELECTION.items()))


def validate_catalog(catalog):
    stripe = catalog["stripe"]
    if (catalog["state"] != "VERIFIED" or stripe["account"] != DEMO_ACCOUNT
            or not any(price["id"] == DEMO_SELECTION["sourcePrice"] for price in stripe["prices"])):
        raise ValueError("Каталог Stripe не подтвердил выделенную песочницу и цену.")
    for area in catalog["clickup"]["workspaces"]:
        if area["id"] != DEMO_SELECTION["destinationWorkspace"]:
            continue
        for target in area["lists"]:
            if target["id"] == DEMO_SELECTION["targetList"] and any(
                    person["id"] == DEMO_SELECTION["assignee"] for person in target["assignees"]):
                return
    raise ValueError("Каталог ClickUp не подтвердил выделенный список и исполнителя.")


def run(args):
    workspace = registered_workspace(args.registry, args.workspace)
    status = request(args.socket, workspace)
    binding = status.get("binding")
    if args.operation == "connect":
        if binding and not matching_binding(binding):
            raise ValueError("Область уже связана с другой конфигурацией.")
        if status["state"] not in {"CONNECTING", "CONNECTED"}:
            keys = credentials(args.credentials, args.prompt)
            catalog = request(args.socket, workspace, {"action": "inspect", **keys})
            validate_catalog(catalog)
            status = request(args.socket, workspace, {"action": "connect", **keys, **DEMO_SELECTION})
            binding = status.get("binding")
            if not matching_binding(binding):
                raise ValueError("Служба вернула другую связку. Проверьте состояние перед повтором.")
    if status["state"] not in {"UNCONNECTED", "UNAVAILABLE", "CONNECTING", "CONNECTED"}:
        raise ValueError("Неизвестное состояние подключения.")
    return {"workspace": workspace, "state": status["state"], "demo_binding": matching_binding(binding)}


def main():
    parser = ArgumentParser(description=__doc__)
    parser.add_argument("operation", choices=("status", "connect"))
    parser.add_argument("--workspace", help="UUID зарегистрированной области; по умолчанию выбирается единственная")
    parser.add_argument("--socket", type=Path, default=ROOT / "provider-api/settings.sock")
    parser.add_argument("--registry", type=Path, default=ROOT / "public/tenants.sqlite3")
    parser.add_argument("--credentials", type=Path, default=KEY_FILE)
    parser.add_argument("--prompt", action="store_true", help="попросить оба секрета через скрытый ввод")
    args = parser.parse_args()
    try:
        print(json.dumps(run(args), ensure_ascii=False))
        return 0
    except ValueError as error:
        print(str(error) if type(error) is ValueError else "Неверные данные настройки.", file=sys.stderr)
    except (AttributeError, KeyError, TypeError, OSError, HTTPException, sqlite3.Error):
        print("Служба или защищённый файл недоступны. Проверьте настройки перед повтором.", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
