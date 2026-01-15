package api

import (
	"encoding/json"
	"fmt"
	"time"
)

// SIFStatus represents the SIF operation status.
type SIFStatus struct {
	Status string `json:"status"`
	Offset int    `json:"offset"`
	Chunk  int    `json:"chunk,omitempty"`
	Size   int    `json:"size,omitempty"`
}

// GetSIFStatus returns the current SIF operation status.
func (c *Client) GetSIFStatus() (*SIFStatus, error) {
	resp, body, err := c.Send("GET", "/sif/info/", nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var status SIFStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// AbortSIF aborts any in-progress SIF operation.
func (c *Client) AbortSIF() error {
	resp, body, err := c.Send("POST", "/sif/abort", nil, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// AbortSIFIfRunning checks SIF status and aborts if an operation is in progress.
func (c *Client) AbortSIFIfRunning() error {
	status, err := c.GetSIFStatus()
	if err != nil {
		return fmt.Errorf("failed to get SIF status: %w", err)
	}

	// Only abort if actively in progress (not finished/complete/idle)
	if status.Status == "inprogress" || status.Status == "ready" || status.Status == "continue" {
		if err := c.AbortSIF(); err != nil {
			return fmt.Errorf("failed to abort SIF: %w", err)
		}
		time.Sleep(500 * time.Millisecond)
	}

	return nil
}

// ReadSIF reads the SIF (support dump) archive.
// Returns a tar archive containing syslog and module database.
func (c *Client) ReadSIF() ([]byte, error) {
	// Step 1: POST /sif/start to initiate
	resp, body, err := c.Send("POST", "/sif/start", nil, &RequestOptions{Timeout: 10 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("failed to start SIF read: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var startResp SIFStatus
	if err := json.Unmarshal(body, &startResp); err != nil {
		return nil, fmt.Errorf("failed to parse start response: %w", err)
	}

	// Allocate buffer for full data
	data := make([]byte, 0, startResp.Size)
	offset := 0
	chunkSize := startResp.Chunk
	if chunkSize == 0 {
		chunkSize = 512
	}

	// Step 2: GET /sif/data/ in a loop to fetch chunks
	for offset < startResp.Size {
		remaining := startResp.Size - offset
		if remaining < chunkSize {
			chunkSize = remaining
		}

		reqBody := fmt.Sprintf(`{"status":"continue","offset":%d,"chunk":%d}`, offset, chunkSize)
		resp, body, err := c.Send("GET", "/sif/data/", []byte(reqBody), &RequestOptions{Timeout: 30 * time.Second})
		if err != nil {
			return nil, fmt.Errorf("failed to read SIF data: %w", err)
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
		}

		// Handle end of data
		if len(body) == 0 {
			break
		}

		data = append(data, body...)
		offset += len(body)
	}

	return data, nil
}
