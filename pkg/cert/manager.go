package cert

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/ngrokapi"
)

// Manager handles certificate lifecycle
type Manager struct {
	provisioner *Provisioner
	apiClient   *ngrokapi.Client
	logger      logr.Logger
	operatorID  string
}

// Config holds the configuration for the certificate manager
type Config struct {
	CertDir     string
	APIKey      string
	Description string
	Metadata    string
	Region      string
	Logger      logr.Logger
}

// NewManager creates a new certificate manager
func NewManager(config Config) *Manager {
	if config.CertDir == "" {
		config.CertDir = filepath.Join(os.Getenv("HOME"), ".ngrok-forward-proxy", "certs")
	}

	return &Manager{
		provisioner: NewProvisioner(config.CertDir),
		apiClient:   ngrokapi.NewClient(config.APIKey),
		logger:      config.Logger,
	}
}

// EnsureCertificate ensures a valid certificate exists, provisioning one if necessary
func (m *Manager) EnsureCertificate(ctx context.Context, config Config) (tls.Certificate, error) {
	// Check if certificate already exists on disk
	if m.provisioner.CertificateExists() {
		m.logger.Info("Loading existing certificate",
			"keyPath", m.provisioner.GetKeyPath(),
			"certPath", m.provisioner.GetCertPath())

		cert, err := m.provisioner.LoadCertificate()
		if err != nil {
			m.logger.Info("Failed to load existing certificate, will provision new one", "error", err)
		} else {
			// Load operator ID from file if it exists
			m.loadOperatorID()
			return cert, nil
		}
	}

	// Provision new certificate
	m.logger.Info("Provisioning new certificate via ngrok API")

	return m.provisionCertificate(ctx, config)
}

// provisionCertificate provisions a new certificate via ngrok API
func (m *Manager) provisionCertificate(ctx context.Context, config Config) (tls.Certificate, error) {
	// Step 1: Generate private key and CSR
	m.logger.Info("Generating private key and CSR")

	privateKeyPEM, csrPEM, err := m.provisioner.GenerateKeyAndCSR()
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate key and CSR: %w", err)
	}

	// Step 2: Register with ngrok API
	m.logger.Info("Registering with ngrok API")

	description := config.Description
	if description == "" {
		description = "ngrok forward proxy agent"
	}

	metadata := config.Metadata
	if metadata == "" {
		metadata = `{"type":"forward-proxy"}`
	}

	region := config.Region
	if region == "" {
		region = "global"
	}

	createReq := &ngrokapi.KubernetesOperatorCreate{
		Description:     description,
		Metadata:        metadata,
		EnabledFeatures: []string{"bindings"},
		Region:          region,
		Binding: &ngrokapi.KubernetesOperatorBindingCreate{
			CSR: string(csrPEM),
		},
	}

	operator, err := m.apiClient.CreateKubernetesOperator(ctx, createReq)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to register with ngrok API: %w", err)
	}

	m.operatorID = operator.ID
	
	ingressEp := "not provided"
	if operator.Binding != nil && operator.Binding.IngressEndpoint != "" {
		ingressEp = operator.Binding.IngressEndpoint
	}
	
	m.logger.Info("Successfully registered with ngrok",
		"operatorID", operator.ID,
		"ingressEndpoint", ingressEp,
		"region", operator.Region)

	// Step 3: Extract certificate from response
	if operator.Binding == nil || operator.Binding.Cert.Cert == "" {
		return tls.Certificate{}, fmt.Errorf("no certificate returned in API response")
	}

	certPEM := []byte(operator.Binding.Cert.Cert)

	m.logger.Info("Received certificate from ngrok",
		"notBefore", operator.Binding.Cert.NotBefore,
		"notAfter", operator.Binding.Cert.NotAfter)

	// Step 4: Save certificate and private key to disk
	if err := m.provisioner.SaveCertificate(privateKeyPEM, certPEM); err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to save certificate: %w", err)
	}

	m.logger.Info("Certificate saved",
		"keyPath", m.provisioner.GetKeyPath(),
		"certPath", m.provisioner.GetCertPath())

	// Save operator ID to file
	if err := m.saveOperatorID(); err != nil {
		m.logger.Info("Failed to save operator ID", "error", err)
	}

	// Step 5: Load and return certificate
	cert, err := tls.X509KeyPair(certPEM, privateKeyPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return cert, nil
}

func (m *Manager) saveOperatorID() error {
	idPath := filepath.Join(m.provisioner.certDir, "operator_id")
	return os.WriteFile(idPath, []byte(m.operatorID), 0644)
}

func (m *Manager) loadOperatorID() {
	idPath := filepath.Join(m.provisioner.certDir, "operator_id")
	data, err := os.ReadFile(idPath)
	if err == nil {
		m.operatorID = string(data)
	}
}

// GetOperatorID returns the operator ID (if registered)
func (m *Manager) GetOperatorID() string {
	return m.operatorID
}

// GetIngressEndpoint returns the ingress endpoint for the operator
func (m *Manager) GetIngressEndpoint(ctx context.Context) (string, error) {
	if m.operatorID == "" {
		return "", fmt.Errorf("operator not registered")
	}

	operator, err := m.apiClient.GetKubernetesOperator(ctx, m.operatorID)
	if err != nil {
		return "", fmt.Errorf("failed to get operator: %w", err)
	}

	if operator.Binding != nil && operator.Binding.IngressEndpoint != "" {
		return operator.Binding.IngressEndpoint, nil
	}

	return "kubernetes-binding-ingress.ngrok.io:443", nil
}
