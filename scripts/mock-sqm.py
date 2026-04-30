#!/usr/bin/env python3
"""Minimal HTTP server that mimics the SQMeter ESP32 REST API using testdata."""
import sys
import os
from http.server import HTTPServer, BaseHTTPRequestHandler

BASE = os.path.join(os.path.dirname(__file__), '..', 'testdata', 'sqm')

with open(os.path.join(BASE, 'sensors.json'), 'rb') as f:
    SENSORS = f.read()
with open(os.path.join(BASE, 'status.json'), 'rb') as f:
    STATUS = f.read()

ROUTES = {
    '/api/sensors': SENSORS,
    '/api/status': STATUS,
}


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        body = ROUTES.get(self.path)
        if body is None:
            self.send_response(404)
            self.end_headers()
            return
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, fmt, *args):  # silence request logs
        pass


port = int(sys.argv[1]) if len(sys.argv) > 1 else 18080
print(f'mock-sqm listening on 127.0.0.1:{port}', flush=True)
HTTPServer(('127.0.0.1', port), Handler).serve_forever()
