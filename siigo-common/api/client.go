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

type Client struct {
	baseURL    string
	email      string
	password   string
	token      string
	httpClient *http.Client
}

// SyncPayload is the universal payload sent to the server
type SyncPayload struct {
	Table  string                 `json:"table"`  // clients, products, movements, cartera
	Action string                 `json:"action"` // add, edit, delete
	Key    string                 `json:"key"`    // unique identifier (NIT, code, etc.)
	Data   map[string]interface{} `json:"data"`   // record fields
}

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

// Sync sends a single record change to the server via the universal endpoint
func (c *Client) Sync(table, action, key string, data map[string]interface{}) error {
	payload := SyncPayload{
		Table:  table,
		Action: action,
		Key:    key,
		Data:   data,
	}

	_, err := c.doRequest("POST", "/siigo/sync", payload, true)
	if err != nil {
		return fmt.Errorf("sync %s/%s failed: %w", table, action, err)
	}
	return nil
}

// IsAuthenticated returns true if the client has a valid token
func (c *Client) IsAuthenticated() bool {
	return c.token != ""
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
