#!/usr/bin/env python3

import argparse
import json
import sys

import anyio
import httpx
from mcp.client.session import ClientSession
from mcp.client.streamable_http import streamable_http_client


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


async def run_loop(args):
    llm_url = args.llm_url.rstrip("/") + "/v1/chat/completions"
    mcp_url = args.mcp_url
    if not mcp_url.endswith("/"):
        mcp_url += "/"

    messages = [{"role": "user", "content": args.prompt}]

    async with httpx.AsyncClient(timeout=15.0) as llm_client:
        async with httpx.AsyncClient(headers={"X-Freeze-Trace-ID": args.freeze_trace_id}, timeout=15.0) as mcp_http_client:
            async with streamable_http_client(
                mcp_url,
                http_client=mcp_http_client,
                terminate_on_close=False,
            ) as (
                read_stream,
                write_stream,
                _,
            ):
                async with ClientSession(read_stream, write_stream) as session:
                    await session.initialize()
                    tools = await session.list_tools()
                    tool_names = [tool.name for tool in tools.tools]
                    print(f"freeze-mcp tools/list => {tool_names}")

                    final_text = ""

                    for turn in range(args.max_turns):
                        payload = {
                            "model": args.model,
                            "messages": messages,
                            "tools": DEFAULT_TOOLS,
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

                        tool_calls = message.get("tool_calls") or []
                        if not tool_calls:
                            final_text = assistant_message["content"]
                            print(f"final assistant response => {final_text}")
                            break

                        for tool_call in tool_calls:
                            tool_name = tool_call["function"]["name"]
                            tool_args = json.loads(tool_call["function"]["arguments"])
                            result = await session.call_tool(tool_name, tool_args)
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

                            if result.isError:
                                print("freeze-mcp returned isError=true", file=sys.stderr)
                                raise SystemExit(1)

                            messages.append(
                                {
                                    "role": "tool",
                                    "tool_call_id": tool_call["id"],
                                    "content": json.dumps(result.structuredContent, separators=(",", ":")),
                                }
                            )
                    else:
                        print("agent loop exceeded max turns", file=sys.stderr)
                        raise SystemExit(1)

    if args.expect_substring and args.expect_substring not in final_text:
        print(
            f"expected final response to contain {args.expect_substring!r}, got {final_text!r}",
            file=sys.stderr,
        )
        raise SystemExit(1)


def main():
    parser = argparse.ArgumentParser(description="Run a minimal tool-calling agent loop against freeze-mcp")
    parser.add_argument("--llm-url", default="http://127.0.0.1:3002")
    parser.add_argument("--mcp-url", default="http://127.0.0.1:9090/mcp/")
    parser.add_argument("--freeze-trace-id", required=True)
    parser.add_argument("--model", default="mock-toolloop-model")
    parser.add_argument("--prompt", default="Use the calculator to add 5 and 3.")
    parser.add_argument("--max-turns", type=int, default=4)
    parser.add_argument("--expect-substring", default="8")
    args = parser.parse_args()

    anyio.run(run_loop, args)


if __name__ == "__main__":
    main()
