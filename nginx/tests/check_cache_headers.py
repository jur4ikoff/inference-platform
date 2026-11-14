#!/usr/bin/env python3
"""Check cache and compression headers of the running local Nginx."""

import gzip
import http.client
import json
import unittest


def get(path, *, port=8000, headers=None):
    connection = http.client.HTTPConnection("127.0.0.1", port, timeout=10)
    try:
        connection.request("GET", path, headers=headers or {})
        response = connection.getresponse()
        return response.status, response.headers, response.read()
    finally:
        connection.close()


class CacheHeadersTest(unittest.TestCase):
    def test_public_assets_and_revalidation(self):
        for path in ("/static/home.css", "/mirror/static/swagger/swagger-ui.css"):
            with self.subTest(path=path):
                status, headers, _ = get(path)
                self.assertEqual(status, 200)
                self.assertEqual(headers.get("Cache-Control"), "public, max-age=3600")
                self.assertIsNotNone(headers.get("ETag"))
                status, cached, _ = get(path, headers={"If-None-Match": headers["ETag"]})
                self.assertEqual(status, 304)
                self.assertEqual(cached.get("Cache-Control"), "public, max-age=3600")

    def test_gzip_and_vary(self):
        path = "/static/home.css"
        status, headers, plain = get(path)
        compressed_status, compressed, body = get(path, headers={"Accept-Encoding": "gzip"})
        self.assertEqual((status, compressed_status), (200, 200))
        self.assertEqual(compressed.get("Content-Encoding"), "gzip")
        self.assertIn("Accept-Encoding", compressed.get("Vary", ""))
        self.assertIn("Accept-Encoding", headers.get("Vary", ""))
        self.assertEqual(gzip.decompress(body), plain)

        status, headers, body = get(
            "/mirror/api/v1/openapi.json", headers={"Accept-Encoding": "gzip"}
        )
        self.assertEqual(status, 200)
        self.assertEqual(headers.get("Content-Encoding"), "gzip")
        self.assertEqual(headers.get("Cache-Control"), "no-store")
        self.assertIn("paths", json.loads(gzip.decompress(body)))

        status, headers, _ = get(
            "/static/img/interface-sketch.png", headers={"Accept-Encoding": "gzip"}
        )
        self.assertEqual(status, 200)
        self.assertIsNone(headers.get("Content-Encoding"))

    def test_non_cacheable_responses(self):
        paths = (
            (8000, "/"), (8000, "/management"), (8000, "/status"),
            (8000, "/status/data"), (8000, "/health"), (8000, "/ready"),
            (8000, "/admin"), (8000, "/api/v1/projects/ml/models"),
            (8000, "/mirror/api/v1/openapi.json"),
            (8000, "/static/management.html"), (8000, "/static/does-not-exist.css"),
            (8083, "/admin/"),
        )
        for port, path in paths:
            with self.subTest(port=port, path=path):
                _, headers, _ = get(path, port=port)
                self.assertEqual(headers.get_all("Cache-Control"), ["no-store"])


if __name__ == "__main__":
    unittest.main()
