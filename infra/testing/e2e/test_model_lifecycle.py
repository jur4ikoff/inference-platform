"""Exercise the model lifecycle through the real HTTP gateway."""
from contextlib import contextmanager, nullcontext, redirect_stdout
from http import HTTPStatus
import http.client
import json
import os
import signal
import subprocess
import time
import unittest

from prepare_test_database import ADMIN_LOGIN, ADMIN_PASSWORD, RESULTS


MODEL_REPOSITORY = "ggml-org/tiny-llamas"
MODEL_NAME = "stories260k"


class ModelLifecycleTest(unittest.TestCase):
    def setUp(self):
        self.token = None

    def request(self, method, path, *expected, body=None, idempotency_key=None, authenticated=True):
        headers = {"Content-Type": "application/json"}
        if authenticated and self.token:
            headers["Authorization"] = f"Bearer {self.token}"
        if idempotency_key:
            headers["Idempotency-Key"] = idempotency_key
        connection = http.client.HTTPConnection("nginx", timeout=20)
        try:
            connection.request(method, path, json.dumps(body) if body is not None else None, headers)
            response = connection.getresponse()
            content = response.read().decode()
        finally:
            connection.close()
        print(f"{method} {path} -> {response.status}", flush=True)
        self.assertIn(response.status, expected, f"{method} {path}: {content}")
        if response.status >= 400 or not content:
            return response.status, None
        return response.status, json.loads(content)

    def test_model_lifecycle(self):
        project = "e2e-model-lifecycle"
        project_path = f"/api/v1/projects/{project}"
        _, login = self.request(
            "POST", "/api/v1/auth/login", HTTPStatus.OK,
            body={"login": ADMIN_LOGIN, "password": ADMIN_PASSWORD},
        )
        self.token = login["access_token"]
        self.assertTrue(self.token)
        self.request(
            "POST", "/api/v1/projects", HTTPStatus.CREATED,
            body={"name": project, "description": "E2E model lifecycle"},
        )
        _, grant = self.request(
            "POST", "/api/v1/auth/project-token", HTTPStatus.OK, body={"project_name": project},
        )
        self.token = grant["access_token"]
        self.assertTrue(self.token)
        _, model = self.request(
            "POST", f"{project_path}/models", HTTPStatus.CREATED,
            body={
                "name": MODEL_NAME,
                "spec": {
                    "provider_type": "HUGGING_FACE",
                    "provider_hugging_face": {"repo": MODEL_REPOSITORY},
                },
            },
        )
        self.assertTrue(model["id"])
        model_path = f"{project_path}/models/{model['id']}"
        deployment_path = f"{model_path}/deployment"

        _, deployment = self.request(
            "POST", deployment_path, HTTPStatus.ACCEPTED, idempotency_key="e2e-model-start",
        )
        self.assertTrue(deployment["id"])
        for _ in range(45):
            _, current = self.request("GET", deployment_path, HTTPStatus.OK)
            self.assertEqual(current["id"], deployment["id"])
            self.assertNotEqual(current["status"], "failed", current)
            if current["status"] == "running":
                break
            time.sleep(1)
        else:
            self.fail(f"Deployment did not reach running: {current}")

        self.check_inference(f"/inference/{project}/{MODEL_NAME}")

        # The interruption demo pauses while the deployment is still running.
        if os.environ.get("TEST_PAUSE_STAGE") == "e2e":
            (RESULTS / "pause-ready").touch()
            time.sleep(300)
        self.assertNotEqual(os.environ.get("TEST_FAIL_STAGE"), "e2e", "Injected E2E failure")

        self.request("DELETE", deployment_path, HTTPStatus.ACCEPTED, HTTPStatus.NO_CONTENT)
        for _ in range(45):
            status, current = self.request("GET", deployment_path, HTTPStatus.OK, HTTPStatus.NOT_FOUND)
            if status == HTTPStatus.NOT_FOUND or current["status"] == "stopped":
                break
            time.sleep(1)
        else:
            self.fail(f"Deployment did not stop: {current}")

        _, deployments = self.request("GET", f"{project_path}/deployments", HTTPStatus.OK)
        self.assertEqual(deployments["deployments"], [])

    def check_inference(self, path):
        # Docker running status can precede readiness of llama-server and its proxy route.
        deadline = time.monotonic() + 120
        while True:
            status, health = self.request(
                "GET", f"{path}/health", HTTPStatus.OK, HTTPStatus.NOT_FOUND,
                HTTPStatus.BAD_GATEWAY, HTTPStatus.SERVICE_UNAVAILABLE, authenticated=False,
            )
            if status == HTTPStatus.OK:
                self.assertEqual(health["status"], "ok")
                break
            if time.monotonic() >= deadline:
                self.fail(f"Model did not become ready: last HTTP status {status}")
            time.sleep(1)

        _, completion = self.request(
            "POST", f"{path}/completion", HTTPStatus.OK, authenticated=False,
            body={"prompt": "Once upon a time", "n_predict": 8, "temperature": 0, "ignore_eos": True},
        )
        self.assertIsInstance(completion["content"], str)
        self.assertTrue(completion["content"].strip(), completion)
        print(f"Model completion: {completion['content']!r}")


@contextmanager
def capture_http():
    capture_log = RESULTS / "capture.log"
    command = ("tcpdump", "-i", "eth0", "-p", "-nn", "-s", "0", "--immediate-mode", "-U", "-w", "-",
               "host nginx and tcp port 80")
    with (RESULTS / "model-lifecycle-auto.pcap").open("wb") as dump, capture_log.open("w") as log:
        with subprocess.Popen(command, stdout=dump, stderr=log) as capture:
            try:
                deadline = time.monotonic() + 5
                while "listening on " not in capture_log.read_text():
                    if capture.poll() is not None or time.monotonic() >= deadline:
                        raise RuntimeError("tcpdump did not start; see capture.log")
                    time.sleep(0.05)
                yield
            finally:
                if capture.poll() is None:
                    capture.send_signal(signal.SIGINT)
                try:
                    status = capture.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    capture.kill()
                    raise
            if status != 0:
                raise subprocess.CalledProcessError(status, command)


def run_e2e(*, capture=False):
    suite = unittest.defaultTestLoader.loadTestsFromTestCase(ModelLifecycleTest)
    packet_capture = capture_http() if capture else nullcontext()
    with packet_capture, (RESULTS / "e2e.log").open("w") as log, redirect_stdout(log):
        result = unittest.TextTestRunner(stream=log, verbosity=2).run(suite)
    return 0 if result.wasSuccessful() else 1


if __name__ == "__main__":
    raise SystemExit(run_e2e())
