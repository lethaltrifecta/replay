# Dev Dockerfile for freeze-mcp — used by docker-compose.dev.yml
# Build context must be the parent directory containing both repos:
#   context: ..
#   dockerfile: replay/dockerfiles/freeze-mcp.dev.Dockerfile
#
# This injects a go.mod replace directive so freeze-mcp can resolve
# the replay module from the local source tree instead of GitHub.

FROM golang:1.26 AS build

WORKDIR /src

# Copy both module trees for dependency resolution
COPY replay/go.mod replay/go.sum ./replay/
COPY freeze-mcp/go.mod freeze-mcp/go.sum ./freeze-mcp/

# Inject replace directive so freeze-mcp resolves replay locally
RUN cd freeze-mcp && go mod edit -replace github.com/lethaltrifecta/replay=../replay

# Download deps (replay is local, everything else from proxy)
RUN cd replay && go mod download
RUN cd freeze-mcp && go mod download

# Copy full source trees
COPY replay/ ./replay/
COPY freeze-mcp/ ./freeze-mcp/

# Re-apply replace directive (COPY overwrites the modified go.mod)
RUN cd freeze-mcp && go mod edit -replace github.com/lethaltrifecta/replay=../replay

# Build
RUN cd freeze-mcp && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /freeze-mcp ./cmd/freeze-mcp
RUN cd freeze-mcp && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /freeze-mcp-migrate ./cmd/freeze-mcp-migrate

FROM debian:bookworm-slim

WORKDIR /app
COPY --from=build /freeze-mcp /usr/local/bin/freeze-mcp
COPY --from=build /freeze-mcp-migrate /usr/local/bin/freeze-mcp-migrate

EXPOSE 9090

RUN printf '#!/bin/sh\nset -eu\nexec wget -q -O - http://127.0.0.1:9090/health >/dev/null\n' > /usr/local/bin/freeze-mcp-healthcheck \
    && chmod +x /usr/local/bin/freeze-mcp-healthcheck \
    && apt-get update \
    && apt-get install -y --no-install-recommends wget ca-certificates \
    && rm -rf /var/lib/apt/lists/*

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/freeze-mcp-healthcheck"]

CMD ["freeze-mcp"]
