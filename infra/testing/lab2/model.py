"""HTTP model boundary: ignores llama arguments and never accesses /cache."""
from http.server import BaseHTTPRequestHandler, HTTPServer


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b'{"status":"ok"}')


HTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
