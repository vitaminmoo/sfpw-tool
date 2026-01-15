package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"
)

// DeviceInfo represents the JSON response from the device
type DeviceInfo struct {
	ID         string `json:"id"`
	FWVersion  string `json:"fwv"`
	APIVersion string `json:"apiVersion"`
	Voltage    string `json:"voltage"`
	Level      string `json:"level"`
}

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

// APIRequest is the JSON envelope for API requests
// The firmware requires "type": "httpRequest" to route to the API handler
type APIRequest struct {
	Type      string   `json:"type"`
	ID        string   `json:"id"`
	Timestamp int64    `json:"timestamp"`
	Method    string   `json:"method"` // HTTP method: GET or POST
	Path      string   `json:"path"`   // API endpoint path
	Headers   struct{} `json:"headers"`
}

// APIResponse is the JSON envelope for API responses
// The firmware sends "type": "httpResponse" for API responses
type APIResponse struct {
	Type       string          `json:"type"`
	ID         string          `json:"id"`
	Timestamp  int64           `json:"timestamp"`
	StatusCode int             `json:"statusCode"`
	Headers    struct{}        `json:"headers"`
	Body       json.RawMessage `json:"body"`
}

// responseData holds the parsed response envelope and body
type responseData struct {
	envelope APIResponse
	body     []byte
}

