#!/bin/bash
# Build release binaries for all platforms

set -e

VERSION="v0.2.0"
REPO="ishanj12/ngrokd"

echo "Building ngrokd $VERSION for multiple platforms..."
echo ""

# Create dist directory
mkdir -p dist

# Build for Linux AMD64
echo "Building Linux AMD64..."
GOOS=linux GOARCH=amd64 go build -o dist/ngrokd-linux-amd64 ./cmd/ngrokd
GOOS=linux GOARCH=amd64 go build -o dist/ngrokctl-linux-amd64 ./cmd/ngrokctl

# Build for Linux ARM64
echo "Building Linux ARM64..."
GOOS=linux GOARCH=arm64 go build -o dist/ngrokd-linux-arm64 ./cmd/ngrokd
GOOS=linux GOARCH=arm64 go build -o dist/ngrokctl-linux-arm64 ./cmd/ngrokctl

# Build for macOS AMD64 (Intel)
echo "Building macOS AMD64..."
GOOS=darwin GOARCH=amd64 go build -o dist/ngrokd-darwin-amd64 ./cmd/ngrokd
GOOS=darwin GOARCH=amd64 go build -o dist/ngrokctl-darwin-amd64 ./cmd/ngrokctl

# Build for macOS ARM64 (Apple Silicon)
echo "Building macOS ARM64..."
GOOS=darwin GOARCH=arm64 go build -o dist/ngrokd-darwin-arm64 ./cmd/ngrokd
GOOS=darwin GOARCH=arm64 go build -o dist/ngrokctl-darwin-arm64 ./cmd/ngrokctl

# Build for Windows AMD64
echo "Building Windows AMD64..."
GOOS=windows GOARCH=amd64 go build -o dist/ngrokd-windows-amd64.exe ./cmd/ngrokd
GOOS=windows GOARCH=amd64 go build -o dist/ngrokctl-windows-amd64.exe ./cmd/ngrokctl

# Build for Windows ARM64
echo "Building Windows ARM64..."
GOOS=windows GOARCH=arm64 go build -o dist/ngrokd-windows-arm64.exe ./cmd/ngrokd
GOOS=windows GOARCH=arm64 go build -o dist/ngrokctl-windows-arm64.exe ./cmd/ngrokctl

# Make all executable
chmod +x dist/*

# Create checksums
echo ""
echo "Creating checksums..."
cd dist
if command -v sha256sum &> /dev/null; then
    sha256sum ngrokd-* ngrokctl-* > checksums.txt
else
    shasum -a 256 ngrokd-* ngrokctl-* > checksums.txt
fi
cd ..

# Show results
echo ""
echo "✓ Build complete!"
echo ""
echo "Binaries created in dist/:"
ls -lh dist/
echo ""
echo "Upload these to GitHub release $VERSION"
