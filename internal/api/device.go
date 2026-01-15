package api

import (
	"encoding/json"
)

// Stats represents device statistics.
type Stats struct {
	Battery      int     `json:"battery"`
	BatteryV     float64 `json:"batteryV"`
	IsLowBattery bool    `json:"isLowBattery"`
	Uptime       int     `json:"uptime"`
	SignalDbm    int     `json:"signalDbm"`
}

// DeviceInfo represents the device info response.
type DeviceInfo struct {
	ID         string `json:"id"`
	FWVersion  string `json:"fwv"`
	APIVersion string `json:"apiVersion"`
	HWVersion  int    `json:"hwv,omitempty"`
}

// FirmwareStatus represents firmware update status.
type FirmwareStatus struct {
	HWVersion       int    `json:"hwv"`
	FWVersion       string `json:"fwv"`
	IsUpdating      bool   `json:"isUPdating"` // Note: typo in API
	Status          string `json:"status"`
	ProgressPercent int    `json:"progressPercent"`
	RemainingTime   int    `json:"remainingTime"`
}

// GetStats returns device statistics.
func (c *Client) GetStats() (*Stats, error) {
	body, err := c.GetJSON("/stats")
	if err != nil {
		return nil, err
	}

	var stats Stats
	if err := json.Unmarshal(body, &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

// GetDeviceInfo returns device information.
func (c *Client) GetDeviceInfo() (*DeviceInfo, error) {
	body, err := c.GetJSON("")
	if err != nil {
		return nil, err
	}

	var info DeviceInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// GetSettings returns device settings as raw JSON.
func (c *Client) GetSettings() (json.RawMessage, error) {
	return c.GetJSON("/settings")
}

// GetBluetooth returns bluetooth parameters as raw JSON.
func (c *Client) GetBluetooth() (json.RawMessage, error) {
	return c.GetJSON("/bt")
}

// GetFirmwareStatus returns firmware status.
func (c *Client) GetFirmwareStatus() (*FirmwareStatus, error) {
	body, err := c.GetJSON("/fw")
	if err != nil {
		return nil, err
	}

	var status FirmwareStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// Reboot reboots the device.
func (c *Client) Reboot() error {
	_, err := c.PostJSON("/reboot", nil)
	// Connection may drop during reboot - that's expected
	// So we only return error if it's not a timeout/connection issue
	return err
}
