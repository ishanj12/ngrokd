package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the agent configuration
type Config struct {
	// Agent configuration
	Agent AgentConfig `yaml:"agent"`

	// Endpoints to forward
	Endpoints []EndpointConfig `yaml:"endpoints"`

	// Health check configuration
	Health HealthConfig `yaml:"health,omitempty"`

	// Logging configuration
	Logging LoggingConfig `yaml:"logging,omitempty"`
}

// AgentConfig holds agent-level settings
type AgentConfig struct {
	// Description for operator registration
	Description string `yaml:"description,omitempty"`

	// Region for ngrok (us, eu, ap, au, sa, jp, in, global)
	Region string `yaml:"region,omitempty"`

	// API key for ngrok (can also use NGROK_API_KEY env var)
	APIKey string `yaml:"api_key,omitempty"`

	// Certificate directory for storing/loading certs
	CertDir string `yaml:"cert_dir,omitempty"`

	// Manual certificate files (alternative to auto-provisioning)
	CertFile string `yaml:"cert_file,omitempty"`
	KeyFile  string `yaml:"key_file,omitempty"`

	// Ingress endpoint (usually auto-detected)
	IngressEndpoint string `yaml:"ingress_endpoint,omitempty"`
}

// EndpointConfig defines a single endpoint to forward
type EndpointConfig struct {
	// Name for this endpoint (for logging/identification)
	Name string `yaml:"name"`

	// URI of the bound endpoint (e.g., https://my-app.ngrok.app)
	URI string `yaml:"uri"`

	// Port on the bound endpoint (default: 443)
	Port int `yaml:"port,omitempty"`

	// Local port to listen on
	LocalPort int `yaml:"local_port"`

	// Local address to bind (default: 127.0.0.1)
	LocalAddress string `yaml:"local_address,omitempty"`

	// Enabled flag (allows disabling without removing from config)
	Enabled *bool `yaml:"enabled,omitempty"`
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	// Level: info, debug, error
	Level string `yaml:"level,omitempty"`

	// Format: text, json
	Format string `yaml:"format,omitempty"`

	// Verbose enables verbose logging
	Verbose bool `yaml:"verbose,omitempty"`
}

// HealthConfig holds health check settings
type HealthConfig struct {
	// Enabled controls whether health check server runs
	Enabled bool `yaml:"enabled,omitempty"`

	// Port for health check server (default: 8081)
	Port int `yaml:"port,omitempty"`

	// Address to bind (default: 127.0.0.1)
	Address string `yaml:"address,omitempty"`
}

// LoadFromFile loads configuration from a YAML file
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults
	cfg.setDefaults()

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets default values for unspecified fields
func (c *Config) setDefaults() {
	// Agent defaults
	if c.Agent.Description == "" {
		c.Agent.Description = "ngrok forward proxy agent"
	}
	if c.Agent.Region == "" {
		c.Agent.Region = "global"
	}
	if c.Agent.CertDir == "" {
		c.Agent.CertDir = filepath.Join(os.Getenv("HOME"), ".ngrok-forward-proxy", "certs")
	}

	// Endpoint defaults
	for i := range c.Endpoints {
		ep := &c.Endpoints[i]
		
		if ep.Port == 0 {
			ep.Port = 443
		}
		if ep.LocalAddress == "" {
			ep.LocalAddress = "127.0.0.1"
		}
		if ep.Enabled == nil {
			enabled := true
			ep.Enabled = &enabled
		}
	}

	// Health check defaults
	if c.Health.Port == 0 {
		c.Health.Port = 8081
	}
	if c.Health.Address == "" {
		c.Health.Address = "127.0.0.1"
	}

	// Logging defaults
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Logging.Format == "" {
		c.Logging.Format = "text"
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate region
	validRegions := map[string]bool{
		"us": true, "eu": true, "ap": true, "au": true,
		"sa": true, "jp": true, "in": true, "global": true,
	}
	if !validRegions[c.Agent.Region] {
		return fmt.Errorf("invalid region: %s (must be one of: us, eu, ap, au, sa, jp, in, global)", c.Agent.Region)
	}

	// Validate endpoints
	if len(c.Endpoints) == 0 {
		return fmt.Errorf("at least one endpoint must be configured")
	}

	for i, ep := range c.Endpoints {
		if ep.Name == "" {
			return fmt.Errorf("endpoint[%d]: name is required", i)
		}
		if ep.URI == "" {
			return fmt.Errorf("endpoint[%d] (%s): uri is required", i, ep.Name)
		}
		if ep.LocalPort <= 0 || ep.LocalPort > 65535 {
			return fmt.Errorf("endpoint[%d] (%s): invalid local_port %d", i, ep.Name, ep.LocalPort)
		}
		if ep.Port <= 0 || ep.Port > 65535 {
			return fmt.Errorf("endpoint[%d] (%s): invalid port %d", i, ep.Name, ep.Port)
		}
	}

	// Validate logging
	validLevels := map[string]bool{"info": true, "debug": true, "error": true}
	if !validLevels[c.Logging.Level] {
		return fmt.Errorf("invalid logging level: %s (must be one of: info, debug, error)", c.Logging.Level)
	}

	validFormats := map[string]bool{"text": true, "json": true}
	if !validFormats[c.Logging.Format] {
		return fmt.Errorf("invalid logging format: %s (must be one of: text, json)", c.Logging.Format)
	}

	return nil
}

// GetEnabledEndpoints returns only the enabled endpoints
func (c *Config) GetEnabledEndpoints() []EndpointConfig {
	var enabled []EndpointConfig
	for _, ep := range c.Endpoints {
		if ep.Enabled != nil && *ep.Enabled {
			enabled = append(enabled, ep)
		}
	}
	return enabled
}

// MergeWithFlags merges CLI flags with config file (flags take precedence)
func (c *Config) MergeWithFlags(flags map[string]interface{}) {
	// Agent settings
	if apiKey, ok := flags["api-key"].(string); ok && apiKey != "" {
		c.Agent.APIKey = apiKey
	}
	if certFile, ok := flags["cert"].(string); ok && certFile != "" {
		c.Agent.CertFile = certFile
	}
	if keyFile, ok := flags["key"].(string); ok && keyFile != "" {
		c.Agent.KeyFile = keyFile
	}
	if certDir, ok := flags["cert-dir"].(string); ok && certDir != "" {
		c.Agent.CertDir = certDir
	}
	if description, ok := flags["description"].(string); ok && description != "" {
		c.Agent.Description = description
	}
	if region, ok := flags["region"].(string); ok && region != "" {
		c.Agent.Region = region
	}
	if ingress, ok := flags["ingress"].(string); ok && ingress != "" {
		c.Agent.IngressEndpoint = ingress
	}

	// Logging settings
	if verbose, ok := flags["v"].(bool); ok && verbose {
		c.Logging.Verbose = true
	}
}
