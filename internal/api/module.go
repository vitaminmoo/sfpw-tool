package api

import (
	"encoding/json"
)

// ModuleDetails represents the inserted SFP module details.
type ModuleDetails struct {
	Type       string `json:"type,omitempty"`
	Present    bool   `json:"present,omitempty"`
	VendorName string `json:"vendorName,omitempty"`
	VendorPN   string `json:"vendorPn,omitempty"`
	VendorSN   string `json:"vendorSn,omitempty"`
}

// GetModuleDetails returns details about the inserted SFP module.
func (c *Client) GetModuleDetails() (*ModuleDetails, error) {
	body, err := c.GetJSON("/xsfp/module/details")
	if err != nil {
		return nil, err
	}

	var details ModuleDetails
	if err := json.Unmarshal(body, &details); err != nil {
		return nil, err
	}
	return &details, nil
}

// ReadModule reads the EEPROM from the physical module.
func (c *Client) ReadModule() ([]byte, error) {
	return c.FetchBinary("/xsfp/module/start", "/xsfp/module/data")
}

// GetSnapshotInfo returns snapshot buffer info as raw JSON.
func (c *Client) GetSnapshotInfo() (json.RawMessage, error) {
	return c.GetJSON("/xsfp/sync/start")
}

// ReadSnapshot reads the snapshot buffer.
func (c *Client) ReadSnapshot() ([]byte, error) {
	return c.FetchBinary("/xsfp/sync/start", "/xsfp/sync/data")
}

// WriteSnapshot writes EEPROM data to the snapshot buffer.
func (c *Client) WriteSnapshot(data []byte) error {
	return c.SendBinary("/xsfp/sync/start", "/xsfp/sync/data", data)
}
