package sqmclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client polls a SQMeter ESP32 REST API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New creates a Client that targets the given base URL.
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// FetchSensors calls GET /api/sensors and returns the parsed response.
func (c *Client) FetchSensors(ctx context.Context) (*SensorsResponse, error) {
	return fetchJSON[SensorsResponse](ctx, c.httpClient, c.baseURL+"/api/sensors")
}

// FetchStatus calls GET /api/status and returns the parsed response.
func (c *Client) FetchStatus(ctx context.Context) (*StatusResponse, error) {
	return fetchJSON[StatusResponse](ctx, c.httpClient, c.baseURL+"/api/status")
}

func fetchJSON[T any](ctx context.Context, client *http.Client, url string) (*T, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request for %s: %w", url, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: unexpected status %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB cap
	if err != nil {
		return nil, fmt.Errorf("reading response from %s: %w", url, err)
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing JSON from %s: %w", url, err)
	}
	return &result, nil
}
