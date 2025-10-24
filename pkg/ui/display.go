package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ANSI color codes
const (
	colorReset   = "\033[0m"
	colorCyan    = "\033[36m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorBold    = "\033[1m"
	colorDim     = "\033[2m"
)

// ClearScreen clears the terminal
func ClearScreen() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
}

// DisplayBanner shows the ngrokd startup banner
func DisplayBanner() {
	ClearScreen()
	
	fmt.Println()
	fmt.Printf("%s%s", colorCyan, colorBold)
	fmt.Println("╔═══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                        ngrokd v0.1.0                          ║")
	fmt.Println("║              ngrok Forward Proxy Agent                        ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════╝")
	fmt.Printf("%s", colorReset)
	fmt.Println()
}

// EndpointInfo holds display info for an endpoint
type EndpointInfo struct {
	Name      string
	LocalURL  string
	TargetURL string
}

// DisplayEndpoints shows the forwarding configuration in a nice table
func DisplayEndpoints(endpoints []EndpointInfo, healthURL string) {
	// Session info
	fmt.Printf("%s%-30s%s%s%s\n", colorDim, "Session Status", colorReset, colorGreen, "online")
	fmt.Printf("%s%-30s%s%s%s\n", colorDim, "Account", colorReset, colorYellow, "(using mTLS certificate)")
	fmt.Printf("%s%-30s%s%s\n", colorDim, "Version", colorReset, "0.1.0")
	fmt.Println()
	
	// Forwarding header
	fmt.Printf("%s%sForwarding%s\n", colorBold, colorCyan, colorReset)
	fmt.Println()
	
	maxNameLen := 0
	maxLocalLen := 0
	for _, ep := range endpoints {
		if len(ep.Name) > maxNameLen {
			maxNameLen = len(ep.Name)
		}
		if len(ep.LocalURL) > maxLocalLen {
			maxLocalLen = len(ep.LocalURL)
		}
	}

	for _, ep := range endpoints {
		namePad := strings.Repeat(" ", maxNameLen-len(ep.Name))
		localPad := strings.Repeat(" ", maxLocalLen-len(ep.LocalURL))
		
		fmt.Printf("  %s%s%s%s  %s%s%s%s  %s→%s  %s%s%s\n",
			colorBold, ep.Name, colorReset, namePad,
			colorGreen, ep.LocalURL, colorReset, localPad,
			colorDim, colorReset,
			colorBlue, ep.TargetURL, colorReset)
	}

	fmt.Println()
	
	// Web interface
	fmt.Printf("%s%-30s%s%s%s\n", colorDim, "Web Interface", colorReset, colorMagenta, healthURL)
	fmt.Println()
	
	// Footer
	fmt.Printf("%s%s%s\n", colorDim, strings.Repeat("─", 65), colorReset)
	fmt.Println()
	fmt.Printf("%s%sPress Ctrl+C to quit%s\n", colorDim, colorBold, colorReset)
	fmt.Println()
}

// DisplayShutdown shows the shutdown message
func DisplayShutdown() {
	fmt.Println()
	fmt.Println("Shutting down...")
	fmt.Println()
}
