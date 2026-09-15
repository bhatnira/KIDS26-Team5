package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Client is the Antelope Go SDK client. It manages authentication and
// provides typed methods for every API endpoint.
type Client struct {
	baseURL    string
	httpClient *http.Client

	mu           sync.Mutex
	accessToken  string
	refreshToken string
	accessExp    time.Time
}

// New creates a new SDK client for the given Antelope server URL.
func New(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Login authenticates with email/password and stores tokens.
func (c *Client) Login(email, password string) error {
	body := map[string]string{"email": email, "password": password}
	resp, err := c.post("/user/login", body, false)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if result.Code != 2000 {
		return fmt.Errorf("login failed: %s", result.Msg)
	}

	data := result.Data
	c.mu.Lock()
	c.accessToken, _ = data["accessToken"].(string)
	c.refreshToken, _ = data["refreshToken"].(string)
	if exp, ok := data["accessExpiresAt"].(float64); ok {
		c.accessExp = time.UnixMilli(int64(exp))
	}
	c.mu.Unlock()
	return nil
}

// SetTokens manually sets tokens (for CLI config persistence).
func (c *Client) SetTokens(access, refresh string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.accessToken = access
	c.refreshToken = refresh
}

type apiResponse struct {
	Code int            `json:"code"`
	Data map[string]any `json:"data"`
	Msg  string         `json:"msg"`
}

func (c *Client) do(method, path string, body any, auth bool) (*http.Response, error) {
	url := c.baseURL + path

	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	if auth {
		c.mu.Lock()
		token := c.accessToken
		c.mu.Unlock()
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

	return c.httpClient.Do(req)
}

func (c *Client) get(path string, auth bool) (*http.Response, error) {
	return c.do(http.MethodGet, path, nil, auth)
}

func (c *Client) post(path string, body any, auth bool) (*http.Response, error) {
	return c.do(http.MethodPost, path, body, auth)
}
