package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"text/tabwriter"
)

const (
	defaultSocketPath = "/var/run/ngrokd.sock"
	healthEndpoint    = "http://127.0.0.1:8081"
)

type Command struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

type Response struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
}

type StatusData struct {
	Registered      bool   `json:"registered"`
	OperatorID      string `json:"operator_id"`
	EndpointCount   int    `json:"endpoint_count"`
	IngressEndpoint string `json:"ingress_endpoint"`
}

type EndpointInfo struct {
	ID            string `json:"id"`
	Hostname      string `json:"hostname"`
	IP            string `json:"ip"`
	Port          int    `json:"port"`
	URL           string `json:"url"`
	LocalListener bool   `json:"local_listener"`
	NetworkPort   int    `json:"network_port"`
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "status":
		cmdStatus()
	case "list":
		cmdList()
	case "health":
		cmdHealth()
	case "set-api-key":
		if len(os.Args) < 3 {
			fmt.Println("Error: API key required")
			fmt.Println("Usage: ngrokctl set-api-key <KEY>")
			os.Exit(1)
		}
		cmdSetAPIKey(os.Args[2])
	case "config":
		cmdConfig()
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("ngrokctl - Control CLI for ngrokd daemon")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  ngrokctl <command> [args]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  status              Show daemon status")
	fmt.Println("  list                List discovered bound endpoints")
	fmt.Println("  health              Check daemon health")
	fmt.Println("  set-api-key <KEY>   Set ngrok API key")
	fmt.Println("  config              Open config file in editor")
	fmt.Println("  help                Show this help message")
	fmt.Println()
	fmt.Println("Environment:")
	fmt.Println("  NGROKD_SOCKET       Unix socket path (default: /var/run/ngrokd.sock)")
}

func getSocketPath() string {
	if path := os.Getenv("NGROKD_SOCKET"); path != "" {
		return path
	}
	return defaultSocketPath
}

func sendCommand(cmd Command) (*Response, error) {
	socketPath := getSocketPath()
	
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon: %w\nIs ngrokd running?", err)
	}
	defer conn.Close()

	// Send command
	if err := json.NewEncoder(conn).Encode(cmd); err != nil {
		return nil, fmt.Errorf("failed to send command: %w", err)
	}

	// Receive response
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return &resp, nil
}

func cmdStatus() {
	resp, err := sendCommand(Command{Command: "status"})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if !resp.Success {
		fmt.Printf("Error: %s\n", resp.Error)
		os.Exit(1)
	}

	var status StatusData
	if err := json.Unmarshal(resp.Data, &status); err != nil {
		fmt.Printf("Error parsing response: %v\n", err)
		os.Exit(1)
	}

	// Print formatted status
	fmt.Println("╔═══════════════════════════════════════════════════════╗")
	fmt.Println("║               ngrokd Daemon Status                    ║")
	fmt.Println("╚═══════════════════════════════════════════════════════╝")
	fmt.Println()
	
	if status.Registered {
		fmt.Printf("  ✓ Registered:        %s\n", "Yes")
		fmt.Printf("  Operator ID:         %s\n", status.OperatorID)
	} else {
		fmt.Printf("  ⚠ Registered:        %s\n", "No (waiting for API key)")
	}
	
	fmt.Printf("  Endpoints:           %d\n", status.EndpointCount)
	fmt.Printf("  Ingress:             %s\n", status.IngressEndpoint)
	fmt.Println()
	
	if status.EndpointCount == 0 {
		fmt.Println("  No endpoints discovered yet.")
		fmt.Println("  Run 'ngrokctl list' to see available endpoints.")
	}
}

func cmdList() {
	resp, err := sendCommand(Command{Command: "list"})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if !resp.Success {
		fmt.Printf("Error: %s\n", resp.Error)
		os.Exit(1)
	}

	var endpoints []EndpointInfo
	if err := json.Unmarshal(resp.Data, &endpoints); err != nil {
		fmt.Printf("Error parsing response: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("╔═══════════════════════════════════════════════════════╗")
	fmt.Println("║            Discovered Bound Endpoints                 ║")
	fmt.Println("╚═══════════════════════════════════════════════════════╝")
	fmt.Println()

	if len(endpoints) == 0 {
		fmt.Println("  No endpoints discovered.")
		fmt.Println()
		fmt.Println("  Endpoints are discovered automatically every 30s.")
		fmt.Println("  Make sure you have bound endpoints created in ngrok.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  URL\tIP:PORT\tNETWORK\tSTATUS")
	fmt.Fprintln(w, "  ---\t-------\t-------\t------")
	
	for _, ep := range endpoints {
		// Determine status
		status := "✓ Full"
		if !ep.LocalListener && ep.NetworkPort > 0 {
			status = "⚠ Network Only"
		} else if !ep.LocalListener && ep.NetworkPort == 0 {
			status = "❌ Unavailable"
		}
		
		// Network port display
		networkPort := "-"
		if ep.NetworkPort > 0 {
			networkPort = fmt.Sprintf(":%d", ep.NetworkPort)
		}
		
		fmt.Fprintf(w, "  %s\t%s:%d\t%s\t%s\n", 
			ep.URL, ep.IP, ep.Port, networkPort, status)
	}
	w.Flush()
	
	fmt.Println()
	fmt.Printf("  Total: %d endpoint(s)\n", len(endpoints))
	fmt.Println()
}

func cmdHealth() {
	// Check health endpoint
	resp, err := http.Get(healthEndpoint + "/status")
	if err != nil {
		fmt.Printf("Error: Failed to connect to health endpoint: %v\n", err)
		fmt.Println("\nIs ngrokd running?")
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("╔═══════════════════════════════════════════════════════╗")
	fmt.Println("║                 Daemon Health                         ║")
	fmt.Println("╚═══════════════════════════════════════════════════════╝")
	fmt.Println()

	// Pretty print JSON
	var prettyJSON map[string]interface{}
	if err := json.Unmarshal(body, &prettyJSON); err == nil {
		prettyBytes, _ := json.MarshalIndent(prettyJSON, "  ", "  ")
		fmt.Println(string(prettyBytes))
	} else {
		fmt.Println(string(body))
	}
	fmt.Println()
}

func cmdSetAPIKey(apiKey string) {
	resp, err := sendCommand(Command{
		Command: "set-api-key",
		Args:    []string{apiKey},
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if !resp.Success {
		fmt.Printf("Error: %s\n", resp.Error)
		os.Exit(1)
	}

	fmt.Println("✓ API key set successfully")
	fmt.Println()
	fmt.Println("The daemon will now:")
	fmt.Println("  1. Register with ngrok API")
	fmt.Println("  2. Provision mTLS certificates")
	fmt.Println("  3. Start polling for bound endpoints")
	fmt.Println()
	fmt.Println("Run 'ngrokctl status' to check registration status")
}

func cmdConfig() {
	configPath := "/etc/ngrokd/config.yml"
	
	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Printf("Config file not found: %s\n", configPath)
		os.Exit(1)
	}
	
	// Get editor from environment or use default
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = "vim" // Default to vim
	}
	
	// Execute editor
	fmt.Printf("Opening %s with %s...\n", configPath, editor)
	cmd := exec.Command("sudo", editor, configPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		fmt.Printf("Error opening config: %v\n", err)
		os.Exit(1)
	}
}
