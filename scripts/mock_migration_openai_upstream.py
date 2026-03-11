#!/usr/bin/env python3

import argparse
import json
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

from migration_demo_common import MIGRATION_NAME, SAFE_FINAL_TEXT, SAFE_SEQUENCE, UNSAFE_BLOCKED_TEXT, UNSAFE_SEQUENCE


def build_tool_call(tool_name: str, args: dict, call_id: str) -> dict:
    return {
        "id": call_id,
        "type": "function",
        "function": {
            "name": tool_name,
            "arguments": json.dumps(args, separators=(",", ":")),
        },
    }


def final_response(content: str, model: str) -> dict:
    return {
        "id": f"chatcmpl-{model}-final",
        "object": "chat.completion",
        "created": int(time.time()),
        "model": model,
        "choices": [
            {
                "index": 0,
                "message": {
                    "role": "assistant",
                    "content": content,
                },
                "finish_reason": "stop",
            }
        ],
        "usage": {
            "prompt_tokens": 64,
            "completion_tokens": 16,
            "total_tokens": 80,
        },
    }


class MigrationHandler(BaseHTTPRequestHandler):
    model = "migration-safe"

    def log_message(self, fmt, *args):
        print(fmt % args, flush=True)

    def _json_response(self, payload):
        data = json.dumps(payload).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_POST(self):
        if self.path != "/v1/chat/completions":
            self.send_error(404, "unsupported path")
            return

        length = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(length)
        payload = json.loads(body.decode("utf-8"))
        print(json.dumps(payload), flush=True)

        model = payload.get("model", self.model)
        messages = payload.get("messages", [])
        behavior = "unsafe" if any("demo_behavior=unsafe" in msg.get("content", "") for msg in messages) else "safe"
        tool_messages = [msg for msg in messages if msg.get("role") == "tool"]
        completed_steps = len(tool_messages)

        if behavior == "unsafe":
            if completed_steps == 0:
                tool_name, tool_args = UNSAFE_SEQUENCE[0]
                self._json_response(
                    {
                        "id": "chatcmpl-migration-unsafe-1",
                        "object": "chat.completion",
                        "created": int(time.time()),
                        "model": model,
                        "choices": [
                            {
                                "index": 0,
                                "message": {
                                    "role": "assistant",
                                    "content": "",
                                    "tool_calls": [
                                        build_tool_call(tool_name, tool_args, "call_drop_table_1")
                                    ],
                                },
                                "finish_reason": "tool_calls",
                            }
                        ],
                        "usage": {
                            "prompt_tokens": 24,
                            "completion_tokens": 8,
                            "total_tokens": 32,
                        },
                    }
                )
                return

            try:
                tool_payload = json.loads(tool_messages[-1].get("content", "{}"))
            except json.JSONDecodeError:
                tool_payload = {"raw": tool_messages[-1].get("content", "")}

            if "error" in tool_payload:
                self._json_response(final_response(UNSAFE_BLOCKED_TEXT, model))
                return

            self._json_response(final_response("Unsafe migration path completed.", model))
            return

        if completed_steps < len(SAFE_SEQUENCE):
            tool_name, tool_args = SAFE_SEQUENCE[completed_steps]
            self._json_response(
                {
                    "id": f"chatcmpl-migration-safe-{completed_steps + 1}",
                    "object": "chat.completion",
                    "created": int(time.time()),
                    "model": model,
                    "choices": [
                        {
                            "index": 0,
                            "message": {
                                "role": "assistant",
                                "content": "",
                                "tool_calls": [
                                    build_tool_call(
                                        tool_name,
                                        tool_args,
                                        f"call_{tool_name}_{completed_steps + 1}",
                                    )
                                ],
                            },
                            "finish_reason": "tool_calls",
                        }
                    ],
                    "usage": {
                        "prompt_tokens": 24 + (completed_steps * 8),
                        "completion_tokens": 8,
                        "total_tokens": 32 + (completed_steps * 8),
                    },
                }
            )
            return

        self._json_response(final_response(SAFE_FINAL_TEXT, model))


def main():
    parser = argparse.ArgumentParser(description="Mock OpenAI-compatible upstream for the migration demo")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=18083)
    args = parser.parse_args()

    print(f"mock migration upstream ready for {MIGRATION_NAME}", flush=True)
    server = HTTPServer((args.host, args.port), MigrationHandler)
    server.serve_forever()


if __name__ == "__main__":
    main()
