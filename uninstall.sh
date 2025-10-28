#!/bin/bash
# Uninstall ngrokd completely

set -e

echo "Uninstalling ngrokd..."

# 1. Stop the daemon
echo "→ Stopping ngrokd daemon..."
sudo pkill -9 ngrokd 2>/dev/null || echo "  No ngrokd process running"

# 2. Remove binaries
echo "→ Removing binaries..."
sudo rm -f /usr/local/bin/ngrokd
sudo rm -f /usr/local/bin/ngrokctl

# 3. Remove configuration and data
echo "→ Removing configuration and data..."
sudo rm -rf /etc/ngrokd

# 4. Remove socket
echo "→ Removing socket..."
sudo rm -f /var/run/ngrokd.sock

# 5. Clean up /etc/hosts
echo "→ Cleaning /etc/hosts..."
if grep -q "BEGIN ngrokd" /etc/hosts; then
  sudo sed -i.bak '/# BEGIN ngrokd managed section/,/# END ngrokd managed section/d' /etc/hosts
  echo "  Cleaned ngrokd entries from /etc/hosts"
else
  echo "  No ngrokd entries found in /etc/hosts"
fi

# 6. Remove loopback aliases (macOS)
if [[ "$OSTYPE" == "darwin"* ]]; then
  echo "→ Removing loopback aliases..."
  for i in {2..254}; do
    sudo ifconfig lo0 -alias 127.0.0.$i 2>/dev/null || true
  done
  echo "  Removed loopback aliases"
fi

# 7. Remove dummy interface (Linux)
if [[ "$OSTYPE" == "linux-gnu"* ]]; then
  echo "→ Removing network interface..."
  sudo ip link delete ngrokd0 2>/dev/null || echo "  No ngrokd0 interface found"
fi

echo ""
echo "✓ Uninstall complete!"
echo ""
echo "Remaining files to check manually:"
echo "  - Build artifacts in: $(pwd)/dist/"
echo "  - Local binaries in: $(pwd)/"
