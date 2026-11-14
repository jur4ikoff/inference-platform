"""Read-only load against model-service through the public Nginx route."""

import fcntl
import json
import os
import time
from pathlib import Path

import gevent
import requests
from gevent.lock import Semaphore
from locust import HttpUser, between, events, task


PROJECT = os.environ.get("MODEL_TEST_PROJECT", "stress-test")
TOKEN = None
TOKEN_REFRESH_AT = 0.0
TOKEN_TTL = 5 * 60
TOKEN_LOCK = Semaphore()
AUTH_CACHE = Path(os.environ.get("LOCUST_AUTH_CACHE", "/results/.login-cache.json"))


def cached_token(host, login, password):
    """Share one login across Locust processes using the common results volume."""
    AUTH_CACHE.parent.mkdir(parents=True, exist_ok=True)
    lock_path = AUTH_CACHE.with_suffix(".lock")
    lock_fd = os.open(lock_path, os.O_CREAT | os.O_RDWR, 0o600)
    with os.fdopen(lock_fd, "r+") as lock:
        while True:
            try:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                break
            except BlockingIOError:
                gevent.sleep(0.05)
        try:
            cached = json.loads(AUTH_CACHE.read_text())
            if (cached["host"] == host and cached["login"] == login
                    and 0 <= time.time() - cached["cached_at"] < TOKEN_TTL
                    and isinstance(cached["access_token"], str) and cached["access_token"]):
                return cached["access_token"], cached["cached_at"] + TOKEN_TTL
        except (OSError, ValueError, KeyError, TypeError):
            pass

        response = requests.post(
            f"{host}/api/v1/auth/login",
            json={"login": login, "password": password},
            timeout=10,
        )
        response.raise_for_status()
        data = response.json()
        token = data["access_token"]
        if not isinstance(token, str) or not token:
            raise ValueError("empty token")
        cached_at = time.time()
        cache = {"host": host, "login": login, "access_token": token, "cached_at": cached_at}
        temp_path = AUTH_CACHE.with_suffix(".tmp")
        fd = os.open(temp_path, os.O_CREAT | os.O_WRONLY | os.O_TRUNC, 0o600)
        with os.fdopen(fd, "w") as temp:
            json.dump(cache, temp)
        os.replace(temp_path, AUTH_CACHE)
        return token, cached_at + TOKEN_TTL


def get_token(host):
    global TOKEN, TOKEN_REFRESH_AT
    if TOKEN and time.time() < TOKEN_REFRESH_AT:
        return TOKEN
    with TOKEN_LOCK:
        if not TOKEN or time.time() >= TOKEN_REFRESH_AT:
            TOKEN, TOKEN_REFRESH_AT = cached_token(
                host.rstrip("/"),
                os.environ["BOOTSTRAP_ADMIN_LOGIN"],
                os.environ["BOOTSTRAP_ADMIN_PASSWORD"],
            )
    return TOKEN


@events.test_start.add_listener
def authenticate(environment, **_kwargs):
    try:
        get_token(environment.host)
    except (requests.RequestException, ValueError, KeyError, TypeError):
        raise SystemExit("Authentication failed; check the admin credentials and user-service.") from None


class ModelReader(HttpUser):
    wait_time = between(0.1, 0.3)

    @task
    def list_deployments(self):
        with self.client.get(
            f"/api/v1/projects/{PROJECT}/deployments",
            name="GET /api/v1/projects/{project}/deployments",
            headers={"Authorization": f"Bearer {get_token(self.environment.host)}"},
            catch_response=True,
        ) as response:
            # Locust reports transport errors with status 0 and preserves the
            # original exception when the response is not manually marked.
            if response.status_code == 0:
                return
            if response.status_code != 200:
                response.failure(f"HTTP {response.status_code}")
                return
            try:
                deployments = response.json().get("deployments")
            except (ValueError, AttributeError):
                response.failure("Invalid JSON response")
                return
            if not isinstance(deployments, list):
                response.failure("Response does not contain a deployments array")
