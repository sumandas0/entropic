package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
)

const (
	defaultTimeout = 30 * time.Second
	apiV1BasePath  = "/api/v1"
)

type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	
	// Services
	Entities      *EntityService
	Relations     *RelationService
	Schemas       *SchemaService
	Search        *SearchService
}

type ClientOption func(*Client)

func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = timeout
	}
}

func NewClient(baseURL string, opts ...ClientOption) (*Client, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	client := &Client{
		baseURL: parsedURL,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}

	for _, opt := range opts {
		opt(client)
	}

	// Initialize services
	client.Entities = &EntityService{client: client}
	client.Relations = &RelationService{client: client}
	client.Schemas = &SchemaService{client: client}
	client.Search = &SearchService{client: client}

	return client, nil
}

// HealthCheck checks if the Entropic service is healthy
func (c *Client) HealthCheck(ctx context.Context) error {
	resp, err := c.doRequest(ctx, http.MethodGet, "/health", nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed with status: %d", resp.StatusCode)
	}

	return nil
}

// ReadinessCheck checks if the Entropic service is ready
func (c *Client) ReadinessCheck(ctx context.Context) error {
	resp, err := c.doRequest(ctx, http.MethodGet, "/ready", nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("readiness check failed with status: %d", resp.StatusCode)
	}

	return nil
}

// GetMetrics retrieves service metrics
func (c *Client) GetMetrics(ctx context.Context) (map[string]interface{}, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/metrics", nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var metrics map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		return nil, fmt.Errorf("failed to decode metrics: %w", err)
	}

	return metrics, nil
}

// doRequest performs an HTTP request with proper error handling
func (c *Client) doRequest(ctx context.Context, method, path string, query url.Values, body interface{}) (*http.Response, error) {
	// Build URL
	u := *c.baseURL
	u.Path = path
	if query != nil {
		u.RawQuery = query.Encode()
	}

	// Prepare body
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// doJSONRequest performs a JSON request and decodes the response
func (c *Client) doJSONRequest(ctx context.Context, method, path string, query url.Values, reqBody, respBody interface{}) error {
	resp, err := c.doRequest(ctx, method, path, query, reqBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode >= 400 {
		return c.handleErrorResponse(resp)
	}

	// Decode response if needed
	if respBody != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// handleErrorResponse processes error responses from the API
func (c *Client) handleErrorResponse(resp *http.Response) error {
	var apiErr APIError
	if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
		// If we can't decode the error, return a generic error
		body, _ := io.ReadAll(resp.Body)
		return &APIError{
			Type:    ErrorTypeUnknown,
			Message: string(body),
			Code:    resp.StatusCode,
		}
	}

	// Set the status code
	apiErr.Code = resp.StatusCode

	// Map status codes to error types if not already set
	if apiErr.Type == "" {
		switch resp.StatusCode {
		case http.StatusBadRequest:
			apiErr.Type = ErrorTypeValidation
		case http.StatusNotFound:
			apiErr.Type = ErrorTypeNotFound
		case http.StatusConflict:
			apiErr.Type = ErrorTypeAlreadyExists
		case http.StatusInternalServerError:
			apiErr.Type = ErrorTypeInternal
		default:
			apiErr.Type = ErrorTypeUnknown
		}
	}

	return &apiErr
}

// ParseUUID is a helper function to parse UUID strings
func ParseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}