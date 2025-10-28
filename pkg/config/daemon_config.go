package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// DaemonConfig represents the ngrokd daemon configuration
type DaemonConfig struct {
	API             APIConfig             `yaml:"api"`
	IngressEndpoint string                `yaml:"ingressEndpoint,omitempty"`
	Server          ServerConfig          `yaml:"server"`
	BoundEndpoints  BoundEndpointsConfig  `yaml:"bound_endpoints"`
	Net             NetConfig             `yaml:"net"`
}

// APIConfig holds ngrok API settings
type APIConfig struct {
	URL string `yaml:"url,omitempty"`
	Key string `yaml:"key,omitempty"`
}

// ServerConfig holds server settings
type ServerConfig struct {
	LogLevel   string `yaml:"log_level,omitempty"`
	SocketPath string `yaml:"socket_path,omitempty"`
	ClientCert string `yaml:"client_cert,omitempty"`
	ClientKey  string `yaml:"client_key,omitempty"`
}

// BoundEndpointsConfig holds bound endpoint settings
type BoundEndpointsConfig struct {
	PollInterval int      `yaml:"poll_interval,omitempty"`
	Selectors    []string `yaml:"selectors,omitempty"`
}

// NetConfig holds network interface settings
type NetConfig struct {
	InterfaceName   string            `yaml:"interface_name,omitempty"`
	Subnet          string            `yaml:"subnet,omitempty"`
	ListenInterface string            `yaml:"listen_interface,omitempty"` // Default: "virtual", "0.0.0.0", or specific IP
	StartPort       int               `yaml:"start_port,omitempty"`       // Starting port for non-virtual modes
	Overrides       map[string]string `yaml:"overrides,omitempty"`        // hostname -> listen_interface override
}

// LoadDaemonConfig loads daemon configuration from file
func LoadDaemonConfig(path string) (*DaemonConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg DaemonConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults
	cfg.setDefaults()

	return &cfg, nil
}

// setDefaults sets default values
func (c *DaemonConfig) setDefaults() {
	if c.API.URL == "" {
		c.API.URL = "https://api.ngrok.com"
	}
	if c.IngressEndpoint == "" {
		c.IngressEndpoint = "kubernetes-binding-ingress.ngrok.io:443"
	}
	if c.Server.LogLevel == "" {
		c.Server.LogLevel = "info"
	}
	if c.Server.SocketPath == "" {
		c.Server.SocketPath = "/var/run/ngrokd.sock"
	}
	if c.Server.ClientCert == "" {
		c.Server.ClientCert = "/etc/ngrokd/tls.crt"
	}
	if c.Server.ClientKey == "" {
		c.Server.ClientKey = "/etc/ngrokd/tls.key"
	}
	if c.BoundEndpoints.PollInterval == 0 {
		c.BoundEndpoints.PollInterval = 30
	}
	if len(c.BoundEndpoints.Selectors) == 0 {
		c.BoundEndpoints.Selectors = []string{"true"}
	}
	if c.Net.InterfaceName == "" {
		c.Net.InterfaceName = "ngrokd0"
	}
	if c.Net.Subnet == "" {
		c.Net.Subnet = "10.107.0.0/16"
	}
	if c.Net.ListenInterface == "" {
		c.Net.ListenInterface = "virtual"
	}
	if c.Net.StartPort == 0 {
		c.Net.StartPort = 9080
	}
}
