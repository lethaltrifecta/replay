#!/usr/bin/env python3

import argparse
import json
import time
from http.server import BaseHTTPRequestHandler, HTTPServer


DEFAULT_TOOL_CALL_ID = "call_calculator_1"


class ToolLoopHandler(BaseHTTPRequestHandler):
    model = "mock-toolloop-model"
    tool_name = "calculator"
    tool_args = {"operation": "add", "a": 5, "b": 3}

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

        messages = payload.get("messages", [])
        tool_message = next((msg for msg in reversed(messages) if msg.get("role") == "tool"), None)

        if tool_message is None:
            self._json_response(
                {
                    "id": "chatcmpl-toolloop-1",
                    "object": "chat.completion",
                    "created": int(time.time()),
                    "model": self.model,
                    "choices": [
                        {
                            "index": 0,
                            "message": {
                                "role": "assistant",
                                "content": "",
                                "tool_calls": [
                                    {
                                        "id": DEFAULT_TOOL_CALL_ID,
                                        "type": "function",
                                        "function": {
                                            "name": self.tool_name,
                                            "arguments": json.dumps(self.tool_args, separators=(",", ":")),
                                        },
                                    }
                                ],
                            },
                            "finish_reason": "tool_calls",
                        }
                    ],
                    "usage": {
                        "prompt_tokens": 18,
                        "completion_tokens": 6,
                        "total_tokens": 24,
                    },
                }
            )
            return

        try:
            tool_payload = json.loads(tool_message.get("content", "{}"))
        except json.JSONDecodeError:
            tool_payload = {"raw": tool_message.get("content", "")}

        if "result" in tool_payload:
            final_text = f"The frozen tool result is {tool_payload['result']}."
        elif "error" in tool_payload:
            final_text = f"Tool execution failed: {tool_payload['error']}"
        else:
            final_text = f"Received frozen tool payload: {json.dumps(tool_payload, separators=(',', ':'))}"

        self._json_response(
            {
                "id": "chatcmpl-toolloop-2",
                "object": "chat.completion",
                "created": int(time.time()),
                "model": self.model,
                "choices": [
                    {
                        "index": 0,
                        "message": {
                            "role": "assistant",
                            "content": final_text,
                        },
                        "finish_reason": "stop",
                    }
                ],
                "usage": {
                    "prompt_tokens": 28,
                    "completion_tokens": 9,
                    "total_tokens": 37,
                },
            }
        )


def main():
    parser = argparse.ArgumentParser(description="Mock OpenAI-compatible upstream that forces one tool call")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=18081)
    parser.add_argument("--model", default="mock-toolloop-model")
    parser.add_argument("--tool-name", default="calculator")
    parser.add_argument("--tool-args", default='{"operation":"add","a":5,"b":3}')
    args = parser.parse_args()

    ToolLoopHandler.model = args.model
    ToolLoopHandler.tool_name = args.tool_name
    ToolLoopHandler.tool_args = json.loads(args.tool_args)

    server = HTTPServer((args.host, args.port), ToolLoopHandler)
    server.serve_forever()


if __name__ == "__main__":
    main()
