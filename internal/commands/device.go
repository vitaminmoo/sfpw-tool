package commands

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/vitaminmoo/sfpw-tool/internal/api"
	"github.com/vitaminmoo/sfpw-tool/internal/ble"
	"github.com/vitaminmoo/sfpw-tool/internal/config"
	"github.com/vitaminmoo/sfpw-tool/internal/protocol"
	"github.com/vitaminmoo/sfpw-tool/internal/util"

	"tinygo.org/x/bluetooth"
)

// Version reads device info by reading from the notify characteristic.
// This is safe and doesn't require writing any commands.
func Version(device bluetooth.Device) {
	config.Debugf("Discovering services...")

	allServices, err := device.DiscoverServices(nil)
	if err != nil {
		log.Fatal("Failed to discover services:", err)
	}

	// Find SFP service
	var sfpService *bluetooth.DeviceService
	for i := range allServices {
		if strings.EqualFold(allServices[i].UUID().String(), ble.SFPServiceUUID) {
			sfpService = &allServices[i]
			break
		}
	}

	if sfpService == nil {
		log.Fatal("SFP service not found")
	}

	// Discover characteristics
	chars, err := sfpService.DiscoverCharacteristics(nil)
	if err != nil {
		log.Fatal("Failed to discover characteristics:", err)
	}

	// Find notify characteristic
	var notifyChar *bluetooth.DeviceCharacteristic
	for i := range chars {
		if strings.EqualFold(chars[i].UUID().String(), ble.SFPNotifyCharUUID) {
			notifyChar = &chars[i]
			break
		}
	}

	if notifyChar == nil {
		log.Fatal("Notify characteristic not found")
	}

	// Read device info directly from characteristic
	config.Debugf("Reading device info...")
	buf := make([]byte, 256)
	n, err := notifyChar.Read(buf)
	if err != nil {
		log.Fatal("Failed to read:", err)
	}

	if n == 0 {
		log.Fatal("No data received")
	}

	data := buf[:n]
	config.Debugf("Received %d bytes: %s", n, string(data))

	// Parse JSON
	var info protocol.DeviceInfo
	if err := json.Unmarshal(data, &info); err != nil {
		// Not JSON, print raw
		fmt.Println(string(data))
		return
	}

	fmt.Printf("Device ID:       %s\n", info.ID)
	fmt.Printf("Firmware:        v%s\n", info.FWVersion)
	fmt.Printf("API Version:     %s\n", info.APIVersion)
	if info.Voltage != "" {
		fmt.Printf("Battery Voltage: %s mV\n", info.Voltage)
	}
	if info.Level != "" {
		fmt.Printf("Battery Level:   %s%%\n", info.Level)
	}
}

// Explore lists all services and characteristics.
// This is safe and doesn't write anything.
func Explore(device bluetooth.Device) {
	fmt.Println("Discovering services...")

	allServices, err := device.DiscoverServices(nil)
	if err != nil {
		log.Fatal("Failed to discover services:", err)
	}

	fmt.Printf("\nFound %d services:\n\n", len(allServices))

	for i, svc := range allServices {
		fmt.Printf("Service #%d: %s\n", i+1, svc.UUID().String())

		chars, err := svc.DiscoverCharacteristics(nil)
		if err != nil {
			fmt.Printf("  Error: %v\n\n", err)
			continue
		}

		for j, char := range chars {
			fmt.Printf("  [%d] %s\n", j+1, char.UUID().String())

			// Try to read (safe operation)
			buf := make([]byte, 256)
			n, err := char.Read(buf)
			if err == nil && n > 0 {
				data := buf[:n]
				if util.IsTextData(data) {
					fmt.Printf("      Value: %s\n", string(data))
				} else {
					fmt.Printf("      Value: %X\n", data)
				}
			}
		}
		fmt.Println()
	}
}

// APIVersion tests the API protocol by calling /api/version
func APIVersion(device bluetooth.Device) {
	config.Debugf("Discovering services...")

	allServices, err := device.DiscoverServices(nil)
	if err != nil {
		log.Fatal("Failed to discover services:", err)
	}

	// Find primary SFP service (where the API lives)
	var sfpService *bluetooth.DeviceService
	for i := range allServices {
		uuidStr := allServices[i].UUID().String()
		if strings.EqualFold(uuidStr, ble.SFPServiceUUID) {
			sfpService = &allServices[i]
			config.Debugf("Found SFP service: %s", uuidStr)
			break
		}
	}

	if sfpService == nil {
		log.Fatal("SFP service not found")
	}

	// Discover characteristics
	chars, err := sfpService.DiscoverCharacteristics(nil)
	if err != nil {
		log.Fatal("Failed to discover characteristics:", err)
	}

	// Find write and notify characteristics
	// Write: 9280f26c (handle 0x10)
	// Notify: d587c47f (handle 0x15) - NOT dc272a22!
	var writeChar, notifyChar *bluetooth.DeviceCharacteristic
	for i := range chars {
		uuidStr := chars[i].UUID().String()
		config.Debugf("Found characteristic: %s", uuidStr)
		if strings.EqualFold(uuidStr, ble.SFPWriteCharUUID) {
			writeChar = &chars[i]
		}
		// Response notifications come on d587c47f, not dc272a22
		if strings.EqualFold(uuidStr, ble.SFPSecondaryNotifyUUID) {
			notifyChar = &chars[i]
		}
	}

	if writeChar == nil {
		log.Fatal("Write characteristic not found")
	}
	if notifyChar == nil {
		log.Fatal("Notify characteristic (d587c47f) not found")
	}

	fmt.Println("Testing API protocol with /api/version...")
	config.Debugf("Writing to 9280f26c, subscribing to d587c47f")

	// Send GET request to /api/version
	resp, body, err := ble.SendAPIRequest(writeChar, notifyChar, "GET", "/api/version", nil)
	if err != nil {
		log.Fatal("API request failed:", err)
	}

	fmt.Printf("Response status: %d\n", resp.StatusCode)

	if resp.StatusCode == 200 {
		// Parse the body from the body section (not the envelope)
		var versionInfo struct {
			FWVersion  string `json:"fwv"`
			APIVersion string `json:"apiVersion"`
		}
		if err := json.Unmarshal(body, &versionInfo); err != nil {
			fmt.Printf("Body (raw): %s\n", string(body))
		} else {
			fmt.Printf("Firmware:    v%s\n", versionInfo.FWVersion)
			fmt.Printf("API Version: %s\n", versionInfo.APIVersion)
		}
	} else {
		fmt.Printf("Body: %s\n", string(body))
	}
}

