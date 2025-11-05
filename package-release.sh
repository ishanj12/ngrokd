#!/bin/bash
# Package release binaries with install script and README

set -e

VERSION="v0.2.0"
DIST_DIR="dist"
PACKAGES_DIR="dist/packages"

echo "Packaging ngrokd $VERSION releases..."
echo ""

# Create packages directory
mkdir -p "$PACKAGES_DIR"

# Function to create package
create_package() {
    local platform=$1
    local arch=$2
    local pkg_name="ngrokd-${VERSION}-${platform}-${arch}"
    local pkg_dir="$PACKAGES_DIR/$pkg_name"
    
    echo "Creating package: $pkg_name"
    
    # Create package directory
    mkdir -p "$pkg_dir"
    
    # Copy binaries (platform-specific)
    if [ "$platform" = "windows" ]; then
        cp "$DIST_DIR/ngrokd-${platform}-${arch}.exe" "$pkg_dir/ngrokd.exe"
        cp "$DIST_DIR/ngrokctl-${platform}-${arch}.exe" "$pkg_dir/ngrokctl.exe"
        
        # Copy Windows install/uninstall scripts
        cp install.ps1 "$pkg_dir/install.ps1"
        cp uninstall.ps1 "$pkg_dir/uninstall.ps1"
    else
        cp "$DIST_DIR/ngrokd-${platform}-${arch}" "$pkg_dir/ngrokd"
        cp "$DIST_DIR/ngrokctl-${platform}-${arch}" "$pkg_dir/ngrokctl"
        chmod +x "$pkg_dir/ngrokd" "$pkg_dir/ngrokctl"
        
        # Copy Unix install/uninstall scripts
        cp install.sh "$pkg_dir/install.sh"
        chmod +x "$pkg_dir/install.sh"
        cp uninstall.sh "$pkg_dir/uninstall.sh"
        chmod +x "$pkg_dir/uninstall.sh"
    fi
    
    # Create README
    cat > "$pkg_dir/README.txt" << 'EOF'
ngrokd - Forward Proxy Daemon for Kubernetes Bound Endpoints
==============================================================

Version: v0.2.0
Platform: ${PLATFORM}-${ARCH}

Quick Install
-------------

Option 1: Using install script (recommended)
    sudo ./install.sh

Option 2: Manual installation
    chmod +x ngrokd ngrokctl
    sudo mv ngrokd /usr/local/bin/
    sudo mv ngrokctl /usr/local/bin/
    
    # Create config directory
    sudo mkdir -p /etc/ngrokd
    
    # Start daemon
    sudo ngrokd --config=/etc/ngrokd/config.yml &
    
    # Set API key
    ngrokctl set-api-key YOUR_NGROK_API_KEY

Quick Start
-----------

1. Install (see above)

2. Configure (if not using install script):
   sudo mkdir -p /etc/ngrokd
   # Create config.yml - see https://github.com/ishanj12/ngrokd

3. Start daemon:
   sudo nohup ngrokd --config=/etc/ngrokd/config.yml > ~/ngrokd.log 2>&1 &

4. Set API key:
   ngrokctl set-api-key YOUR_NGROK_API_KEY

5. Check status:
   ngrokctl status
   ngrokctl list

Uninstall
---------
    sudo ./uninstall.sh

Documentation
-------------
- GitHub: https://github.com/ishanj12/ngrokd
- README: https://github.com/ishanj12/ngrokd/blob/main/README.md
- macOS Guide: https://github.com/ishanj12/ngrokd/blob/main/MACOS.md
- Linux Guide: https://github.com/ishanj12/ngrokd/blob/main/LINUX.md

Requirements
------------
- ngrok API key (get from https://dashboard.ngrok.com/api)
- Bound endpoints created in ngrok
- sudo/root access
- Linux or macOS

Support
-------
Issues: https://github.com/ishanj12/ngrokd/issues
EOF
    
    # Replace placeholders in README
    sed -i.bak "s/\${PLATFORM}/${platform}/g" "$pkg_dir/README.txt"
    sed -i.bak "s/\${ARCH}/${arch}/g" "$pkg_dir/README.txt"
    rm "$pkg_dir/README.txt.bak"
    
    # Create tarball
    cd "$PACKAGES_DIR"
    tar czf "${pkg_name}.tar.gz" "$pkg_name"
    
    # Create checksum
    if command -v sha256sum &> /dev/null; then
        sha256sum "${pkg_name}.tar.gz" >> checksums.txt
    else
        shasum -a 256 "${pkg_name}.tar.gz" >> checksums.txt
    fi
    
    # Clean up directory (keep tarball)
    rm -rf "$pkg_name"
    
    cd - > /dev/null
    echo "✓ Created ${pkg_name}.tar.gz"
}

# Clean old packages
rm -f "$PACKAGES_DIR"/*.tar.gz "$PACKAGES_DIR/checksums.txt"

# Create packages for each platform
create_package "linux" "amd64"
create_package "linux" "arm64"
create_package "darwin" "amd64"
create_package "darwin" "arm64"
create_package "windows" "amd64"
create_package "windows" "arm64"

# Show results
echo ""
echo "✓ Packaging complete!"
echo ""
echo "Packages created in $PACKAGES_DIR/:"
ls -lh "$PACKAGES_DIR"/*.tar.gz
echo ""
echo "Checksums:"
cat "$PACKAGES_DIR/checksums.txt"
echo ""
echo "Upload to GitHub release:"
echo "  gh release create $VERSION dist/packages/*.tar.gz dist/packages/checksums.txt"
