#!/bin/bash
# Simple socket client for ngrokd daemon

SOCKET_PATH="${1:-/tmp/ngrokd-test.sock}"

if [ ! -S "$SOCKET_PATH" ]; then
    echo "Error: Socket not found at $SOCKET_PATH"
    echo "Usage: $0 [socket-path]"
    exit 1
fi

echo "Connected to: $SOCKET_PATH"
echo ""
echo "Available commands:"
echo "  1) status"
echo "  2) list"
echo "  3) set-api-key <key>"
echo "  4) custom JSON"
echo "  q) quit"
echo ""

while true; do
    echo -n "> "
    read -r cmd args
    
    case "$cmd" in
        q|quit|exit)
            break
            ;;
        1|status)
            echo '{"command":"status"}' | nc -U "$SOCKET_PATH" | jq '.'
            ;;
        2|list)
            echo '{"command":"list"}' | nc -U "$SOCKET_PATH" | jq '.'
            ;;
        3|set-api-key)
            if [ -z "$args" ]; then
                echo "Usage: set-api-key <api-key>"
            else
                echo "{\"command\":\"set-api-key\",\"args\":[\"$args\"]}" | nc -U "$SOCKET_PATH" | jq '.'
            fi
            ;;
        4|custom)
            echo "Enter JSON:"
            read -r json
            echo "$json" | nc -U "$SOCKET_PATH" | jq '.'
            ;;
        *)
            echo "Unknown command. Use 1-4 or q to quit."
            ;;
    esac
    echo ""
done
