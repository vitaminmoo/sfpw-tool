package ble

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/vitaminmoo/sfpw-tool/internal/config"
	"github.com/vitaminmoo/sfpw-tool/internal/protocol"
	"github.com/vitaminmoo/sfpw-tool/internal/util"

	"tinygo.org/x/bluetooth"
)

// SendAPIRequest sends a JSON API request and waits for the response.
// The request is wrapped in the binme binary envelope format with zlib compression.
// method is the HTTP method (GET or POST), path is the API endpoint.
func SendAPIRequest(writeChar, notifyChar *bluetooth.DeviceCharacteristic, method, path string, body []byte) (*protocol.APIResponse, []byte, error) {
	requestID, seqNum := protocol.NextRequestID()

	// Channel to receive the response
	responseChan := make(chan protocol.ResponseData, 1)
	var mu sync.Mutex

	// Enable notifications to receive response
	config.Debugf("Enabling notifications on characteristic...")
	err := notifyChar.EnableNotifications(func(buf []byte) {
		config.Debugf("Notification received: %d bytes", len(buf))
		config.Debugf("Raw hex: %X", buf)

		// Decode binme envelope
		headerJSON, bodyData, err := protocol.BinmeDecode(buf)
		if err != nil {
			config.Debugf("Failed to decode binme: %v", err)
			return
		}

		config.Debugf("Decoded binme header JSON: %s", string(headerJSON))
		if len(bodyData) > 0 {
			if util.IsTextData(bodyData) {
				config.Debugf("Decoded binme body: %s", string(bodyData))
			} else {
				config.Debugf("Decoded binme body hex: %X", bodyData)
			}
		}

		// Parse as API response
		var resp protocol.APIResponse
		if err := json.Unmarshal(headerJSON, &resp); err != nil {
			config.Debugf("Failed to parse as APIResponse: %v", err)
			return
		}

		config.Debugf("Parsed response: type=%s, id=%s, status=%d", resp.Type, resp.ID, resp.StatusCode)

		// Check if this is our response
		if resp.ID == requestID {
			mu.Lock()
			select {
			case responseChan <- protocol.ResponseData{Envelope: resp, Body: bodyData}:
			default:
			}
			mu.Unlock()
		} else {
			config.Debugf("Response ID mismatch: got %s, want %s", resp.ID, requestID)
		}
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to enable notifications: %w", err)
	}
	config.Debugf("Notifications enabled successfully")

	// Small delay to let subscription settle
	time.Sleep(100 * time.Millisecond)

	// Build the request envelope
	req := protocol.APIRequest{
		Type:      "httpRequest",
		ID:        requestID,
		Timestamp: time.Now().UnixMilli(),
		Method:    method,
		Path:      path,
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	config.Debugf("JSON request: %s", string(reqData))

	// Wrap in binme envelope with zlib compression
	dataToSend, err := protocol.BinmeEncode(reqData, body, seqNum)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode binme: %w", err)
	}
	config.Debugf("Binme encoded: %d bytes", len(dataToSend))
	config.Debugf("Binme hex: %X", dataToSend)

	// Write request to characteristic
	// NOTE: tinygo bluetooth on Linux doesn't support Write with Response (only WriteWithoutResponse)
	// See: https://github.com/tinygo-org/bluetooth/issues/153
	// The official app uses Write Request (0x12), but we have to try WriteWithoutResponse
	config.Debugf("Writing %d bytes to characteristic...", len(dataToSend))
	_, err = writeChar.WriteWithoutResponse(dataToSend)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write request: %w", err)
	}
	config.Debugf("Write completed")

	// Wait for response with timeout
	select {
	case resp := <-responseChan:
		return &resp.Envelope, resp.Body, nil
	case <-time.After(5 * time.Second):
		return nil, nil, fmt.Errorf("timeout waiting for response (request ID: %s)", requestID)
	}
}

// SendLargeAPIRequest sends an API request and handles fragmented BLE responses.
// Large responses (like SIF data) are split across multiple BLE notifications.
func SendLargeAPIRequest(writeChar, notifyChar *bluetooth.DeviceCharacteristic, method, path string, body []byte, timeout time.Duration) (*protocol.APIResponse, []byte, error) {
	requestID, seqNum := protocol.NextRequestID()

	// Buffer to accumulate fragmented response
	var responseBuf bytes.Buffer
	var expectedLen int
	responseChan := make(chan bool, 1)
	var mu sync.Mutex

	config.Debugf("Enabling notifications for large response...")
	err := notifyChar.EnableNotifications(func(buf []byte) {
		mu.Lock()
		defer mu.Unlock()

		config.Debugf("Notification received: %d bytes (total so far: %d)", len(buf), responseBuf.Len())

		// First packet - parse outer header to get expected length
		if responseBuf.Len() == 0 && len(buf) >= 4 {
			expectedLen = int(binary.BigEndian.Uint16(buf[0:2]))
			config.Debugf("Expected total length: %d bytes", expectedLen)
		}

		responseBuf.Write(buf)

		// Check if we have complete response
		if expectedLen > 0 && responseBuf.Len() >= expectedLen {
			config.Debugf("Response complete: %d/%d bytes", responseBuf.Len(), expectedLen)
			select {
			case responseChan <- true:
			default:
			}
		}
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to enable notifications: %w", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Build and send request
	req := protocol.APIRequest{
		Type:      "httpRequest",
		ID:        requestID,
		Timestamp: time.Now().UnixMilli(),
		Method:    method,
		Path:      path,
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	config.Debugf("JSON request: %s", string(reqData))

	dataToSend, err := protocol.BinmeEncode(reqData, body, seqNum)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode binme: %w", err)
	}

	config.Debugf("Writing %d bytes...", len(dataToSend))
	_, err = writeChar.WriteWithoutResponse(dataToSend)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Wait for complete response
	select {
	case <-responseChan:
		mu.Lock()
		data := responseBuf.Bytes()
		mu.Unlock()

		headerJSON, bodyData, err := protocol.BinmeDecode(data)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decode response: %w", err)
		}

		var resp protocol.APIResponse
		if err := json.Unmarshal(headerJSON, &resp); err != nil {
			return nil, nil, fmt.Errorf("failed to parse response: %w", err)
		}

		return &resp, bodyData, nil

	case <-time.After(timeout):
		mu.Lock()
		got := responseBuf.Len()
		mu.Unlock()
		return nil, nil, fmt.Errorf("timeout waiting for response (got %d/%d bytes)", got, expectedLen)
	}
}
