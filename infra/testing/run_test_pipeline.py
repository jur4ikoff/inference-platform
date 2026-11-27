"""Build and run the isolated Compose test stand and collect its results."""
import argparse
from datetime import datetime
import os
from pathlib import Path
import re
import secrets
import signal
import subprocess
import sys

from pipeline_report import TEST_STAGES, event, read_object

ROOT = Path(__file__).resolve().parents[2]
TESTING = Path(__file__).resolve().parent
REPORTS = ROOT / "test-results" / "pipeline"
CACHED_IMAGE = "inference-test-runner:local"
SERVICES = ("nginx", "user-service", "model-service", "inference-service")
RUN_MODES = ("full", *TEST_STAGES)


class TestPipeline:
    def __init__(self, mode, *, capture=False):
        self.finalizing = mode == "finalize"
        self.capture = capture
        project = os.environ.get("TEST_PROJECT")
        if self.finalizing and not project:
            raise ValueError("finalize requires the original TEST_PROJECT")
        if not project:
            project = f"inference-tests-{datetime.now():%Y%m%d%H%M%S}-{os.getpid()}-{secrets.token_hex(3)}"
        if not re.fullmatch(r"inference-tests-[a-z0-9-]+", project):
            raise ValueError("TEST_PROJECT must start with inference-tests- and contain lowercase letters, digits or hyphens")

        self.project = project
        self.network = f"{project}_default"
        self.runner = f"{project}-runner"
        self.results = Path(os.environ.get("TEST_RESULTS_DIR") or REPORTS / project).resolve()
        self.mode_file = self.results / "mode.txt"
        if self.finalizing:
            mode = self.mode_file.read_text().strip() if self.mode_file.is_file() else "full"
            if mode not in RUN_MODES:
                raise ValueError(f"invalid test mode in {self.mode_file}: {mode!r}")
        self.mode = mode
        default_history = REPORTS / "history"
        if mode != "full":
            default_history /= mode
        self.history = Path(os.environ.get("TEST_HISTORY_DIR") or default_history).resolve()
        self.model_image = f"{project}-model:local"
        self.default_runner_image = f"{project}-runner:local"
        self.runner_image = os.environ.get("TEST_RUNNER_IMAGE") or self.default_runner_image
        self.env = dict(
            os.environ,
            TEST_MODE=mode, TEST_PROJECT=project, TEST_RESULTS_DIR=str(self.results),
            TEST_HISTORY_DIR=str(self.history), TEST_MODEL_IMAGE=self.model_image,
            TEST_RUNNER_IMAGE=self.runner_image,
            TEST_SOURCE_REVISION=self.git("rev-parse", "HEAD").strip(),
            POSTGRES_USER="test", POSTGRES_PASSWORD="test", POSTGRES_DB="inference_test",
            MODEL_SERVICE_RW_PASSWORD="test", JWT_KEY_ID="test-key", JWT_ISSUER="user-service",
            JWT_AUDIENCE="inference-platform", JWT_PRIVATE_KEY_FILE=str(self.results / "keys" / "jwt.pem"),
            GRAFANA_ADMIN_PASSWORD="test-grafana-password",
        )
        self.compose_command = (
            "compose", "--env-file", "/dev/null", "--project-directory", str(ROOT),
            "-p", project, "-f", str(ROOT / "infra" / "docker-compose.yaml"),
            "-f", str(TESTING / "compose.test.yaml"),
        )
        if mode in ("full", "e2e"):
            self.compose_command += ("-f", str(TESTING / "e2e" / "compose.yaml"))

    @staticmethod
    def git(*args):
        return subprocess.check_output(["git", "-C", str(ROOT), *args], text=True)

    def docker(self, *args, **options):
        options.setdefault("check", True)
        return subprocess.run(["docker", *args], cwd=ROOT, env=self.env, text=True, **options)

    def compose(self, *args, **options):
        return self.docker(*self.compose_command, *args, **options)

    def exists(self, resource, name):
        return self.docker(
            resource, "inspect", name, check=False,
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        ).returncode == 0

    def initialize(self):
        self.results.mkdir(parents=True, exist_ok=True)
        self.history.mkdir(parents=True, exist_ok=True)
        if not self.finalizing:
            if any(self.results.iterdir()):
                raise ValueError("results directory must be empty")
            if self.exists("network", self.network):
                raise ValueError("project already exists; choose a fresh TEST_PROJECT")
            self.mode_file.write_text(self.mode + "\n")
        (self.results / "project.txt").write_text(self.project + "\n")
        if not (self.results / "commit.txt").is_file():
            (self.results / "commit.txt").write_text(self.env["TEST_SOURCE_REVISION"] + "\n")
        if not (self.results / "worktree.txt").is_file():
            (self.results / "worktree.txt").write_text(self.git("status", "--short", "--untracked-files=all"))
        print(f"Test project: {self.project}. Results: {self.results}", flush=True)

    def stage(self, name, status, message=""):
        event(name, status, message, root=self.results)

    def runner_task(self, action, log):
        with (self.results / log).open("w") as output:
            self.compose("run", "--rm", "--no-deps", "--name", f"{self.project}-{action}",
                         "-T", "runner", action,
                         stdout=output, stderr=subprocess.STDOUT)

    def run(self):
        if self.finalizing:
            if not self.exists("image", self.runner_image):
                self.docker("image", "tag", CACHED_IMAGE, self.runner_image)
            return 0
        if os.environ.get("TEST_SKIP_BUILD") == "1":
            self.docker("image", "tag", CACHED_IMAGE, self.runner_image)
        else:
            self.compose("build", "runner")
            self.docker("image", "tag", self.runner_image, CACHED_IMAGE)
        if self.mode in ("full", "e2e"):
            self.compose("build", "--with-dependencies", "nginx", "e2e-model")
        elif self.mode == "integration":
            self.compose("build", "postgres")

        if self.mode == "unit":
            self.stage("setup", "skipped", "unit tests do not require the application stand")
        else:
            self.stage("setup", "running")
            self.compose("up", "-d", "--no-build", "--wait", "postgres")
            self.runner_task("setup", "setup.log")
            if self.mode in ("full", "e2e"):
                self.compose("up", "-d", "--no-build", "--no-recreate", "nginx")
                self.runner_task("ready", "ready.log")
            self.stage("setup", "passed")

        runner_args = ("--capture",) if self.capture else ()
        self.compose("run", "-d", "--no-deps", "--name", self.runner, "runner", "pipeline", *runner_args)
        with (self.results / "runner.log").open("w") as output:
            self.docker("logs", "--follow", self.runner, stdout=output, stderr=subprocess.STDOUT)
        return int(self.docker("wait", self.runner, capture_output=True).stdout.strip())

    def remove_models(self, log):
        containers = self.docker(
            "ps", "-aq", "--filter", "label=com.docker.compose.project=inference-models",
            stdout=subprocess.PIPE, stderr=log,
        ).stdout
        (self.results / "model-containers.txt").write_text(containers)
        for container in containers.splitlines():
            inspected = self.docker(
                "inspect", "--format", "{{.HostConfig.NetworkMode}}", container,
                stdout=subprocess.PIPE, stderr=log, check=False,
            )
            if inspected.returncode and self.exists("container", container):
                raise subprocess.CalledProcessError(inspected.returncode, inspected.args)
            if inspected.stdout.strip() == self.network:
                self.docker("rm", "-f", container, stdout=log, stderr=subprocess.STDOUT)

    def cleanup(self):
        errors = []
        with (self.results / "cleanup.log").open("a") as log:
            def attempt(action, function, *args, **options):
                try:
                    return function(*args, **options)
                except (OSError, subprocess.CalledProcessError) as error:
                    errors.append(action)
                    print(f"{action}: {error}", file=log, flush=True)
                    return None

            output = {"stdout": log, "stderr": subprocess.STDOUT}
            for container in (self.runner, f"{self.project}-setup", f"{self.project}-ready"):
                if self.exists("container", container):
                    self.docker("stop", "--time", "10", container, check=False, **output)
                    if self.exists("container", container):
                        attempt(f"remove {container}", self.docker, "rm", "-f", container, **output)
            postgres = None
            if self.mode != "unit":
                postgres = attempt("check PostgreSQL", self.compose, "ps", "-q", "postgres",
                                   stdout=subprocess.PIPE, stderr=log)
            if postgres and postgres.stdout.strip():
                with (self.results / "stand.log").open("w") as stand_log:
                    attempt("save stand logs", self.compose, "logs", "--no-color",
                            stdout=stand_log, stderr=subprocess.STDOUT)
                attempt("stop application services", self.compose, "stop", "-t", "20", *SERVICES, **output)
                attempt("remove model containers", self.remove_models, log)
                attempt("restore and verify S0", self.compose, "run", "--rm", "--no-deps", "-T",
                        "runner", "setup", **output)
            elif self.mode != "unit":
                errors.append("PostgreSQL was unavailable for S0 verification")
            attempt("remove Compose stand", self.compose, "down", "--volumes", "--remove-orphans", **output)
            stand_images = [f"{self.project}-{service}:latest" for service in
                            ("user-service", "model-service", "inference-service", "observability-service")]
            for image in (self.model_image, *stand_images):
                if self.exists("image", image):
                    attempt(f"remove {image}", self.docker, "image", "rm", image, **output)
            if errors:
                print("; ".join(errors), file=log)
        return errors

    def finish(self, status):
        errors = self.cleanup()
        if not self.exists("image", self.runner_image):
            return status or 1
        if errors:
            self.stage("cleanup", "failed", "; ".join(errors))
            status = status or 1
        elif self.mode == "unit":
            self.stage("cleanup", "passed", "unit runner resources removed")
        else:
            self.stage("cleanup", "passed", "S0 restored and verified; own stand removed")

        report_command = (
            "run", "--rm", "-e", f"TEST_MODE={self.mode}", "-v", f"{self.results}:/results",
            "-v", f"{self.history}:/history", self.runner_image, "report",
        )
        with (self.results / "report.log").open("w") as log:
            report = self.docker(*report_command, check=False, stdout=log, stderr=subprocess.STDOUT)
            status = status or report.returncode
        if self.runner_image == self.default_runner_image:
            with (self.results / "cleanup.log").open("a") as log:
                removal = self.docker("image", "rm", self.runner_image, check=False,
                                      stdout=log, stderr=subprocess.STDOUT)
            if removal.returncode:
                self.stage("cleanup", "failed", "runner image removal failed; inspect cleanup.log")
                with (self.results / "report.log").open("a") as log:
                    self.docker(*report_command, check=False, stdout=log, stderr=subprocess.STDOUT)
                status = status or 1
        return status


