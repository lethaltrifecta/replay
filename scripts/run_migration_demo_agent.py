#!/usr/bin/env python3

import argparse
import json
import secrets
import sys
import time
from copy import deepcopy

import anyio
import httpx
from mcp.client.session import ClientSession
from mcp.client.streamable_http import streamable_http_client

from migration_demo_common import DEFAULT_PROMPT, MIGRATION_TOOL_SCHEMAS


def make_attr(key: str, value_type: str, value) -> dict:
    return {"key": key, "value": {value_type: value}}


def coerce_content(value) -> str:
    if value is None:
        return ""
    if isinstance(value, str):
        return value
    return json.dumps(value, separators=(",", ":"))


async def emit_turn_span(otlp_client: httpx.AsyncClient, args, turn: int, request_messages: list[dict], assistant_message: dict, usage: dict, tool_events: list[dict]) -> None:
    trace_id = args.trace_id
    span_id = secrets.token_hex(8)
    start_ns = time.time_ns()
    end_ns = start_ns + 100_000_000

    attributes = [
        make_attr("gen_ai.request.model", "stringValue", args.model),
        make_attr("gen_ai.provider.name", "stringValue", args.provider),
        make_attr("gen_ai.request.tools", "stringValue", json.dumps(MIGRATION_TOOL_SCHEMAS, separators=(",", ":"))),
        make_attr("demo.mode", "stringValue", args.mode),
        make_attr("demo.scenario", "stringValue", args.scenario),
        make_attr("demo.turn", "intValue", str(turn)),
    ]

    if args.mode == "replay" and args.freeze_trace_id:
        attributes.append(make_attr("demo.freeze_trace_id", "stringValue", args.freeze_trace_id))

    for index, message in enumerate(request_messages):
        attributes.append(make_attr(f"gen_ai.prompt.{index}.role", "stringValue", message["role"]))
        attributes.append(make_attr(f"gen_ai.prompt.{index}.content", "stringValue", coerce_content(message.get("content", ""))))
        if message.get("name"):
            attributes.append(make_attr(f"gen_ai.prompt.{index}.name", "stringValue", message["name"]))
        if message.get("tool_call_id"):
            attributes.append(make_attr(f"gen_ai.prompt.{index}.tool_call_id", "stringValue", message["tool_call_id"]))
        if message.get("tool_calls") is not None:
            attributes.append(
                make_attr(
                    f"gen_ai.prompt.{index}.tool_calls",
                    "stringValue",
                    json.dumps(message["tool_calls"], separators=(",", ":")),
                )
            )

    attributes.append(make_attr("gen_ai.completion.0.content", "stringValue", assistant_message.get("content", "")))
    attributes.append(make_attr("gen_ai.usage.input_tokens", "intValue", str(usage.get("prompt_tokens", 0))))
    attributes.append(make_attr("gen_ai.usage.output_tokens", "intValue", str(usage.get("completion_tokens", 0))))

    events = []
    for tool_event in tool_events:
        event_attrs = [
            make_attr("tool.name", "stringValue", tool_event["name"]),
            make_attr("tool.args", "stringValue", json.dumps(tool_event["args"], separators=(",", ":"))),
            make_attr("tool.latency_ms", "intValue", str(tool_event["latency_ms"])),
        ]
        if tool_event.get("result") is not None:
            event_attrs.append(
                make_attr("tool.result", "stringValue", json.dumps(tool_event["result"], separators=(",", ":")))
            )
        if tool_event.get("error"):
            event_attrs.append(make_attr("tool.error", "stringValue", tool_event["error"]))
        events.append(
            {
                "timeUnixNano": str(start_ns + ((len(events) + 1) * 10_000_000)),
                "name": "tool.call",
                "attributes": event_attrs,
            }
        )

    payload = {
        "resourceSpans": [
            {
                "resource": {
                    "attributes": [
                        make_attr("service.name", "stringValue", args.service_name),
                    ]
                },
                "scopeSpans": [
                    {
                        "spans": [
                            {
                                "traceId": trace_id,
                                "spanId": span_id,
                                "name": "llm.chat.completions",
                                "kind": 1,
                                "startTimeUnixNano": str(start_ns),
                                "endTimeUnixNano": str(end_ns),
                                "attributes": attributes,
                                "events": events,
                                "status": {"code": 1},
                            }
                        ]
                    }
                ],
            }
        ]
    }

    response = await otlp_client.post(args.otlp_url.rstrip("/") + "/v1/traces", json=payload)
    response.raise_for_status()


