package api

import (
	"encoding/json"
)

// ModuleDetails represents the inserted SFP module details.
type ModuleDetails struct {
	PartNumber string `json:"partNumber,omitempty"`
	Vendor     string `json:"vendor,omitempty"`
	SN         string `json:"sn,omitempty"`
	Rev        string `json:"rev,omitempty"`
	Compliance string `json:"compliance,omitempty"`
}

// IsModulePresent returns true if a module is detected.
func (d *ModuleDetails) IsModulePresent() bool {
	return d.PartNumber != "" || d.Vendor != ""
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

// SnapshotInfo represents the snapshot buffer status.
type SnapshotInfo struct {
	Size       int    `json:"size"`
	Chunk      int    `json:"chunk"`
	PartNumber string `json:"partNumber,omitempty"`
	Vendor     string `json:"vendor,omitempty"`
	SN         string `json:"sn,omitempty"`
}

// HasData returns true if the snapshot has data.
func (s *SnapshotInfo) HasData() bool {
	return s.Size > 0
}

// GetSnapshotInfo returns snapshot buffer info.
func (c *Client) GetSnapshotInfo() (*SnapshotInfo, error) {
	body, err := c.GetJSON("/xsfp/sync/start")
	if err != nil {
		return nil, err
	}

	var info SnapshotInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// ReadSnapshot reads the snapshot buffer.
func (c *Client) ReadSnapshot() ([]byte, error) {
	return c.FetchBinary("/xsfp/sync/start", "/xsfp/sync/data")
}

// WriteSnapshot writes EEPROM data to the snapshot buffer.
func (c *Client) WriteSnapshot(data []byte) error {
	return c.SendBinary("/xsfp/sync/start", "/xsfp/sync/data", data)
}
