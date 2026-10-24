#!/usr/bin/env python3
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse


TEST_TOKEN = "EUROMNI_LOCAL_FAKE_TOKEN_NOT_A_CREDENTIAL"
with open("/app/catalog-products.json", encoding="utf-8") as fixture_file:
    catalog_products = json.load(fixture_file)
state = {
    "product_lookups": 0,
    "product_lookup_ids": {},
    "catalog_product_lookups": 0,
    "catalog_error_lookups": 0,
    "shipping_zone_lookups": 0,
    "store_lookups": 0,
    "order_lookups": {},
    "order_posts": 0,
    "payload_valid": False,
}


class Handler(BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        return

    def send_json(self, status, payload, headers=None):
        body = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        for name, value in (headers or {}).items():
            self.send_header(name, value)
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        path = urlparse(self.path).path.rstrip("/") or "/"
        if path == "/health":
            self.send_json(200, {"status": "ok"})
            return

        if path == "/products":
            if self.headers.get("Authorization") != f"Bearer {TEST_TOKEN}":
                self.send_json(401, {"message": "Unauthorized"})
                return
            state["catalog_product_lookups"] += 1
            self.send_json(
                200,
                catalog_products,
                {
                    "X-Pagination-Current-Page": "1",
                    "X-Pagination-Page-Count": "1",
                    "X-Pagination-Total-Count": "1",
                    "X-Pagination-Per-Page": "10",
                },
            )
            return

        if path == "/unauthorized/products":
            state["catalog_error_lookups"] += 1
            self.send_json(401, {"message": "Unauthorized"})
            return

        if path == "/shipping-zones":
            if self.headers.get("Authorization") != "Bearer " + TEST_TOKEN:
                self.send_json(401, {"message": "Unauthorized"})
                return
            state["shipping_zone_lookups"] += 1
            self.send_json(
                200,
                [
                    {
                        "id": 71,
                        "shippingGroup": "PILOT-EU-STANDARD",
                        "countries": ["FR", "BE"],
                        "rates": [
                            {"totalWeightFrom": 0, "totalWeightTo": 1000, "price": 4.5},
                            {"totalWeightFrom": 1001, "totalWeightTo": 5000, "price": 7.25},
                        ],
                    }
                ],
                {
                    "X-Pagination-Current-Page": "1",
                    "X-Pagination-Page-Count": "1",
                    "X-Pagination-Total-Count": "1",
                    "X-Pagination-Per-Page": "50",
                },
            )
            return

        if path == "/stores":
            if self.headers.get("Authorization") != "Bearer " + TEST_TOKEN:
                self.send_json(401, {"message": "Unauthorized"})
                return
            state["store_lookups"] += 1
            self.send_json(
                200,
                [
                    {
                        "platformAccessToken": "EUROMNI_LOCAL_FAKE_CALLBACK_TOKEN",
                        "isUsingCustomShippingProfile": True,
                    }
                ],
                {
                    "X-Pagination-Current-Page": "1",
                    "X-Pagination-Page-Count": "1",
                    "X-Pagination-Total-Count": "1",
                    "X-Pagination-Per-Page": "20",
                },
            )
            return

        if path.startswith("/orders/"):
            if self.headers.get("Authorization") != "Bearer " + TEST_TOKEN:
                self.send_json(401, {"message": "Unauthorized"})
                return
            order_id = path.rsplit("/", 1)[-1]
            state["order_lookups"][order_id] = state["order_lookups"].get(order_id, 0) + 1
            if order_id == "9001":
                self.send_json(
                    200,
                    {
                        "id": 9001,
                        "status": "SHIPPED",
                        "plannedDeliveryAt": "2026-11-02T12:00:00Z",
                        "trackingIds": ["PILOT-TRACK-9001", "PILOT-TRACK-9001-B"],
                        "trackingUrls": [
                            "https://tracking.example.invalid/9001",
                            "https://tracking.example.invalid/9001-b",
                        ],
                    },
                )
            elif order_id == "9002":
                self.send_json(
                    200,
                    {
                        "id": 9002,
                        "status": "CANCELLED",
                        "cancelReason": "Synthetic cancellation",
                        "plannedDeliveryAt": "",
                        "trackingIds": [],
                        "trackingUrls": [],
                    },
                )
            else:
                self.send_json(404, {"message": "Not found"})
            return

        if path.startswith("/products/"):
            if self.headers.get("Authorization") != f"Bearer {TEST_TOKEN}":
                self.send_json(401, {"message": "Unauthorized"})
                return
            state["product_lookups"] += 1
            product_id = path.rsplit("/", 1)[-1]
            state["product_lookup_ids"][product_id] = state["product_lookup_ids"].get(product_id, 0) + 1
            if product_id == "842002":
                self.send_json(
                    200,
                    {
                        "id": 842002,
                        "name": "Synthetic Callback Balm",
                        "description": "Updated only by the local callback fixture.",
                        "price": 31.25,
                        "volume": 210,
                        "weight": 250,
                        "isInStock": False,
                        "images": [],
                        "image": "",
                        "tags": ["callback"],
                    },
                )
            else:
                self.send_json(200, {"id": 842001, "weight": 120, "volume": 320})
            return

        if path == "/assertions":
            self.send_json(
                200,
                {
                    "product_lookups": state["product_lookups"],
                    "product_lookup_ids": state["product_lookup_ids"],
                    "catalog_product_lookups": state["catalog_product_lookups"],
                    "catalog_error_lookups": state["catalog_error_lookups"],
                    "shipping_zone_lookups": state["shipping_zone_lookups"],
                    "store_lookups": state["store_lookups"],
                    "order_lookups": state["order_lookups"],
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