def interrupt(signum, _frame):
    raise SystemExit(128 + signum)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=(*RUN_MODES, "finalize"), nargs="?", default="full")
    parser.add_argument("--capture", action="store_true", help="record E2E HTTP traffic to a pcap file")
    args = parser.parse_args()
    if args.capture and args.mode != "e2e":
        parser.error("--capture requires e2e mode")
    try:
        pipeline = TestPipeline(args.mode, capture=args.capture)
        pipeline.initialize()
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        print(error, file=sys.stderr)
        return 2

    status = 1
    signal.signal(signal.SIGINT, interrupt)
    signal.signal(signal.SIGTERM, interrupt)
    try:
        status = pipeline.run()
    except subprocess.CalledProcessError as error:
        status = error.returncode
        setup = pipeline.results / "stages" / "setup.json"
        if not setup.is_file() or read_object(setup)["status"] == "running":
            pipeline.stage("setup", "failed", "stand preparation failed; see setup.log, ready.log and stand.log")
        print(error, file=sys.stderr)
    except OSError as error:
        print(error, file=sys.stderr)
    except SystemExit as error:
        status = error.code
    finally:
        signal.signal(signal.SIGINT, signal.SIG_IGN)
        signal.signal(signal.SIGTERM, signal.SIG_IGN)
        status = pipeline.finish(status)
    print(f"Test pipeline finished (exit {status}). Results: {pipeline.results}", flush=True)
    return status


if __name__ == "__main__":
    raise SystemExit(main())
