package api

import (
	"encoding/json"
	"fmt"
	"time"

	"sfpw-tool/internal/config"
)

// FirmwareStartResponse represents the response from POST /fw/start.
type FirmwareStartResponse struct {
	Status string `json:"status"`
	Offset int    `json:"offset"`
	Chunk  int    `json:"chunk"`
	Size   int    `json:"size"`
}

// StartFirmwareUpdate initiates a firmware update.
func (c *Client) StartFirmwareUpdate(size int) (*FirmwareStartResponse, error) {
	reqBody := fmt.Sprintf(`{"size":%d}`, size)
	resp, body, err := c.Send("POST", "/fw/start", []byte(reqBody), nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var startResp FirmwareStartResponse
	if err := json.Unmarshal(body, &startResp); err != nil {
		// Return with defaults if we can't parse
		config.Debugf("Could not parse start response: %v, body: %s", err, string(body))
		return &FirmwareStartResponse{Status: "ready"}, nil
	}

	return &startResp, nil
}

// SendFirmwareChunk sends a chunk of firmware data.
func (c *Client) SendFirmwareChunk(chunk []byte) error {
	resp, body, err := c.Send("POST", "/fw/data", chunk, &RequestOptions{
		Timeout: 30 * time.Second,
		RawBody: true,
	})
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// AbortFirmwareUpdate aborts an in-progress firmware update.
func (c *Client) AbortFirmwareUpdate() error {
	resp, body, err := c.Send("POST", "/fw/abort", nil, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
