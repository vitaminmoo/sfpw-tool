package protocol

import "encoding/json"

// DeviceInfo represents the JSON response from the device info characteristic
type DeviceInfo struct {
	ID         string `json:"id"`
	FWVersion  string `json:"fwv"`
	APIVersion string `json:"apiVersion"`
	Voltage    string `json:"voltage"`
	Level      string `json:"level"`
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

// ResponseData holds the parsed response envelope and body
type ResponseData struct {
	Envelope APIResponse
	Body     []byte
}
