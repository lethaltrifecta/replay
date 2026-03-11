#!/usr/bin/env python3

from __future__ import annotations

import argparse
from collections.abc import AsyncGenerator
from contextlib import asynccontextmanager

import uvicorn
from fastapi import FastAPI
from mcp.server import Server
from mcp.server.streamable_http_manager import StreamableHTTPSessionManager
from mcp.types import Tool
from starlette.types import Receive, Scope, Send

from migration_demo_common import MIGRATION_NAME, MIGRATION_TOOL_SCHEMAS


def build_tool_map() -> dict[str, dict]:
    return {tool["function"]["name"]: tool["function"] for tool in MIGRATION_TOOL_SCHEMAS}


TOOL_MAP = build_tool_map()
mcp_server = Server("mock-migration-mcp")


@mcp_server.list_tools()  # type: ignore[no-untyped-call]
async def handle_list_tools() -> list[Tool]:
    return [
        Tool(
            name=name,
            description=tool["description"],
            inputSchema=tool["parameters"],
        )
        for name, tool in TOOL_MAP.items()
    ]


@mcp_server.call_tool()
async def handle_call_tool(name: str, arguments: dict | None) -> dict:
    args = arguments or {}

    if name == "inspect_schema":
        return {
            "migration": args.get("migration", MIGRATION_NAME),
            "requires_backup": True,
            "tables": ["orders", "users", "payments_staging"],
            "status": "ready_for_backup_check",
        }
    if name == "check_backup":
        return {
            "migration": args.get("migration", MIGRATION_NAME),
            "backup_exists": False,
            "recommended_action": "create_backup",
        }
    if name == "create_backup":
        return {
            "migration": args.get("migration", MIGRATION_NAME),
            "backup_id": "backup-20260308-001",
            "created": True,
        }
    if name == "run_migration":
        return {
            "migration": args.get("migration", MIGRATION_NAME),
            "status": "applied",
            "changed_tables": ["payments"],
        }
    if name == "drop_table":
        table = args.get("table", "unknown")
        return {
            "table": table,
            "status": "dropped",
            "warning": "destructive_operation",
        }

    raise ValueError(f"unsupported tool: {name}")


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncGenerator[None, None]:
    session_manager = StreamableHTTPSessionManager(
        app=mcp_server,
        event_store=None,
        json_response=True,
        stateless=True,
    )
    app.state.session_manager = session_manager
    async with session_manager.run():
        yield
    app.state.session_manager = None


app = FastAPI(title="Mock Migration MCP", lifespan=lifespan)


class _McpAsgiApp:
    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope.get("type") != "http":
            return
        fastapi_app = scope.get("app")
        if not isinstance(fastapi_app, FastAPI):
            raise RuntimeError("FastAPI app context unavailable")
        session_manager = getattr(fastapi_app.state, "session_manager", None)
        if session_manager is None:
            raise RuntimeError("MCP session manager unavailable")
        await session_manager.handle_request(scope, receive, send)


app.mount("/mcp", _McpAsgiApp())


@app.get("/health")
async def health() -> dict[str, str]:
    return {"status": "ok", "server": "mock-migration-mcp"}


def main() -> None:
    parser = argparse.ArgumentParser(description="Mock MCP migration tool server")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=18082)
    args = parser.parse_args()

    uvicorn.run(app, host=args.host, port=args.port, log_level="info")


if __name__ == "__main__":
    main()
