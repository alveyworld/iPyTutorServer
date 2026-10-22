#!/bin/bash

# Test server startup script for iPyIntervuServer with caching enabled

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
PORT=8765
SERVER_DIR="/home/manager/iPyIntervuServer/openrouter-app"
BINARY="$SERVER_DIR/openrouter-app"
LOG_FILE="$SERVER_DIR/server.log"

# Check if API key is set
if [ -z "$OPENROUTER_API_KEY" ]; then
    echo -e "${YELLOW}⚠️  OPENROUTER_API_KEY not set${NC}"
    echo "You need to set it before running: export OPENROUTER_API_KEY='your-key'"
    exit 1
fi

# Check if binary exists
if [ ! -f "$BINARY" ]; then
    echo -e "${YELLOW}Building server...${NC}"
    cd "$SERVER_DIR" && go build -o openrouter-app . || exit 1
fi

echo -e "${GREEN}Starting iPyIntervuServer with prompt caching${NC}"
echo "Port: $PORT"
echo "Log file: $LOG_FILE"
echo ""
echo "To test the server, open your browser to: http://localhost:$PORT"
echo "Press Ctrl+C to stop the server"
echo ""

# Run server with updated model that supports caching
export PORT=$PORT
export OPENROUTER_API_KEY="$OPENROUTER_API_KEY"

cd "$SERVER_DIR"
$BINARY 2>&1 | tee "$LOG_FILE"
