#!/bin/bash
set -e

# ngrokd installer

INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
BINARY_NAME="ngrokd"

echo "Installing ngrokd to $INSTALL_DIR..."

# Build the binary
echo "Building ngrokd..."
go build -o "$BINARY_NAME" ./cmd/ngrokd

# Check if install directory exists
if [ ! -d "$INSTALL_DIR" ]; then
    echo "Error: $INSTALL_DIR does not exist"
    exit 1
fi

# Check if we need sudo
if [ -w "$INSTALL_DIR" ]; then
    cp "$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
    chmod +x "$INSTALL_DIR/$BINARY_NAME"
else
    echo "Installing to $INSTALL_DIR requires sudo..."
    sudo cp "$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
    sudo chmod +x "$INSTALL_DIR/$BINARY_NAME"
fi

echo "✓ ngrokd installed to $INSTALL_DIR/$BINARY_NAME"
echo ""
echo "Usage:"
echo "  ngrokd connect --all                 # Connect to all endpoints"
echo "  ngrokd list                          # List available endpoints"
echo "  ngrokd connect --endpoint-uri=...    # Connect to specific endpoint"
echo ""
echo "For more info: ngrokd help"