async def run_loop(args):
    llm_url = args.llm_url.rstrip("/") + "/v1/chat/completions"
    mcp_url = args.mcp_url if args.mcp_url.endswith("/") else args.mcp_url + "/"

    messages = [{"role": "system", "content": f"demo_behavior={args.behavior}"}]
    messages.append({"role": "user", "content": args.prompt})
    tool_error_seen = False
    final_text = ""

    mcp_headers = {}
    if args.freeze_trace_id:
        mcp_headers["X-Freeze-Trace-ID"] = args.freeze_trace_id

    async with httpx.AsyncClient(timeout=20.0) as llm_client:
        async with httpx.AsyncClient(headers=mcp_headers, timeout=20.0) as mcp_http_client:
            otlp_client = httpx.AsyncClient(timeout=10.0) if args.otlp_url else None
            try:
                async with streamable_http_client(
                    mcp_url,
                    http_client=mcp_http_client,
                    terminate_on_close=False,
                ) as (read_stream, write_stream, _):
                    async with ClientSession(read_stream, write_stream) as session:
                        await session.initialize()
                        listed_tools = await session.list_tools()
                        print(f"mcp tools/list => {[tool.name for tool in listed_tools.tools]}")
                        print(f"agent trace_id => {args.trace_id}")

                        for turn in range(args.max_turns):
                            request_messages = deepcopy(messages)
                            payload = {
                                "model": args.model,
                                "messages": request_messages,
                                "tools": MIGRATION_TOOL_SCHEMAS,
                                "stream": False,
                            }
                            response = await llm_client.post(llm_url, json=payload)
                            response.raise_for_status()
                            data = response.json()

                            choice = data["choices"][0]
                            message = choice["message"]
                            assistant_message = {
                                "role": "assistant",
                                "content": message.get("content", "") or "",
                            }
                            if "tool_calls" in message:
                                assistant_message["tool_calls"] = message["tool_calls"]
                            messages.append(assistant_message)

                            tool_events = []
                            tool_calls = message.get("tool_calls") or []
                            for tool_call in tool_calls:
                                tool_name = tool_call["function"]["name"]
                                tool_args = json.loads(tool_call["function"]["arguments"])
                                tool_start = time.monotonic()
                                result = await session.call_tool(tool_name, tool_args)
                                latency_ms = int((time.monotonic() - tool_start) * 1000)

                                structured = result.structuredContent or {}
                                error_message = None
                                if result.isError:
                                    tool_error_seen = True
                                    error_payload = structured.get("error", {}) if isinstance(structured, dict) else {}
                                    error_message = error_payload.get("message") or "tool call failed"

                                tool_events.append(
                                    {
                                        "name": tool_name,
                                        "args": tool_args,
                                        "result": structured,
                                        "error": error_message,
                                        "latency_ms": latency_ms,
                                    }
                                )

                                print(
                                    "tool call =>",
                                    json.dumps(
                                        {
                                            "name": tool_name,
                                            "args": tool_args,
                                            "result": result.model_dump(),
                                        },
                                        default=str,
                                    ),
                                )

                                tool_message_payload = structured
                                if result.isError and "error" not in tool_message_payload:
                                    tool_message_payload = {"error": {"message": error_message}}

                                messages.append(
                                    {
                                        "role": "tool",
                                        "name": tool_name,
                                        "tool_call_id": tool_call["id"],
                                        "content": json.dumps(tool_message_payload, separators=(",", ":")),
                                    }
                                )

                            usage = data.get("usage", {})
                            if otlp_client is not None:
                                await emit_turn_span(
                                    otlp_client,
                                    args,
                                    turn,
                                    request_messages,
                                    assistant_message,
                                    usage,
                                    tool_events,
                                )

                            if not tool_calls:
                                final_text = assistant_message["content"]
                                print(f"final assistant response => {final_text}")
                                break
                        else:
                            print("agent loop exceeded max turns", file=sys.stderr)
                            raise SystemExit(1)
            finally:
                if otlp_client is not None:
                    await otlp_client.aclose()

    if args.expect_final_substring and args.expect_final_substring not in final_text:
        print(
            f"expected final response to contain {args.expect_final_substring!r}, got {final_text!r}",
            file=sys.stderr,
        )
        raise SystemExit(1)

    if args.expect_tool_error and not tool_error_seen:
        print("expected to observe at least one tool error but none occurred", file=sys.stderr)
        raise SystemExit(1)


def main():
    parser = argparse.ArgumentParser(description="Run the database migration demo agent loop")
    parser.add_argument("--llm-url", required=True)
    parser.add_argument("--mcp-url", required=True)
    parser.add_argument("--model", default="migration-safe")
    parser.add_argument("--provider", default="mock-openai")
    parser.add_argument("--behavior", default="safe", choices=["safe", "unsafe"])
    parser.add_argument("--prompt", default=DEFAULT_PROMPT)
    parser.add_argument("--trace-id", default=secrets.token_hex(16))
    parser.add_argument("--freeze-trace-id")
    parser.add_argument("--otlp-url")
    parser.add_argument("--service-name", default="migration-demo-agent")
    parser.add_argument("--scenario", default="database-migration")
    parser.add_argument("--mode", default="capture")
    parser.add_argument("--max-turns", type=int, default=8)
    parser.add_argument("--expect-final-substring")
    parser.add_argument("--expect-tool-error", action="store_true")
    args = parser.parse_args()

    anyio.run(run_loop, args)


if __name__ == "__main__":
    main()
