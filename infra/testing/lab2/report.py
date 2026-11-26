#!/usr/bin/env python3
"""Small bridge from native Go Allure results and shell stages to Allure 2."""
import fcntl
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import time
import uuid

ROOT = Path(os.environ.get("LAB2_REPORT_ROOT", "/results"))
HISTORY = Path(os.environ.get("LAB2_REPORT_HISTORY", "/history"))


def read_object(path):
    data = json.loads(path.read_text())
    if not isinstance(data, dict):
        raise ValueError("expected a JSON object")
    return data


def event(name, status, message=""):
    path = ROOT / "stages" / f"{name}.json"
    path.parent.mkdir(parents=True, exist_ok=True)
    now = int(time.time() * 1000)
    try:
        previous = read_object(path) if path.exists() else {}
    except (OSError, ValueError):
        previous = {}
    temporary = path.with_name(f".{path.name}.{uuid.uuid4()}.tmp")
    try:
        temporary.write_text(json.dumps(dict(name=name, status=status, message=message,
                                            start=previous.get("start", now), stop=now)))
        temporary.replace(path)
    finally:
        temporary.unlink(missing_ok=True)


def result(out, name, status, message="", start=None, stop=None, full_name=None):
    identifier = str(uuid.uuid4())
    full = full_name or "lab2/" + name
    now = int(time.time() * 1000)
    data = dict(uuid=identifier, name=name, fullName=full,
                historyId=hashlib.md5(full.encode()).hexdigest(),
                status=status, stage="finished", start=start or now, stop=stop or now,
                statusDetails={"message": message},
                labels=[{"name": "suite", "value": "Lab 2"}])
    (out / f"{identifier}-result.json").write_text(json.dumps(data))


def read_events(path):
    events = []
    if path.exists():
        for line in path.read_text().splitlines():
            try:
                events.append(json.loads(line))
            except json.JSONDecodeError:
                pass  # An interrupted writer can leave a partial final line.
    return events


def generate():
    (ROOT / "report-complete").unlink(missing_ok=True)
    out = ROOT / "allure-results"
    # A run owns this directory; previous runs contribute history only.
    if out.exists():
        shutil.rmtree(out)
    out.mkdir()
    stages = {}
    for name in ("setup", "unit", "integration", "e2e", "cleanup"):
        path = ROOT / "stages" / f"{name}.json"
        try:
            state = read_object(path) if path.exists() else dict(
                name=name, status="skipped", message="not run because an earlier stage failed or was interrupted")
            if state.get("status") not in ("running", "passed", "failed", "broken", "skipped"):
                raise ValueError("missing or invalid stage status")
        except (OSError, ValueError) as error:
            state = dict(name=name, status="broken", message=f"malformed stage result {path.name}: {error}")
        if state["status"] == "running":
            state.update(status="broken", message="interrupted before stage completion")
        stages[name] = state
    integration_seen = set()
    for stage in ("unit", "integration"):
        for module in ("user-service", "model-service", "inference-service"):
            source = ROOT / stage / module / "allure-results"
            for path in source.glob("*"):
                if not path.is_file():
                    continue
                try:
                    data = read_object(path) if path.name.endswith(("-result.json", "-container.json")) else None
                    if path.name.endswith("-result.json") and (
                        not isinstance(data.get("name"), str)
                        or data.get("status") not in ("passed", "failed", "broken", "skipped", "unknown")
                        or not isinstance(data.get("fullName", data["name"]), str)
                        or not isinstance(data.get("labels", []), list)
                    ):
                        raise ValueError("missing or invalid native test fields")
                except (OSError, ValueError) as error:
                    message = f"malformed native result {module}/{stage}/{path.name}: {error}"
                    if stage == "integration" and path.name.endswith("-result.json"):
                        result(out, "TestServiceIntegration", "broken", message,
                               full_name=module + "/internal/service/integration_test.go/TestServiceIntegration")
                        integration_seen.add(module)
                    else:
                        result(out, f"{module}/{stage}/{path.name}", "broken", message)
                    if stages[stage]["status"] == "passed":
                        stages[stage].update(status="broken", message=message)
                    continue
                if path.name.endswith("-result.json"):
                    # Native Go names are relative paths, identical across modules.
                    data["fullName"] = module + "/" + data.get("fullName", data["name"])
                    data["historyId"] = hashlib.md5(data["fullName"].encode()).hexdigest()
                    data.setdefault("labels", []).append({"name": "parentSuite", "value": module})
                    if stage == "integration" and data["name"] == "TestServiceIntegration":
                        integration_seen.add(module)
                    (out / path.name).write_text(json.dumps(data))
                else:
                    shutil.copy2(path, out / path.name)
    for module in ("user-service", "model-service", "inference-service"):
        if module not in integration_seen:
            state = stages["integration"]
            log = ROOT / "integration" / f"{module}.json"
            events = read_events(log)
            started = any(e.get("Test") == "TestServiceIntegration" and e.get("Action") == "run" for e in events)
            status = "broken" if started else "skipped"
            if not started and state["status"] == "passed":
                status = "broken"
            if status == "broken" and state["status"] == "passed":
                state.update(status="broken", message=f"missing native integration result for {module}")
            result(out, "TestServiceIntegration", status, state.get("message", "no completed Go test result"),
                   full_name=module + "/internal/service/integration_test.go/TestServiceIntegration")
    for name in ("setup", "unit", "integration", "e2e", "cleanup"):
        state = stages[name]
        result(out, name if name != "e2e" else "E2E model lifecycle (curl + tcpdump)",
               state["status"], state.get("message", ""), state.get("start"), state.get("stop"))
    (ROOT / "summary.json").write_text(json.dumps(stages, indent=2))
    HISTORY.mkdir(parents=True, exist_ok=True)
    # Concurrent runs merge no raw results and serialize history replacement.
    with (HISTORY / ".lock").open("w") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        if (HISTORY / "history").is_dir():
            shutil.copytree(HISTORY / "history", out / "history")
        subprocess.run(["allure", "generate", str(out), "--clean", "-o", str(ROOT / "allure-report")], check=True)
        if (HISTORY / "history").exists():
            shutil.rmtree(HISTORY / "history")
        shutil.copytree(ROOT / "allure-report" / "history", HISTORY / "history")
    (ROOT / "report-complete").write_text("complete\n")
    if any(state["status"] == "broken" for state in stages.values()):
        raise SystemExit("report contains broken stages")


if __name__ == "__main__":
    if sys.argv[1] == "event":
        event(*sys.argv[2:])
    elif sys.argv[1] == "generate":
        generate()
    else:
        raise SystemExit("expected event or generate")
