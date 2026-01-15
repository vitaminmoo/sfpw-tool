package api

import (
	"encoding/json"
	"fmt"
	"time"

	"sfpw-tool/internal/ble"
	"sfpw-tool/internal/protocol"

	"tinygo.org/x/bluetooth"
)

// Client provides a high-level API for communicating with SFP Wizard devices.
// It wraps the low-level BLE operations and provides typed methods for each endpoint.
type Client struct {
	device  bluetooth.Device
	ctx     *ble.APIContext
	timeout time.Duration
}

// New creates a new API client for the given BLE device.
func New(device bluetooth.Device) *Client {
	return &Client{
		device:  device,
		timeout: 10 * time.Second,
	}
}

// Connect establishes the API context for communication.
func (c *Client) Connect() error {
	c.ctx = ble.SetupAPI(c.device)
	if c.ctx == nil {
		return fmt.Errorf("failed to setup API context")
	}
	return nil
}

// Disconnect releases resources (device disconnect handled separately).
func (c *Client) Disconnect() {
	// Currently nothing to do - device.Disconnect() called by caller
}

// SetTimeout sets the default request timeout.
func (c *Client) SetTimeout(d time.Duration) {
	c.timeout = d
}

// Context returns the underlying API context for direct access if needed.
func (c *Client) Context() *ble.APIContext {
	return c.ctx
}

// MAC returns the device MAC address.
func (c *Client) MAC() string {
	if c.ctx != nil {
		return c.ctx.MAC
	}
	return ""
}

// --- Low-level send methods ---

// RequestOptions configures how a request is sent.
type RequestOptions struct {
	Timeout     time.Duration // Request timeout (default: client timeout)
	RawBody     bool          // Use raw binary body encoding (for EEPROM writes)
	LargeChunks bool          // Fragment outgoing data for large writes
}

// Send sends an API request and returns the response.
func (c *Client) Send(method, endpoint string, body []byte, opts *RequestOptions) (*protocol.APIResponse, []byte, error) {
	if c.ctx == nil {
		return nil, nil, fmt.Errorf("not connected")
	}

	timeout := c.timeout
	if opts != nil && opts.Timeout > 0 {
		timeout = opts.Timeout
	}

	path := c.ctx.APIPath(endpoint)

	if opts != nil && opts.RawBody {
		return c.ctx.SendRawBodyRequest(method, path, body, timeout)
	}
	return c.ctx.SendRequest(method, path, body, timeout)
}

// --- JSON helpers ---

// GetJSON sends a GET request and returns the JSON body.
func (c *Client) GetJSON(endpoint string) (json.RawMessage, error) {
	resp, body, err := c.Send("GET", endpoint, nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// PostJSON sends a POST request with a JSON body and returns the JSON response.
func (c *Client) PostJSON(endpoint string, payload any) (json.RawMessage, error) {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err)
		}
	}

	resp, respBody, err := c.Send("POST", endpoint, body, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

// --- Binary data helpers ---

// FetchBinary fetches binary data using the start/data pattern.
// Used for module read, snapshot read, and SIF dump operations.
func (c *Client) FetchBinary(startEndpoint, dataEndpoint string) ([]byte, error) {
	// Step 1: GET start endpoint to initialize and get size
	resp, body, err := c.Send("GET", startEndpoint, nil, &RequestOptions{Timeout: 10 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize: %w", err)
	}
	if resp.StatusCode != 200 {
		if len(body) > 0 {
			return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
		}
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	// Parse to get size info
	var startResp struct {
		Size  int `json:"size"`
		Chunk int `json:"chunk"`
	}
	if err := json.Unmarshal(body, &startResp); err != nil {
		// Default to SFP size
		startResp.Size = 512
		startResp.Chunk = 512
	}
	if startResp.Size == 0 {
		startResp.Size = 512
	}

	// Step 2: GET data endpoint to read the data
	reqBody := fmt.Sprintf(`{"offset":0,"chunk":%d}`, startResp.Size)
	resp, body, err = c.Send("GET", dataEndpoint, []byte(reqBody), &RequestOptions{Timeout: 30 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("failed to read data: %w", err)
	}
	if resp.StatusCode != 200 {
		if len(body) > 0 {
			return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
		}
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	return body, nil
}

// SendBinary sends binary data using the start/data pattern.
// Used for snapshot write operations.
func (c *Client) SendBinary(startEndpoint, dataEndpoint string, data []byte) error {
	// Step 1: POST start endpoint with size
	startBody := fmt.Sprintf(`{"size":%d}`, len(data))
	resp, body, err := c.Send("POST", startEndpoint, []byte(startBody), &RequestOptions{Timeout: 10 * time.Second})
	if err != nil {
		return fmt.Errorf("failed to initialize: %w", err)
	}
	if resp.StatusCode != 200 {
		if len(body) > 0 {
			return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
		}
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	// Step 2: POST data endpoint with raw binary
	resp, body, err = c.Send("POST", dataEndpoint, data, &RequestOptions{
		Timeout: 30 * time.Second,
		RawBody: true,
	})
	if err != nil {
		return fmt.Errorf("failed to send data: %w", err)
	}
	if resp.StatusCode != 200 {
		if len(body) > 0 {
			return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
		}
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	return nil
}
