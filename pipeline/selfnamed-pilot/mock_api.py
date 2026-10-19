#!/usr/bin/env python3
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse


TEST_TOKEN = "EUROMNI_LOCAL_FAKE_TOKEN_NOT_A_CREDENTIAL"
state = {"product_lookups": 0, "order_posts": 0, "payload_valid": False}


class Handler(BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        return

    def send_json(self, status, payload):
        body = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        path = urlparse(self.path).path
        if path == "/health":
            self.send_json(200, {"status": "ok"})
            return

        if path.startswith("/products/"):
            if self.headers.get("Authorization") != f"Bearer {TEST_TOKEN}":
                self.send_json(401, {"message": "Unauthorized"})
                return
            state["product_lookups"] += 1
            self.send_json(200, {"id": 842001, "weight": 120, "volume": 320})
            return

        if path == "/assertions":
            self.send_json(
                200,
                {
                    "product_lookups": state["product_lookups"],
                    "order_posts": state["order_posts"],
                    "payload_valid": state["payload_valid"],
                },
            )
            return

        self.send_json(404, {"message": "Not found"})

    def do_POST(self):
        if urlparse(self.path).path != "/orders":
            self.send_json(404, {"message": "Not found"})
            return
        if self.headers.get("Authorization") != f"Bearer {TEST_TOKEN}":
            self.send_json(401, {"message": "Unauthorized"})
            return

        length = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(length).decode()
        fields = parse_qs(body, keep_blank_values=True)

        def field(name):
            return fields.get(name, [""])[0]

        state["order_posts"] += 1
        state["payload_valid"] = (
            field("orderItems[0][productId]") == "842001"
            and field("orderItems[0][quantity]") == "2"
            and field("shippingGroup") == "pilot-eu-standard"
            and field("invoice[buyer][firstName]") == "Synthetic"
            and field("invoice[buyer][lastName]") == "Buyer"
            and field("invoice[buyer][email]") == "buyer@example.invalid"
            and field("invoice[shippingAddress][city]") == "Testville"
        )
        self.send_json(
            201,
            {
                "id": 9001,
                "status": "PROCESSING",
                "plannedDeliveryAt": "2026-11-01T12:00:00Z",
            },
        )


ThreadingHTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
