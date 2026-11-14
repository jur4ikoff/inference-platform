#!/usr/bin/env python3
"""Check Grafana's subpath and asset loading with an expired session."""

import re
import unittest

from check_cache_headers import get


class GrafanaProxyTest(unittest.TestCase):
    def test_redirect_and_browser_script_with_expired_session(self):
        status, headers, _ = get("/monitoring")
        self.assertEqual(status, 308)
        self.assertEqual(headers.get("Location"), "/monitoring/")

        status, _, page = get("/monitoring/login")
        self.assertEqual(status, 200)
        match = re.search(rb'src="(public/build/runtime\.[^"]+\.js)"', page)
        self.assertIsNotNone(match, "Grafana runtime script was not found")

        status, headers, body = get(
            "/monitoring/" + match.group(1).decode(),
            headers={
                "Host": "localhost:8000",
                "Origin": "http://localhost:8000",
                "Cookie": "grafana_session=0123456789abcdef0123456789abcdef; grafana_session_expiry=4102444800",
            },
        )
        self.assertEqual(status, 200)
        self.assertIn("javascript", headers.get("Content-Type", ""))
        self.assertTrue(body)


if __name__ == "__main__":
    unittest.main()
