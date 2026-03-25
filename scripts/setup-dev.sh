#!/bin/bash

# CMDR Local Development Setup Script

set -e

echo "🚀 Setting up CMDR local development environment..."

# Check if .env exists
if [ -f .env ]; then
    echo "✅ .env file already exists"
else
    echo "📝 Creating .env from .env.example..."
    cp .env.example .env
    echo "✅ Created .env file"
    echo "⚠️  Please review .env and adjust CMDR_AGENTGATEWAY_URL if needed"
fi

# Check if docker compose is available (V2 - newer)
if command -v docker &> /dev/null && docker compose version &> /dev/null; then
    DOCKER_COMPOSE="docker compose -f docker-compose.dev.yml"
# Check if docker-compose is available (V1 - older)
elif command -v docker-compose &> /dev/null; then
    DOCKER_COMPOSE="docker-compose -f docker-compose.dev.yml"
else
    echo "❌ Docker Compose not found. Please install Docker and Docker Compose."
    echo "   Visit: https://docs.docker.com/get-docker/"
    exit 1
fi

echo ""
echo "🐳 Starting PostgreSQL and Jaeger with $DOCKER_COMPOSE..."
$DOCKER_COMPOSE up -d postgres jaeger

echo ""
echo "⏳ Waiting for PostgreSQL to be ready..."
max_attempts=30
attempt=0
while ! $DOCKER_COMPOSE exec -T postgres pg_isready -U cmdr &> /dev/null; do
    attempt=$((attempt + 1))
    if [ $attempt -eq $max_attempts ]; then
        echo "❌ PostgreSQL failed to start after $max_attempts attempts"
        exit 1
    fi
    echo "   Attempt $attempt/$max_attempts..."
    sleep 1
done

echo "✅ PostgreSQL is ready"

echo ""
echo "📦 Downloading Go dependencies..."
go mod download
go mod tidy

echo ""
echo "🏗️  Building CMDR binary..."
make build

echo ""
echo "✅ Setup complete!"
echo ""
echo "📋 Next steps:"
echo "   1. Review and adjust .env file if needed"
echo "   2. Start CMDR service: make run"
echo "   3. Run tests: make test"
echo ""
echo "🔗 Service URLs:"
echo "   - CMDR health:    http://localhost:4318/health"
echo "   - OTLP gRPC:      localhost:4317"
echo "   - OTLP HTTP:      localhost:4318"
echo "   - Jaeger UI:      http://localhost:16686"
echo "   - PostgreSQL:     localhost:5432"
echo "   - Freeze-Tools:   separate service/repo"
echo ""
echo "📚 Documentation:"
echo "   - README.md"
echo "   - TODO.md"
echo "   - docs/DATABASE_LAYER.md"
echo ""
