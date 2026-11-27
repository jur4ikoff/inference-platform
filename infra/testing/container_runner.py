"""Run tests and reporting inside the Compose test network."""
import argparse
import os
from pathlib import Path
import shutil
import signal
import subprocess
import time
from urllib.request import urlopen

from pipeline_report import event, generate, selected_stages
from prepare_test_database import RESULTS, prepare_database
from e2e.test_model_lifecycle import run_e2e

SCRIPTS = Path(__file__).resolve().parent
READINESS_URL = "http://nginx/ready"


class RunnerInterrupted(KeyboardInterrupt):
    """Propagate container stop signals through unittest."""


def wait_ready():
    for _ in range(60):
        try:
            with urlopen(READINESS_URL, timeout=4) as response:
                print(f"{READINESS_URL}: HTTP {response.status}")
                return 0
        except OSError as error:
            print(f"{READINESS_URL}: {error}")
        time.sleep(1)
    return 1


def run_go_tests(stage):
    env = dict(os.environ, TEST_ALLURE_BY_MODULE="1")
    mode = stage
    if stage == "integration":
        mode = "service-integration"
        for service in ("USER", "MODEL", "INFERENCE"):
            env[f"{service}_TEST_DATABASE_URL"] = env["DATABASE_URL"]
    command = ["bash", str(SCRIPTS / "run-go-tests.sh"), mode, str(RESULTS / stage)]
    return subprocess.run(command, env=env).returncode


def interrupt(signum, _frame):
    raise RunnerInterrupted(128 + signum)


def run_pipeline(*, capture=False):
    signal.signal(signal.SIGINT, interrupt)
    signal.signal(signal.SIGTERM, interrupt)
    shutil.copyfile("/usr/local/share/test-snapshot-revision", RESULTS / "snapshot-revision.txt")
    mode = os.environ.get("TEST_MODE", "full")
    injected_failure = os.environ.get("TEST_FAIL_STAGE", "")

    for stage in selected_stages(mode):
        event(stage, "running")
        try:
            if stage == "integration" and os.environ.get("TEST_PAUSE_STAGE") == stage:
                (RESULTS / "pause-ready").touch()
                time.sleep(300)
            status = run_e2e(capture=capture) if stage == "e2e" else run_go_tests(stage)
            if injected_failure == stage:
                status = 1
            if stage == "integration" and status == 0:
                with (RESULTS / "integration-restoration.log").open("w") as log:
                    prepare_database(verify_only=True, stdout=log)
        except subprocess.CalledProcessError as error:
            status = error.returncode
        except BaseException as error:
            event(stage, "broken", f"runner interrupted or aborted: {error}")
            raise
        if status:
            event(stage, "failed", f"stage returned {status} (injected failure: {injected_failure or 'none'})")
            return status
        event(stage, "passed")
    return 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("pipeline", "setup", "ready", "report"), nargs="?", default="pipeline")
    parser.add_argument("--capture", action="store_true")
    args = parser.parse_args()
    action = args.action
    if action == "setup":
        prepare_database()
    elif action == "report":
        generate()
    elif action == "ready":
        return wait_ready()
    else:
        try:
            return run_pipeline(capture=args.capture)
        except RunnerInterrupted as error:
            return error.args[0]
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
