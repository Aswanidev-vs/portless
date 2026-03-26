#!/bin/bash
# GoTunnel Quick Start Script
# This script demonstrates basic GoTunnel usage

set -e

echo "🚀 GoTunnel Quick Start"
echo "========================"

# Check if gotunnel is installed
if ! command -v gotunnel &> /dev/null; then
    echo "❌ GoTunnel is not installed"
    echo "Install it with: go install ./cmd/gotunnel"
    exit 1
fi

# Check if token is set
if [ -z "$GOTUNNEL_TOKEN" ]; then
    echo "❌ GOTUNNEL_TOKEN environment variable is not set"
    echo "Set it with: export GOTUNNEL_TOKEN=your-token"
    exit 1
fi

echo "✅ GoTunnel installed: $(gotunnel version)"
echo "✅ Token configured"

# Start a simple HTTP tunnel
echo ""
echo "📝 Starting HTTP tunnel for localhost:3000..."
echo "Press Ctrl+C to stop"

gotunnel tunnel start \
    --name quickstart \
    --protocol http \
    --local-url http://localhost:3000 \
    --subdomain quickstart-demo \
    --inspect true