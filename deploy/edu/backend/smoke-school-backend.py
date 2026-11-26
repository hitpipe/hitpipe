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


def business_client(share, ready):
    context = ssl.create_default_context(cafile=share / ready["caPath"])
    context.load_cert_chain(share / ready["certPath"], share / ready["keyPath"])

    def act(method, path, data=None, expect=200, request_id=None):
        request_id = request_id or str(uuid.uuid4())
        if data is not None:
            meta = data.get("command", data)
            meta["request_id"] = request_id
        return exchange(UnixHTTP(share / ready["processingSocket"], context,
                                 ready["processingServerName"]), method, path, data,
                        {"X-Request-ID": request_id, "X-HitPipe-Session": ready["sessionToken"],
                         "X-CSRF-Token": ready["csrfToken"], "Content-Type": "application/json"},
                        expected=expect)

    return act


def activation(share, ready):
    act = business_client(share, ready)
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


def recheck(share, ready, provider, before):
    act = business_client(share, ready)
    case_path = f"/api/v1/workspaces/{ready['workspaceId']}/cases/{before['caseId']}"
    previous = act("GET", case_path)
    if ("RECHECK" not in previous["available_actions"] or not previous["case"]["target_url"]
            or not previous["last_observation_at"]):
        raise RuntimeError("the confirmed case did not expose the owner recheck action")
    request_id = str(uuid.uuid4())
    command = {"action": "RECHECK", "reason": "Verify the existing ClickUp task",
               "command": {"expected_revision": previous["case"]["revision"]}}
    receipt = act("POST", case_path + "/recovery", command, 202, request_id=request_id)
    if receipt["case"]["case_id"] != before["caseId"]:
        raise RuntimeError("the recovery receipt changed the case identity")
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline:
        observed = act("GET", case_path)
        if (observed["last_observation_at"] != previous["last_observation_at"]
                and observed["case"]["revision"] > previous["case"]["revision"]):
            break
        time.sleep(0.15)
    else:
        raise RuntimeError("Processing did not apply a fresh Delivery observation")
    if (not observed["last_observation_at"] or not observed["observation_complete"]
            or observed["case"]["state"] != "CONFIRMED"
            or observed["case"]["target_url"] != previous["case"]["target_url"]):
        raise RuntimeError("the recheck changed the known outcome or target")
    act("POST", case_path + "/recovery", command, 202, request_id=request_id)
    history = act("GET", case_path + "/history")["items"]
    if sum(row["reason_code"] == "RECHECK_ACCEPTED" for row in history) != 1:
        raise RuntimeError("repeating the accepted request changed its history receipt count")
    after = provider("GET", "/status")
    validate_single(after)
    if (after["caseId"], after["taskId"], after["providerReadCount"]) != (
            before["caseId"], before["taskId"], before["providerReadCount"] + 1):
        raise RuntimeError("the recheck read a different task or did not read exactly once")
    return {"caseId": after["caseId"], "clickUpTaskId": after["taskId"], "recoveryStatus": 202,
            "providerCreates": after["providerCreateCount"], "readsBefore": before["providerReadCount"],
            "readsAfter": after["providerReadCount"], "observationBefore": previous["last_observation_at"],
            "observationAfter": observed["last_observation_at"], "recheckHistoryReceipts": 1}


