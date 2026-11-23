#!/usr/bin/env python3
"""Exercise the school owners over private sockets and emit a sanitized proof."""
import argparse
import http.client
import json
from pathlib import Path
import socket
import ssl
import time
import uuid


class UnixHTTP(http.client.HTTPConnection):
    def __init__(self, path, context=None, server_name="localhost"):
        super().__init__(server_name, timeout=10)
        self.context = context
        self.path = str(path)

    def connect(self):
        transport = socket.socket(socket.AF_UNIX)
        transport.settimeout(self.timeout)
        transport.connect(self.path)
        self.sock = (self.context.wrap_socket(transport, server_hostname=self.host)
                     if self.context else transport)


def exchange(connection, method, path, data=None, headers=None, expected=200):
    try:
        connection.request(method, path, json.dumps(data) if data is not None else None, headers or {})
        response = connection.getresponse()
        raw = response.read(100000)
        value = json.loads(raw) if raw else {}
        if response.status != expected:
            raise RuntimeError(f"{method} {path} returned {response.status}: {value.get('code', value.get('error', 'invalid response'))}")
        return value
    finally:
        connection.close()


def activation(share, ready):
    context = ssl.create_default_context(cafile=share / ready["caPath"])
    context.load_cert_chain(share / ready["certPath"], share / ready["keyPath"])

    def act(method, path, data=None, expect=200):
        request_id = str(uuid.uuid4())
        if data is not None:
            meta = data.get("command", data)
            meta["request_id"] = request_id
        return exchange(UnixHTTP(share / ready["processingSocket"], context,
                                 ready["processingServerName"]), method, path, data,
                        {"X-Request-ID": request_id, "X-HitPipe-Session": ready["sessionToken"],
                         "X-CSRF-Token": ready["csrfToken"], "Content-Type": "application/json"},
                        expected=expect)

    workspace = f"/api/v1/workspaces/{ready['workspaceId']}"
    operations = act("GET", workspace + "/operations")
    if operations["items"]:
        raise RuntimeError("fresh smoke needs an unused school workspace")
    draft_id = str(uuid.uuid4())
    config = {
        "draft_id": draft_id, "revision": 1, "name": "School verified example",
        "source_connection_id": ready["sourceConnectionId"],
        "destination_connection_id": ready["destinationConnectionId"],
        "mode": "TEST", "price_alias": ready["priceAlias"], "list_alias": ready["listAlias"],
        "assignee_alias": ready["assigneeAlias"], "service_label": ready["serviceLabel"],
        "title_pattern": "SERVICE_ORDER", "required_context_keys": [],
    }
    draft = act("POST", workspace + "/drafts", {"command": {}, "config": config}, expect=201)
    revision = draft["revision"]
    check = act("POST", f"{workspace}/drafts/{draft_id}/readiness", {"expected_revision": revision})
    if not check["passed"]:
        raise RuntimeError("Processing rejected readiness")
    examples = [{"synthetic_session_alias": name, "amount_minor": amount, "paid": paid,
                 "duplicate": repeated, "missing_context_keys": missing}
                for name, amount, paid, repeated, missing in [
                    ("example_paid", 100, True, False, []), ("example_unpaid", 100, False, False, []),
                    ("example_zero", 0, True, False, []), ("example_duplicate", 100, True, True, []),
                    ("example_missing", 100, True, False, ["brief"]),
                ]]
    rehearsal = act("POST", workspace + "/rehearsals",
                    {"command": {"expected_revision": revision}, "draft_id": draft_id,
                     "draft_revision": revision, "readiness_check_id": check["check_id"], "examples": examples})
    if rehearsal["live_requests_made"] != 0:
        raise RuntimeError("unexpected provider call during rehearsal")
    return act("POST", workspace + "/operations/activate", {
        "command": {}, "draft_id": draft_id, "draft_revision": revision,
        "readiness_check_id": check["check_id"], "rehearsal_id": rehearsal["rehearsal_id"],
        "accepted_config_digest": check["activation_preview"]["accepted_config_digest"],
        "acceptance_text_version": "managed-operation-v1", "confirm_activate": True,
    }, expect=201)


def wait_status(provider, condition):
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline:
        value = provider("GET", "/status")
        if condition(value):
            return value
        time.sleep(0.15)
    raise RuntimeError(f"state was not reached: {json.dumps(value, ensure_ascii=False)}")


def validate_single(value):
    if (value.get("caseState") != "CONFIRMED" or value.get("providerCreateCount") != 1
            or value.get("providerReadCount", 0) < 1 or value.get("caseCount") != 1
            or not value.get("taskId")):
        raise RuntimeError("the expected single durable confirmed checkout was not found")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("share", type=Path)
    parser.add_argument("operation", choices=["fresh", "verify"])
    args = parser.parse_args()
    share = args.share.resolve()
    ready = json.loads((share / "ready.json").read_text())

    def provider(method, path, expect=200):
        return exchange(UnixHTTP(share / ready["providerSocket"]), method, path, expected=expect)

    if args.operation == "fresh":
        operation = activation(share, ready)
        if operation["state"] != "ACTIVE":
            raise RuntimeError("Processing did not activate the operation")
        provider("POST", "/events/paid", 202)
        proof = wait_status(provider, lambda v: v.get("caseState") == "CONFIRMED")
        validate_single(proof)
    else:
        proof = provider("GET", "/status")
        validate_single(proof)
    provider("POST", "/events/replay", 202)
    time.sleep(1)
    final = provider("GET", "/status")
    validate_single(final)
    if (final["operationId"], final["caseId"], final["taskId"]) != (
            proof["operationId"], proof["caseId"], proof["taskId"]):
        raise RuntimeError("durable IDs changed after replay")
    print(json.dumps(final, indent=2))


if __name__ == "__main__":
    main()
