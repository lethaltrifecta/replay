#!/usr/bin/env python3

import argparse
import json
import secrets
import time
import urllib.error
import urllib.request


DEFAULT_TOOLS = [
    {
        "type": "function",
        "function": {
            "name": "calculator",
            "description": "Perform basic arithmetic operations.",
            "parameters": {
                "type": "object",
                "properties": {
                    "operation": {"type": "string"},
                    "a": {"type": "number"},
                    "b": {"type": "number"},
                },
                "required": ["operation", "a", "b"],
            },
        },
    }
]


def attr(key, value_type, value):
    return {"key": key, "value": {value_type: value}}


def main():
    parser = argparse.ArgumentParser(description="Send a minimal CMDR baseline trace via OTLP HTTP")
    parser.add_argument("--otlp-url", default="http://127.0.0.1:4318")
    parser.add_argument("--trace-id", required=True)
    parser.add_argument("--model", default="mock-toolloop-model")
    parser.add_argument("--provider", default="mock-openai")
    parser.add_argument("--prompt", default="Use the calculator to add 5 and 3.")
    parser.add_argument("--completion", default="I'll use the calculator.")
    parser.add_argument("--tool-name", default="calculator")
    parser.add_argument("--tool-args", default='{"operation":"add","a":5,"b":3}')
    parser.add_argument("--tool-result", default='{"result":8}')
    parser.add_argument("--service-name", default="freeze-loop-capture")
    args = parser.parse_args()

    start_ns = time.time_ns()
    end_ns = start_ns + 100_000_000
    span_id = secrets.token_hex(8)

    tool_args = json.loads(args.tool_args)
    tool_result = json.loads(args.tool_result)
    tools_json = json.dumps(DEFAULT_TOOLS, separators=(",", ":"))

    payload = {
        "resourceSpans": [
            {
                "resource": {
                    "attributes": [
                        attr("service.name", "stringValue", args.service_name),
                    ]
                },
                "scopeSpans": [
                    {
                        "spans": [
                            {
                                "traceId": args.trace_id,
                                "spanId": span_id,
                                "name": "llm.chat.completions",
                                "kind": 1,
                                "startTimeUnixNano": str(start_ns),
                                "endTimeUnixNano": str(end_ns),
                                "attributes": [
                                    attr("gen_ai.request.model", "stringValue", args.model),
                                    attr("gen_ai.provider.name", "stringValue", args.provider),
                                    attr("gen_ai.prompt.0.role", "stringValue", "user"),
                                    attr("gen_ai.prompt.0.content", "stringValue", args.prompt),
                                    attr("gen_ai.request.tools", "stringValue", tools_json),
                                    attr("gen_ai.completion.0.content", "stringValue", args.completion),
                                    attr("gen_ai.usage.input_tokens", "intValue", "18"),
                                    attr("gen_ai.usage.output_tokens", "intValue", "6"),
                                ],
                                "events": [
                                    {
                                        "timeUnixNano": str(start_ns + 10_000_000),
                                        "name": "tool.call",
                                        "attributes": [
                                            attr("tool.name", "stringValue", args.tool_name),
                                            attr(
                                                "tool.args",
                                                "stringValue",
                                                json.dumps(tool_args, separators=(",", ":")),
                                            ),
                                            attr(
                                                "tool.result",
                                                "stringValue",
                                                json.dumps(tool_result, separators=(",", ":")),
                                            ),
                                            attr("tool.latency_ms", "intValue", "5"),
                                        ],
                                    }
                                ],
                                "status": {"code": 1},
                            }
                        ]
                    }
                ],
            }
        ]
    }

    request = urllib.request.Request(
        args.otlp_url.rstrip("/") + "/v1/traces",
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )

    try:
        with urllib.request.urlopen(request, timeout=10) as response:
            response.read()
    except urllib.error.HTTPError as exc:
        raise SystemExit(f"OTLP request failed with HTTP {exc.code}: {exc.read().decode('utf-8', errors='replace')}")
    except urllib.error.URLError as exc:
        raise SystemExit(f"OTLP request failed: {exc}")

    print(f"captured baseline trace_id={args.trace_id} via {args.otlp_url}")


if __name__ == "__main__":
    main()
