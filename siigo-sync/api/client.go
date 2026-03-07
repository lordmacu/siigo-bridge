package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// Client is the HTTP client for the Finearom API
type Client struct {
	baseURL    string
	email      string
	password   string
	token      string
	httpClient *http.Client
}

// NewClient creates a new Finearom API client
func NewClient(baseURL, email, password string) *Client {
	return &Client{
		baseURL:  baseURL,
		email:    email,
		password: password,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Login authenticates with the Finearom API and stores the token
func (c *Client) Login() error {
	body := map[string]string{
		"email":    c.email,
		"password": c.password,
	}

	resp, err := c.doRequest("POST", "/siigo/login", body, false)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("login response parse error: %w", err)
	}

	// Laravel Sanctum returns token in different formats
	if token, ok := result["token"].(string); ok {
		c.token = token
	} else if data, ok := result["data"].(map[string]interface{}); ok {
		if token, ok := data["token"].(string); ok {
			c.token = token
		}
	} else if token, ok := result["access_token"].(string); ok {
		c.token = token
	}

	if c.token == "" {
		return fmt.Errorf("no token in login response: %s", string(resp))
	}

	log.Printf("[api] Logged in successfully")
	return nil
}

// SyncClient sends a client record to Finearom
func (c *Client) SyncClient(data map[string]interface{}) error {
	_, err := c.doRequest("POST", "/siigo/clients", data, true)
	if err != nil {
		return fmt.Errorf("sync client failed: %w", err)
	}
	return nil
}

// SyncProduct sends a product record to Finearom
func (c *Client) SyncProduct(data map[string]interface{}) error {
	_, err := c.doRequest("POST", "/siigo/products", data, true)
	if err != nil {
		return fmt.Errorf("sync product failed: %w", err)
	}
	return nil
}

// SyncMovement sends a movement/transaction to Finearom
func (c *Client) SyncMovement(data map[string]interface{}) error {
	_, err := c.doRequest("POST", "/siigo/movements", data, true)
	if err != nil {
		return fmt.Errorf("sync movement failed: %w", err)
	}
	return nil
}

// SyncCartera sends a cartera/portfolio entry to Finearom
func (c *Client) SyncCartera(data map[string]interface{}) error {
	_, err := c.doRequest("POST", "/siigo/cartera", data, true)
	if err != nil {
		return fmt.Errorf("sync cartera failed: %w", err)
	}
	return nil
}

func (c *Client) doRequest(method, endpoint string, body interface{}, auth bool) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	url := c.baseURL + endpoint
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if auth && c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP %s %s: %w", method, endpoint, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d from %s %s: %s", resp.StatusCode, method, endpoint, string(respBody))
	}

	return respBody, nil
}

// IsAuthenticated returns true if the client has a valid token
func (c *Client) IsAuthenticated() bool {
	return c.token != ""
}
