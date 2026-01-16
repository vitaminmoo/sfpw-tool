package ble

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"sfpw-tool/internal/config"
	"sfpw-tool/internal/protocol"
	"sfpw-tool/internal/util"

	"tinygo.org/x/bluetooth"
)

// APIContext holds the BLE characteristics needed for API communication
type APIContext struct {
	WriteChar  *bluetooth.DeviceCharacteristic
	NotifyChar *bluetooth.DeviceCharacteristic
	MAC        string // lowercase, no separators (e.g., "deadbeefcafe")

	// For handling responses
	responseMu    sync.Mutex
	responseBuf   bytes.Buffer
	expectedLen   int
	responseChan  chan bool
	notifyEnabled bool
}

// APIPath builds an API path with the device MAC
func (ctx *APIContext) APIPath(endpoint string) string {
	return fmt.Sprintf("/api/1.0/%s%s", ctx.MAC, endpoint)
}

// enableNotifications sets up the notification handler for API responses
func (ctx *APIContext) enableNotifications() error {
	if ctx.notifyEnabled {
		return nil
	}

	ctx.responseChan = make(chan bool, 1)

	err := ctx.NotifyChar.EnableNotifications(func(buf []byte) {
		ctx.responseMu.Lock()
		defer ctx.responseMu.Unlock()

		config.Debugf("Notification received: %d bytes (total so far: %d)", len(buf), ctx.responseBuf.Len())
		if config.Verbose {
			util.PrintHexDump(buf)
		}

		// First packet - parse outer header to get expected length
		if ctx.responseBuf.Len() == 0 && len(buf) >= 4 {
			ctx.expectedLen = int(binary.BigEndian.Uint16(buf[0:2]))
			config.Debugf("Expected total length: %d bytes", ctx.expectedLen)
		}

		ctx.responseBuf.Write(buf)

		// Check if we have complete response
		if ctx.expectedLen > 0 && ctx.responseBuf.Len() >= ctx.expectedLen {
			config.Debugf("Response complete: %d/%d bytes", ctx.responseBuf.Len(), ctx.expectedLen)
			select {
			case ctx.responseChan <- true:
			default:
			}
		}
	})
	if err != nil {
		return err
	}

	ctx.notifyEnabled = true
	time.Sleep(100 * time.Millisecond)
	return nil
}

// resetResponseBuffer clears the response buffer for a new request
func (ctx *APIContext) resetResponseBuffer() {
	ctx.responseMu.Lock()
	ctx.responseBuf.Reset()
	ctx.expectedLen = 0
	ctx.responseMu.Unlock()
	// Drain channel
	select {
	case <-ctx.responseChan:
	default:
	}
}

// waitForResponse waits for a complete response with timeout
func (ctx *APIContext) waitForResponse(timeout time.Duration) ([]byte, error) {
	select {
	case <-ctx.responseChan:
		ctx.responseMu.Lock()
		data := make([]byte, ctx.responseBuf.Len())
		copy(data, ctx.responseBuf.Bytes())
		ctx.responseMu.Unlock()
		return data, nil
	case <-time.After(timeout):
		ctx.responseMu.Lock()
		got := ctx.responseBuf.Len()
		expected := ctx.expectedLen
		ctx.responseMu.Unlock()
		return nil, fmt.Errorf("timeout (got %d/%d bytes)", got, expected)
	}
}

// SendRequest sends an API request and waits for response
func (ctx *APIContext) SendRequest(method, path string, body []byte, timeout time.Duration) (*protocol.APIResponse, []byte, error) {
	if err := ctx.enableNotifications(); err != nil {
		return nil, nil, fmt.Errorf("failed to enable notifications: %w", err)
	}

	ctx.resetResponseBuffer()

	requestID, seqNum := protocol.NextRequestID()

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
	if config.Verbose {
		util.PrintHexDump(dataToSend)
	}
	_, err = ctx.WriteChar.WriteWithoutResponse(dataToSend)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Wait for response
	data, err := ctx.waitForResponse(timeout)
	if err != nil {
		return nil, nil, err
	}

	headerJSON, bodyData, err := protocol.BinmeDecode(data)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var resp protocol.APIResponse
	if err := json.Unmarshal(headerJSON, &resp); err != nil {
		return nil, nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, bodyData, nil
}

// SendRawBodyRequest sends an API request with a raw binary body (for XSFP writes)
// Large packets are fragmented across multiple BLE writes.
func (ctx *APIContext) SendRawBodyRequest(method, path string, body []byte, timeout time.Duration) (*protocol.APIResponse, []byte, error) {
	if err := ctx.enableNotifications(); err != nil {
		return nil, nil, fmt.Errorf("failed to enable notifications: %w", err)
	}

	ctx.resetResponseBuffer()

	requestID, seqNum := protocol.NextRequestID()

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
	config.Debugf("Body: %d bytes of binary data", len(body))

	// Use raw body encoding for binary data
	dataToSend, err := protocol.BinmeEncodeRawBody(reqData, body, seqNum)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode binme: %w", err)
	}

	config.Debugf("Total packet size: %d bytes", len(dataToSend))

	// Fragment into BLE MTU-sized chunks (244 bytes is typical for BLE 4.2+)
	const bleMTU = 244
	for offset := 0; offset < len(dataToSend); offset += bleMTU {
		end := offset + bleMTU
		if end > len(dataToSend) {
			end = len(dataToSend)
		}
		chunk := dataToSend[offset:end]

		config.Debugf("Writing chunk %d-%d (%d bytes)", offset, end, len(chunk))
		if config.Verbose {
			util.PrintHexDump(chunk)
		}
		_, err = ctx.WriteChar.WriteWithoutResponse(chunk)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to write chunk at offset %d: %w", offset, err)
		}

		// Small delay between chunks to let device process
		if end < len(dataToSend) {
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Wait for response
	data, err := ctx.waitForResponse(timeout)
	if err != nil {
		return nil, nil, err
	}

	headerJSON, bodyData, err := protocol.BinmeDecode(data)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var resp protocol.APIResponse
	if err := json.Unmarshal(headerJSON, &resp); err != nil {
		return nil, nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, bodyData, nil
}
