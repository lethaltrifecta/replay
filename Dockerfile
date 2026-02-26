# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git make

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/cmdr ./cmd/cmdr

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /bin/cmdr /bin/cmdr

# Expose ports
# 8080: HTTP API
# 4317: OTLP gRPC
# 4318: OTLP HTTP
# 9090: Freeze-Tools MCP
EXPOSE 8080 4317 4318 9090

ENTRYPOINT ["/bin/cmdr"]
CMD ["serve"]