def lifecycle(share, ready, provider, enqueue=False):
    act = business_client(share, ready)
    workspace = f"/api/v1/workspaces/{ready['workspaceId']}"

    def operation():
        operations = act("GET", workspace + "/operations")["items"]
        if len(operations) != 1:
            raise RuntimeError("lifecycle smoke needs one operation")
        return operations[0]

    active = operation()
    if active["state"] != "ACTIVE":
        raise RuntimeError("lifecycle smoke needs an active operation")
    path = f"{workspace}/operations/{active['operation_id']}"
    command = path + "/lifecycle"
    draining = act("POST", command, {"command": {"expected_revision": active["revision"]}, "action": "PAUSE"}, 202)
    if draining["state"] != "DRAINING":
        raise RuntimeError("pause request did not start drain")
    wait_status(provider, lambda v: v["operationState"] == "PAUSED")
    paused = operation()
    for _ in range(15):
        try:
            act("POST", command, {"command": {"expected_revision": paused["revision"]},
                                  "action": "PREPARE_RESUME"}, 202)
            break
        except RuntimeError as error:
            if "returned 503:" not in str(error):
                raise
            time.sleep(0.3)
    else:
        raise RuntimeError("the paused operation did not get an empty queue proof")
    snapshot = act("GET", path + "/resume-snapshot")
    if snapshot["signal_count"] != 0 or snapshot["serialized_bytes"] != 0 or snapshot["gaps"]:
        raise RuntimeError("the resumed queue was not durably observed empty")
    accept = {
        "command": {"expected_revision": snapshot["revision"]},
        "action": "ACCEPT_RESUME", "resume_snapshot_id": snapshot["snapshot_id"],
        "held_disposition": "PROCESS_ELIGIBLE",
    }
    if enqueue:
        provider("POST", "/events/paid", 202)
        act("POST", command, accept, 503)
        for _ in range(3):
            act("POST", command, {"command": {"expected_revision": snapshot["revision"]},
                                  "action": "PREPARE_RESUME"}, 503)
            time.sleep(0.4)
        stopped = operation()
        if stopped["state"] != "PAUSED" or stopped["revision"] != snapshot["revision"]:
            raise RuntimeError("an event entered after the preview but the operation still changed")
        return {"operationId": stopped["operation_id"], "state": stopped["state"],
                "invalidatedSnapshotId": snapshot["snapshot_id"], "newEventAcceptedByIntake": True,
                "acceptOldSnapshotStatus": 503, "prepareResumeStatus": 503}
    resumed = act("POST", command, accept, 202)
    if resumed["state"] != "ACTIVE" or resumed["first_activation_at"] != active["first_activation_at"]:
        raise RuntimeError("resuming the operation changed the original activation")
    return {"operationId": resumed["operation_id"], "drainReached": draining["state"],
            "pauseReached": paused["state"], "resumeState": resumed["state"],
            "resumeSnapshotId": snapshot["snapshot_id"],
            "signalCount": snapshot["signal_count"], "serializedBytes": snapshot["serialized_bytes"],
            "firstActivationAt": resumed["first_activation_at"]}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("share", type=Path)
    parser.add_argument("operation", choices=["fresh", "fresh-lifecycle", "fresh-recheck", "verify", "lifecycle",
                                               "lifecycle-held", "activate-only", "late-payment"])
    args = parser.parse_args()
    share = args.share.resolve()
    ready = json.loads((share / "ready.json").read_text())

    def provider(method, path, expect=200):
        return exchange(UnixHTTP(share / ready["providerSocket"]), method, path, expected=expect)

    if args.operation.startswith("lifecycle"):
        print(json.dumps(lifecycle(share, ready, provider, enqueue=args.operation == "lifecycle-held"), indent=2))
        return
    if args.operation == "activate-only":
        print(json.dumps(activation(share, ready), indent=2))
        return
    if args.operation == "late-payment":
        previous = provider("GET", "/status")
        expected_cases = previous["caseCount"] + 1
        expected_posts = previous["providerCreateCount"] + 1
        provider("POST", "/events/unpaid", 202)
        waiting = wait_status(provider, lambda v: v["caseCount"] == expected_cases and v["caseState"] == "WAITING_DATA")
        if (waiting["providerCreateCount"] != previous["providerCreateCount"] or waiting["taskId"]):
            raise RuntimeError("ClickUp received an unpaid case")
        provider("POST", "/events/async-paid", 202)
        paid = wait_status(provider, lambda v: v["caseState"] == "CONFIRMED" and v["caseId"] == waiting["caseId"])
        if (paid["providerCreateCount"] != expected_posts or paid["caseCount"] != expected_cases or not paid["taskId"]):
            raise RuntimeError("late payment did not create exactly one task for the same checkout")
        print(json.dumps({"caseId": paid["caseId"], "before": waiting["caseState"],
                          "after": paid["caseState"], "clickUpTaskId": paid["taskId"]}, indent=2))
        return
    if args.operation in {"fresh", "fresh-lifecycle", "fresh-recheck"}:
        operation = activation(share, ready)
        if operation["state"] != "ACTIVE":
            raise RuntimeError("Processing did not activate the operation")
        provider("POST", "/events/paid", 202)
        proof = wait_status(provider, lambda v: v.get("caseState") == "CONFIRMED")
        validate_single(proof)
    else:
        proof = provider("GET", "/status")
        validate_single(proof)
    if args.operation == "fresh-recheck":
        print(json.dumps(recheck(share, ready, provider, proof), indent=2))
        return
    pause_resume = None
    if args.operation == "fresh-lifecycle":
        pause_resume = lifecycle(share, ready, provider)
        if pause_resume["operationId"] != proof["operationId"]:
            raise RuntimeError("pause and resume did not apply to the operation of the original payment")
    provider("POST", "/events/replay", 202)
    time.sleep(1)
    final = provider("GET", "/status")
    validate_single(final)
    if (final["operationId"], final["caseId"], final["taskId"]) != (
            proof["operationId"], proof["caseId"], proof["taskId"]):
        raise RuntimeError("durable IDs changed after replay")
    if args.operation == "fresh-lifecycle":
        if final["operationState"] != "ACTIVE":
            raise RuntimeError("the operation was no longer active after replay")
        print(json.dumps({"payment": proof, "pauseResume": pause_resume, "afterReplay": final}, indent=2))
        return
    print(json.dumps(final, indent=2))


if __name__ == "__main__":
    main()
