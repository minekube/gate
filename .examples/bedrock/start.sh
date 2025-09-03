#!/bin/bash

# Gate + Geyser Bedrock Support Example
# This script helps you get started with Bedrock support in Gate

set -e

echo "🚀 Starting Gate + Geyser Bedrock Support Example"
echo "================================================="

# Check if Docker is running
if ! docker info > /dev/null 2>&1; then
    echo "❌ Docker is not running. Please start Docker and try again."
    exit 1
fi

# Check if key.pem exists
if [ ! -f "geyser/key.pem" ]; then
    echo "🔑 Generating Floodgate key..."
    openssl genpkey -algorithm RSA -out geyser/key.pem
    echo "✅ Floodgate key generated at geyser/key.pem"
else
    echo "✅ Floodgate key already exists"
fi

# Start services
echo "📦 Starting Docker services..."
docker compose up -d

echo ""
echo "🎉 Services started successfully!"
echo ""
echo "📊 Service Status:"
docker compose ps

echo ""
echo "🎮 Connection Information:"
echo "  Java Players:    localhost:25565"
echo "  Bedrock Players: localhost:19132"
echo ""
echo "📋 Useful Commands:"
echo "  View logs:       docker compose logs -f"
echo "  Stop services:   docker compose down"
echo "  Restart:         docker compose restart"
echo ""
echo "🔍 Troubleshooting:"
echo "  - Check logs if players can't connect"
echo "  - Ensure ports 25565 (Java) and 19132/udp (Bedrock) are open"
echo "  - Verify the Floodgate key is shared between all services"
echo ""
echo "📖 Documentation: https://gate.minekube.com/guide/bedrock"
