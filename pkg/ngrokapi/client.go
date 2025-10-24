package ngrokapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultBaseURL = "https://api.ngrok.com"
	apiVersion     = "2"
)

// Client is an ngrok API client
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new ngrok API client
func NewClient(apiKey string) *Client {
	return &Client{
		baseURL: defaultBaseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// KubernetesOperatorCreate represents the request to create a Kubernetes operator
type KubernetesOperatorCreate struct {
	Description     string                            `json:"description,omitempty"`
	Metadata        string                            `json:"metadata,omitempty"`
	EnabledFeatures []string                          `json:"enabled_features,omitempty"`
	Region          string                            `json:"region,omitempty"`
	Binding         *KubernetesOperatorBindingCreate  `json:"binding,omitempty"`
}

// KubernetesOperatorBindingCreate represents the binding configuration
type KubernetesOperatorBindingCreate struct {
	EndpointSelectors []string `json:"endpoint_selectors,omitempty"`
	CSR               string   `json:"csr,omitempty"`
}

// KubernetesOperator represents an ngrok Kubernetes operator
type KubernetesOperator struct {
	ID              string                       `json:"id"`
	URI             string                       `json:"uri"`
	CreatedAt       string                       `json:"created_at"`
	UpdatedAt       string                       `json:"updated_at"`
	Description     string                       `json:"description,omitempty"`
	Metadata        string                       `json:"metadata,omitempty"`
	EnabledFeatures []string                     `json:"enabled_features,omitempty"`
	Region          string                       `json:"region,omitempty"`
	Binding         *KubernetesOperatorBinding   `json:"binding,omitempty"`
}

// KubernetesOperatorBinding represents the binding configuration
type KubernetesOperatorBinding struct {
	EndpointSelectors []string                 `json:"endpoint_selectors,omitempty"`
	Cert              KubernetesOperatorCert   `json:"cert,omitempty"`
	IngressEndpoint   string                   `json:"ingress_endpoint,omitempty"`
}

// KubernetesOperatorCert represents the certificate information
type KubernetesOperatorCert struct {
	Cert      string `json:"cert"`
	NotBefore string `json:"not_before"`
	NotAfter  string `json:"not_after"`
}

// CreateKubernetesOperator creates a new Kubernetes operator and returns the certificate
func (c *Client) CreateKubernetesOperator(ctx context.Context, req *KubernetesOperatorCreate) (*KubernetesOperator, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/kubernetes_operators", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Ngrok-Version", apiVersion)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var operator KubernetesOperator
	if err := json.Unmarshal(bodyBytes, &operator); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &operator, nil
}

// Endpoint represents an ngrok endpoint
type Endpoint struct {
	ID          string `json:"id"`
	URI         string `json:"uri"`
	URL         string `json:"url"`
	Type        string `json:"type"`
	Proto       string `json:"proto"`
	HostnameID  string `json:"hostname_id,omitempty"`
	Hostname    string `json:"hostname,omitempty"`
	Port        int    `json:"port,omitempty"`
	Metadata    string `json:"metadata,omitempty"`
	Description string `json:"description,omitempty"`
	Binding     string `json:"binding,omitempty"`
}

// ListBoundEndpoints lists all endpoints bound to a Kubernetes operator
func (c *Client) ListBoundEndpoints(ctx context.Context, operatorID string) ([]Endpoint, error) {
	if operatorID == "" {
		return nil, fmt.Errorf("operator ID is empty - certificate may not be properly provisioned")
	}

	url := fmt.Sprintf("%s/kubernetes_operators/%s/bound_endpoints", c.baseURL, operatorID)
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Ngrok-Version", apiVersion)
	httpReq.Header.Set("User-Agent", "ngrokd-forward-proxy/0.1.0")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Endpoints []Endpoint `json:"endpoints"`
		URI       string     `json:"uri"`
		NextPage  string     `json:"next_page_uri,omitempty"`
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result.Endpoints, nil
}

// GetKubernetesOperator retrieves a Kubernetes operator by ID
func (c *Client) GetKubernetesOperator(ctx context.Context, id string) (*KubernetesOperator, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/kubernetes_operators/"+id, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Ngrok-Version", apiVersion)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var operator KubernetesOperator
	if err := json.Unmarshal(bodyBytes, &operator); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &operator, nil
}