// Stats gets device statistics (battery, signal, uptime)
func Stats(device bluetooth.Device) {
	ctx := ble.SetupAPI(device)

	resp, body, err := ble.SendAPIRequest(ctx.WriteChar, ctx.NotifyChar, "GET", ctx.APIPath("/stats"), nil)
	if err != nil {
		log.Fatal("API request failed:", err)
	}

	if resp.StatusCode != 200 {
		fmt.Printf("Error: status %d\n", resp.StatusCode)
		fmt.Printf("Body: %s\n", string(body))
		return
	}

	var stats struct {
		Battery      int     `json:"battery"`
		BatteryV     float64 `json:"batteryV"`
		IsLowBattery bool    `json:"isLowBattery"`
		Uptime       int     `json:"uptime"`
		SignalDbm    int     `json:"signalDbm"`
	}
	if err := json.Unmarshal(body, &stats); err != nil {
		fmt.Printf("Body (raw): %s\n", string(body))
		return
	}

	fmt.Printf("Battery:      %d%% (%.3fV)\n", stats.Battery, stats.BatteryV)
	fmt.Printf("Low Battery:  %v\n", stats.IsLowBattery)
	fmt.Printf("Uptime:       %s\n", formatUptime(stats.Uptime))
	fmt.Printf("Signal:       %d dBm\n", stats.SignalDbm)
}

// formatUptime converts milliseconds to a human-readable format.
func formatUptime(ms int) string {
	seconds := ms / 1000
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm %ds", seconds/60, seconds%60)
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	if hours < 24 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	days := hours / 24
	hours = hours % 24
	return fmt.Sprintf("%dd %dh", days, hours)
}

// Info gets device info via API
func Info(device bluetooth.Device) {
	GetAndDisplayJSON(device, "")
}

// Settings gets device settings
func Settings(device bluetooth.Device) {
	GetAndDisplayJSON(device, "/settings")
}

// Bluetooth gets bluetooth parameters
func Bluetooth(device bluetooth.Device) {
	GetAndDisplayJSON(device, "/bt")
}

// Firmware gets firmware status
func Firmware(device bluetooth.Device) {
	GetAndDisplayJSON(device, "/fw")
}

// Reboot reboots the device
func Reboot(device bluetooth.Device) {
	ctx := ble.SetupAPI(device)

	fmt.Println("Rebooting device...")

	resp, body, err := ctx.SendRequest("POST", ctx.APIPath("/reboot"), nil, 10*1000000000)
	if err != nil {
		// Connection may drop during reboot - that's expected
		fmt.Println("Reboot command sent (connection lost - this is normal)")
		return
	}

	if resp.StatusCode == 200 {
		fmt.Println("Reboot initiated")
	} else {
		fmt.Printf("Reboot failed: status %d\n", resp.StatusCode)
		fmt.Printf("Body: %s\n", string(body))
	}
}

// DumpAll dumps all read-only API endpoints as raw JSON for archival/debugging.
func DumpAll(device bluetooth.Device) {
	client := api.New(device)
	if err := client.Connect(); err != nil {
		log.Fatal("Failed to connect:", err)
	}

	// Define all read-only endpoints to dump
	endpoints := []struct {
		name     string
		endpoint string
	}{
		{"info", ""},
		{"api_version", "/api/version"},
		{"stats", "/stats"},
		{"settings", "/settings"},
		{"bluetooth", "/bt"},
		{"firmware", "/fw"},
		{"module_details", "/xsfp/module/details"},
		{"snapshot_info", "/xsfp/sync/start"},
	}

	// Collect all responses into a map
	result := make(map[string]json.RawMessage)

	for _, ep := range endpoints {
		config.Debugf("--- Fetching %s (%s) ---", ep.name, ep.endpoint)
		body, err := client.GetJSON(ep.endpoint)
		if err != nil {
			config.Debugf("Error: %s", err.Error())
			// Store error as string value
			errMsg := fmt.Sprintf("error: %s", err.Error())
			result[ep.name] = json.RawMessage(fmt.Sprintf("%q", errMsg))
			continue
		}
		config.Debugf("Raw JSON for %s: %s", ep.name, string(body))
		result[ep.name] = body
	}

	// Output as formatted JSON
	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatal("Failed to marshal JSON:", err)
	}
	fmt.Println(string(output))
}
