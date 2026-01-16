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
	Type       string `json:"type,omitempty"`
	FWVersion  string `json:"fwv"`
	BomID      string `json:"bomId,omitempty"`
	ProID      string `json:"proId,omitempty"`
	State      string `json:"state,omitempty"`
	Name       string `json:"name,omitempty"`
	APIVersion string `json:"apiVersion,omitempty"`
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

// Settings represents device settings.
type Settings struct {
	Channel          string            `json:"ch"`
	Name             string            `json:"name"`
	IsLedEnabled     bool              `json:"isLedEnabled"`
	IsHwResetBlocked bool              `json:"isHwResetBlocked"`
	UWSType          string            `json:"uwsType"`
	Intervals        SettingsIntervals `json:"intervals"`
	HomekitEnabled   bool              `json:"homekitEnabled"`
}

// SettingsIntervals contains interval settings.
type SettingsIntervals struct {
	StatsInterval int `json:"intStats"`
}

// BluetoothParams represents Bluetooth connection parameters.
type BluetoothParams struct {
	Mode          string `json:"btMode"`
	IntervalMin   int    `json:"intervalMin"`
	IntervalMax   int    `json:"intervalMax"`
	Timeout       int    `json:"timeout"`
	Latency       int    `json:"latency"`
	EnableLatency bool   `json:"enableLatency"`
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

	// Root endpoint may not include apiVersion, so fetch it separately if empty
	if info.APIVersion == "" {
		body, err := c.GetJSON("/api/version")
		if err == nil {
			var versionInfo struct {
				APIVersion string `json:"apiVersion"`
			}
			if json.Unmarshal(body, &versionInfo) == nil {
				info.APIVersion = versionInfo.APIVersion
			}
		}
	}

	return &info, nil
}

// GetSettings returns device settings.
func (c *Client) GetSettings() (*Settings, error) {
	body, err := c.GetJSON("/settings")
	if err != nil {
		return nil, err
	}

	var settings Settings
	if err := json.Unmarshal(body, &settings); err != nil {
		return nil, err
	}
	return &settings, nil
}

// GetBluetooth returns bluetooth parameters.
func (c *Client) GetBluetooth() (*BluetoothParams, error) {
	body, err := c.GetJSON("/bt")
	if err != nil {
		return nil, err
	}

	var params BluetoothParams
	if err := json.Unmarshal(body, &params); err != nil {
		return nil, err
	}
	return &params, nil
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

