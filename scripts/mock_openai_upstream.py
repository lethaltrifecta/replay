#!/usr/bin/env python3

import argparse
import json
import time
from http.server import BaseHTTPRequestHandler, HTTPServer


class MockHandler(BaseHTTPRequestHandler):
    response_text = "Mock response from local upstream."
    model = "mock-model"

    def log_message(self, fmt, *args):
        print(fmt % args, flush=True)

    def do_POST(self):
        if self.path != "/v1/chat/completions":
            self.send_error(404, "unsupported path")
            return

        length = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(length)
        print(body.decode("utf-8"), flush=True)

        payload = {
            "id": "chatcmpl-mock",
            "object": "chat.completion",
            "created": int(time.time()),
            "model": self.model,
            "choices": [
                {
                    "index": 0,
                    "message": {
                        "role": "assistant",
                        "content": self.response_text,
                    },
                    "finish_reason": "stop",
                }
            ],
            "usage": {
                "prompt_tokens": 12,
                "completion_tokens": 7,
                "total_tokens": 19,
            },
        }
        data = json.dumps(payload).encode("utf-8")

        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)


def main():
    parser = argparse.ArgumentParser(description="Local OpenAI-compatible mock for agentgateway capture tests")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=18080)
    parser.add_argument("--model", default="mock-model")
    parser.add_argument("--response-text", default="Mock response from local upstream.")
    args = parser.parse_args()

    MockHandler.model = args.model
    MockHandler.response_text = args.response_text

    server = HTTPServer((args.host, args.port), MockHandler)
    server.serve_forever()


if __name__ == "__main__":
    main()