// apiPath builds an API path with the device MAC
func (ctx *APIContext) apiPath(endpoint string) string {
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

		debugf("Notification received: %d bytes (total so far: %d)", len(buf), ctx.responseBuf.Len())

		// First packet - parse outer header to get expected length
		if ctx.responseBuf.Len() == 0 && len(buf) >= 4 {
			ctx.expectedLen = int(binary.BigEndian.Uint16(buf[0:2]))
			debugf("Expected total length: %d bytes", ctx.expectedLen)
		}

		ctx.responseBuf.Write(buf)

		// Check if we have complete response
		if ctx.expectedLen > 0 && ctx.responseBuf.Len() >= ctx.expectedLen {
			debugf("Response complete: %d/%d bytes", ctx.responseBuf.Len(), ctx.expectedLen)
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

// sendRequest sends an API request and waits for response
func (ctx *APIContext) sendRequest(method, path string, body []byte, timeout time.Duration) (*APIResponse, []byte, error) {
	if err := ctx.enableNotifications(); err != nil {
		return nil, nil, fmt.Errorf("failed to enable notifications: %w", err)
	}

	ctx.resetResponseBuffer()

	requestID, seqNum := nextRequestID()

	req := APIRequest{
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

	debugf("JSON request: %s", string(reqData))

	dataToSend, err := binmeEncode(reqData, body, seqNum)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode binme: %w", err)
	}

	debugf("Writing %d bytes...", len(dataToSend))
	_, err = ctx.WriteChar.WriteWithoutResponse(dataToSend)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Wait for response
	data, err := ctx.waitForResponse(timeout)
	if err != nil {
		return nil, nil, err
	}

	headerJSON, bodyData, err := binmeDecode(data)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var resp APIResponse
	if err := json.Unmarshal(headerJSON, &resp); err != nil {
		return nil, nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, bodyData, nil
}

// sendRawBodyRequest sends an API request with a raw binary body (for XSFP writes)
// Large packets are fragmented across multiple BLE writes.
func (ctx *APIContext) sendRawBodyRequest(method, path string, body []byte, timeout time.Duration) (*APIResponse, []byte, error) {
	if err := ctx.enableNotifications(); err != nil {
		return nil, nil, fmt.Errorf("failed to enable notifications: %w", err)
	}

	ctx.resetResponseBuffer()

	requestID, seqNum := nextRequestID()

	req := APIRequest{
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

	debugf("JSON request: %s", string(reqData))
	debugf("Body: %d bytes of binary data", len(body))

	// Use raw body encoding for binary data
	dataToSend, err := binmeEncodeRawBody(reqData, body, seqNum)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode binme: %w", err)
	}

	debugf("Total packet size: %d bytes", len(dataToSend))

	// Fragment into BLE MTU-sized chunks (244 bytes is typical for BLE 4.2+)
	const bleMTU = 244
	for offset := 0; offset < len(dataToSend); offset += bleMTU {
		end := offset + bleMTU
		if end > len(dataToSend) {
			end = len(dataToSend)
		}
		chunk := dataToSend[offset:end]

		debugf("Writing chunk %d-%d (%d bytes)", offset, end, len(chunk))
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

	headerJSON, bodyData, err := binmeDecode(data)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var resp APIResponse
	if err := json.Unmarshal(headerJSON, &resp); err != nil {
		return nil, nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, bodyData, nil
}

// sendAPIRequest sends a JSON API request and waits for the response.
// The request is wrapped in the binme binary envelope format with zlib compression.
// method is the HTTP method (GET or POST), path is the API endpoint.
func sendAPIRequest(writeChar, notifyChar *bluetooth.DeviceCharacteristic, method, path string, body []byte) (*APIResponse, []byte, error) {
	requestID, seqNum := nextRequestID()

	// Channel to receive the response
	responseChan := make(chan responseData, 1)
	var mu sync.Mutex

	// Enable notifications to receive response
	debugf("Enabling notifications on characteristic...")
	err := notifyChar.EnableNotifications(func(buf []byte) {
		debugf("Notification received: %d bytes", len(buf))
		debugf("Raw hex: %X", buf)

		// Decode binme envelope
		headerJSON, bodyData, err := binmeDecode(buf)
		if err != nil {
			debugf("Failed to decode binme: %v", err)
			return
		}

		debugf("Decoded binme header JSON: %s", string(headerJSON))
		if len(bodyData) > 0 {
			if isTextData(bodyData) {
				debugf("Decoded binme body: %s", string(bodyData))
			} else {
				debugf("Decoded binme body hex: %X", bodyData)
			}
		}

		// Parse as API response
		var resp APIResponse
		if err := json.Unmarshal(headerJSON, &resp); err != nil {
			debugf("Failed to parse as APIResponse: %v", err)
			return
		}

		debugf("Parsed response: type=%s, id=%s, status=%d", resp.Type, resp.ID, resp.StatusCode)

		// Check if this is our response
		if resp.ID == requestID {
			mu.Lock()
			select {
			case responseChan <- responseData{envelope: resp, body: bodyData}:
			default:
			}
			mu.Unlock()
		} else {
			debugf("Response ID mismatch: got %s, want %s", resp.ID, requestID)
		}
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to enable notifications: %w", err)
	}
	debugf("Notifications enabled successfully")

	// Small delay to let subscription settle
	time.Sleep(100 * time.Millisecond)

	// Build the request envelope
	req := APIRequest{
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

	debugf("JSON request: %s", string(reqData))

	// Wrap in binme envelope with zlib compression
	dataToSend, err := binmeEncode(reqData, body, seqNum)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode binme: %w", err)
	}
	debugf("Binme encoded: %d bytes", len(dataToSend))
	debugf("Binme hex: %X", dataToSend)

	// Write request to characteristic
	// NOTE: tinygo bluetooth on Linux doesn't support Write with Response (only WriteWithoutResponse)
	// See: https://github.com/tinygo-org/bluetooth/issues/153
	// The official app uses Write Request (0x12), but we have to try WriteWithoutResponse
	debugf("Writing %d bytes to characteristic...", len(dataToSend))
	_, err = writeChar.WriteWithoutResponse(dataToSend)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write request: %w", err)
	}
	debugf("Write completed")

	// Wait for response with timeout
	select {
	case resp := <-responseChan:
		return &resp.envelope, resp.body, nil
	case <-time.After(5 * time.Second):
		return nil, nil, fmt.Errorf("timeout waiting for response (request ID: %s)", requestID)
	}
}

// sendLargeAPIRequest sends an API request and handles fragmented BLE responses.
// Large responses (like SIF data) are split across multiple BLE notifications.
func sendLargeAPIRequest(writeChar, notifyChar *bluetooth.DeviceCharacteristic, method, path string, body []byte, timeout time.Duration) (*APIResponse, []byte, error) {
	requestID, seqNum := nextRequestID()

	// Buffer to accumulate fragmented response
	var responseBuf bytes.Buffer
	var expectedLen int
	responseChan := make(chan bool, 1)
	var mu sync.Mutex
	var decodeErr error

	debugf("Enabling notifications for large response...")
	err := notifyChar.EnableNotifications(func(buf []byte) {
		mu.Lock()
		defer mu.Unlock()

		debugf("Notification received: %d bytes (total so far: %d)", len(buf), responseBuf.Len())

		// First packet - parse outer header to get expected length
		if responseBuf.Len() == 0 && len(buf) >= 4 {
			expectedLen = int(binary.BigEndian.Uint16(buf[0:2]))
			debugf("Expected total length: %d bytes", expectedLen)
		}

		responseBuf.Write(buf)

		// Check if we have complete response
		if expectedLen > 0 && responseBuf.Len() >= expectedLen {
			debugf("Response complete: %d/%d bytes", responseBuf.Len(), expectedLen)
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
	req := APIRequest{
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

	debugf("JSON request: %s", string(reqData))

	dataToSend, err := binmeEncode(reqData, body, seqNum)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode binme: %w", err)
	}

	debugf("Writing %d bytes...", len(dataToSend))
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

		if decodeErr != nil {
			return nil, nil, decodeErr
		}

		headerJSON, bodyData, err := binmeDecode(data)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decode response: %w", err)
		}

		var resp APIResponse
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

// zlibCompress compresses data using zlib
func zlibCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, err := w.Write(data)
	if err != nil {
		return nil, err
	}
	err = w.Close()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// zlibDecompress decompresses zlib data
func zlibDecompress(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// binmeEncode wraps JSON data in the binme binary envelope format with zlib compression.
// Format:
//
//	[Outer Header - 4 bytes]
//	  bytes 0-1: total message length (big-endian)
//	  bytes 2-3: flags (00 03 for requests)
//	[Header Section - 9 bytes + zlib data]
//	  byte 0: marker (0x03 = header section)
//	  byte 1: format (0x01 = JSON)
//	  byte 2: compression (0x01 = zlib)
//	  byte 3: flags (0x01)
//	  bytes 4-7: decompressed length (big-endian)
//	  byte 8: compressed length (for short messages)
//	  bytes 9+: zlib compressed JSON
//	[Body Section - 8 bytes + zlib data]
//	  byte 0: marker (0x02 = body section)
//	  byte 1: format (0x01 = JSON)
//	  byte 2: compression (0x01 = zlib)
//	  byte 3: reserved (0x00)
//	  bytes 4-7: compressed length (big-endian)
//	  bytes 8+: zlib compressed body
func binmeEncode(jsonData []byte, bodyData []byte, seqNum uint16) ([]byte, error) {
	// Compress header JSON
	compressedHeader, err := zlibCompress(jsonData)
	if err != nil {
		return nil, fmt.Errorf("failed to compress header: %w", err)
	}

	// Compress body
	compressedBody, err := zlibCompress(bodyData)
	if err != nil {
		return nil, fmt.Errorf("failed to compress body: %w", err)
	}

	// Build the message
	var buf bytes.Buffer

	// We'll write the content first, then prepend the outer header

	// Header section: 9 bytes header + compressed data
	headerSection := make([]byte, 9+len(compressedHeader))
	headerSection[0] = 0x03 // marker: header section
	headerSection[1] = 0x01 // format: JSON
	headerSection[2] = 0x01 // compression: zlib
	headerSection[3] = 0x01 // flags
	// bytes 4-7: always 00 00 00 00 in captured traffic
	headerSection[4] = 0x00
	headerSection[5] = 0x00
	headerSection[6] = 0x00
	headerSection[7] = 0x00
	// Compressed length (single byte)
	headerSection[8] = byte(len(compressedHeader))
	copy(headerSection[9:], compressedHeader)

	// Body section: 8 bytes header + compressed data
	bodySection := make([]byte, 8+len(compressedBody))
	bodySection[0] = 0x02 // marker: body section
	bodySection[1] = 0x01 // format: JSON
	bodySection[2] = 0x01 // compression: zlib
	bodySection[3] = 0x00 // reserved
	// Compressed length (big-endian)
	binary.BigEndian.PutUint32(bodySection[4:8], uint32(len(compressedBody)))
	copy(bodySection[8:], compressedBody)

	// Total message length (excluding outer header)
	totalLen := len(headerSection) + len(bodySection)

	// Write outer header
	outerHeader := make([]byte, 4)
	binary.BigEndian.PutUint16(outerHeader[0:2], uint16(totalLen+4)) // total including header
	binary.BigEndian.PutUint16(outerHeader[2:4], seqNum)             // sequence number matches request ID

	buf.Write(outerHeader)
	buf.Write(headerSection)
	buf.Write(bodySection)

	return buf.Bytes(), nil
}

// binmeDecode extracts JSON data from a binme binary envelope with zlib decompression.
// Returns the header JSON and body data.
func binmeDecode(data []byte) (headerJSON []byte, bodyData []byte, err error) {
	if len(data) < 4 {
		return nil, nil, fmt.Errorf("binme data too short: %d bytes", len(data))
	}

	// Skip outer header (4 bytes)
	// totalLen := binary.BigEndian.Uint16(data[0:2])
	// flags := binary.BigEndian.Uint16(data[2:4])
	pos := 4

	if len(data) < pos+9 {
		return nil, nil, fmt.Errorf("binme data too short for header section")
	}

	// Parse header section
	headerMarker := data[pos]
	if headerMarker != 0x03 {
		return nil, nil, fmt.Errorf("expected header marker 0x03, got 0x%02x", headerMarker)
	}
	// headerFormat := data[pos+1]
	headerCompressed := data[pos+2]
	// headerFlags := data[pos+3]
	// decompressedLen := binary.BigEndian.Uint32(data[pos+4 : pos+8])
	compressedHeaderLen := int(data[pos+8])

	pos += 9
	if len(data) < pos+compressedHeaderLen {
		return nil, nil, fmt.Errorf("binme header data truncated")
	}

	compressedHeader := data[pos : pos+compressedHeaderLen]
	pos += compressedHeaderLen

	// Decompress header if needed - check for zlib magic byte (0x78)
	// Response may have compression=01 but actually send raw JSON
	// Zlib headers: 78 01 (none), 78 5e (fast), 78 9c (default), 78 da (best)
	if headerCompressed == 0x01 && len(compressedHeader) >= 2 && compressedHeader[0] == 0x78 {
		headerJSON, err = zlibDecompress(compressedHeader)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decompress header: %w", err)
		}
	} else {
		// Raw data (not actually compressed despite flag)
		headerJSON = compressedHeader
	}

	// Parse body section if present
	if len(data) < pos+8 {
		// No body section
		return headerJSON, nil, nil
	}

	bodyMarker := data[pos]
	if bodyMarker != 0x02 {
		return nil, nil, fmt.Errorf("expected body marker 0x02, got 0x%02x", bodyMarker)
	}
	// bodyFormat := data[pos+1]
	bodyCompressed := data[pos+2]
	// bodyReserved := data[pos+3]
	compressedBodyLen := int(binary.BigEndian.Uint32(data[pos+4 : pos+8]))

	pos += 8
	if len(data) < pos+compressedBodyLen {
		return nil, nil, fmt.Errorf("binme body data truncated")
	}

	compressedBody := data[pos : pos+compressedBodyLen]

	// Decompress body if needed - check for zlib magic byte (0x78)
	// Zlib headers: 78 01 (none), 78 5e (fast), 78 9c (default), 78 da (best)
	if bodyCompressed == 0x01 && compressedBodyLen >= 2 && compressedBody[0] == 0x78 {
		bodyData, err = zlibDecompress(compressedBody)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decompress body: %w", err)
		}
	} else {
		bodyData = compressedBody
	}

	return headerJSON, bodyData, nil
}

// binmeEncodeRawBody wraps JSON header with a raw binary body (format=0x03).
// Used for XSFP write operations that send binary EEPROM data.
func binmeEncodeRawBody(jsonData []byte, bodyData []byte, seqNum uint16) ([]byte, error) {
	// Compress header JSON
	compressedHeader, err := zlibCompress(jsonData)
	if err != nil {
		return nil, fmt.Errorf("failed to compress header: %w", err)
	}

	// Build the message
	var buf bytes.Buffer

	// Header section: 9 bytes header + compressed data
	headerSection := make([]byte, 9+len(compressedHeader))
	headerSection[0] = 0x03 // marker: header section
	headerSection[1] = 0x01 // format: JSON
	headerSection[2] = 0x01 // compression: zlib
	headerSection[3] = 0x01 // flags
	headerSection[4] = 0x00
	headerSection[5] = 0x00
	headerSection[6] = 0x00
	headerSection[7] = 0x00
	headerSection[8] = byte(len(compressedHeader))
	copy(headerSection[9:], compressedHeader)

	// Body section: 8 bytes header + raw binary data (NOT compressed)
	bodySection := make([]byte, 8+len(bodyData))
	bodySection[0] = 0x02 // marker: body section
	bodySection[1] = 0x03 // format: raw binary (0x03)
	bodySection[2] = 0x00 // compression: none
	bodySection[3] = 0x00 // reserved
	binary.BigEndian.PutUint32(bodySection[4:8], uint32(len(bodyData)))
	copy(bodySection[8:], bodyData)

	// Total message length (excluding outer header)
	totalLen := len(headerSection) + len(bodySection)

	// Write outer header
	outerHeader := make([]byte, 4)
	binary.BigEndian.PutUint16(outerHeader[0:2], uint16(totalLen+4))
	binary.BigEndian.PutUint16(outerHeader[2:4], seqNum)

	buf.Write(outerHeader)
	buf.Write(headerSection)
	buf.Write(bodySection)

	return buf.Bytes(), nil
}
