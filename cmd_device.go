package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"tinygo.org/x/bluetooth"
)

// cmdVersion reads device info by reading from the notify characteristic.
// This is safe and doesn't require writing any commands.
func cmdVersion(device bluetooth.Device) {
	debugf("Discovering services...")

	allServices, err := device.DiscoverServices(nil)
	if err != nil {
		log.Fatal("Failed to discover services:", err)
	}

	// Find SFP service
	var sfpService *bluetooth.DeviceService
	for i := range allServices {
		if strings.EqualFold(allServices[i].UUID().String(), SFPServiceUUID) {
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
		if strings.EqualFold(chars[i].UUID().String(), SFPNotifyCharUUID) {
			notifyChar = &chars[i]
			break
		}
	}

	if notifyChar == nil {
		log.Fatal("Notify characteristic not found")
	}

	// Read device info directly from characteristic
	debugf("Reading device info...")
	buf := make([]byte, 256)
	n, err := notifyChar.Read(buf)
	if err != nil {
		log.Fatal("Failed to read:", err)
	}

	if n == 0 {
		log.Fatal("No data received")
	}

	data := buf[:n]
	debugf("Received %d bytes: %s", n, string(data))

	// Parse JSON
	var info DeviceInfo
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

// cmdExplore lists all services and characteristics.
// This is safe and doesn't write anything.
func cmdExplore(device bluetooth.Device) {
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
				if isTextData(data) {
					fmt.Printf("      Value: %s\n", string(data))
				} else {
					fmt.Printf("      Value: %X\n", data)
				}
			}
		}
		fmt.Println()
	}
}

// cmdAPIVersion tests the API protocol by calling /api/version
func cmdAPIVersion(device bluetooth.Device) {
	debugf("Discovering services...")

	allServices, err := device.DiscoverServices(nil)
	if err != nil {
		log.Fatal("Failed to discover services:", err)
	}

	// Find primary SFP service (where the API lives)
	var sfpService *bluetooth.DeviceService
	for i := range allServices {
		uuidStr := allServices[i].UUID().String()
		if strings.EqualFold(uuidStr, SFPServiceUUID) {
			sfpService = &allServices[i]
			debugf("Found SFP service: %s", uuidStr)
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
		debugf("Found characteristic: %s", uuidStr)
		if strings.EqualFold(uuidStr, SFPWriteCharUUID) {
			writeChar = &chars[i]
		}
		// Response notifications come on d587c47f, not dc272a22
		if strings.EqualFold(uuidStr, SFPSecondaryNotifyUUID) {
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
	debugf("Writing to 9280f26c, subscribing to d587c47f")

	// Send GET request to /api/version
	resp, body, err := sendAPIRequest(writeChar, notifyChar, "GET", "/api/version", nil)
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

// cmdStats gets device statistics (battery, signal, uptime)
func cmdStats(device bluetooth.Device) {
	ctx := setupAPI(device)

	resp, body, err := sendAPIRequest(ctx.WriteChar, ctx.NotifyChar, "GET", ctx.apiPath("/stats"), nil)
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
	fmt.Printf("Uptime:       %d seconds\n", stats.Uptime)
	fmt.Printf("Signal:       %d dBm\n", stats.SignalDbm)
}

// cmdInfo gets device info via API
func cmdInfo(device bluetooth.Device) {
	ctx := setupAPI(device)

	resp, body, err := sendAPIRequest(ctx.WriteChar, ctx.NotifyChar, "GET", ctx.apiPath(""), nil)
	if err != nil {
		log.Fatal("API request failed:", err)
	}

	if resp.StatusCode != 200 {
		fmt.Printf("Error: status %d\n", resp.StatusCode)
		fmt.Printf("Body: %s\n", string(body))
		return
	}

	// Print raw JSON for now - structure may vary
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, body, "", "  "); err != nil {
		fmt.Printf("Body: %s\n", string(body))
	} else {
		fmt.Println(prettyJSON.String())
	}
}

// cmdSettings gets device settings
func cmdSettings(device bluetooth.Device) {
	ctx := setupAPI(device)

	resp, body, err := sendAPIRequest(ctx.WriteChar, ctx.NotifyChar, "GET", ctx.apiPath("/settings"), nil)
	if err != nil {
		log.Fatal("API request failed:", err)
	}

	if resp.StatusCode != 200 {
		fmt.Printf("Error: status %d\n", resp.StatusCode)
		fmt.Printf("Body: %s\n", string(body))
		return
	}

	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, body, "", "  "); err != nil {
		fmt.Printf("Body: %s\n", string(body))
	} else {
		fmt.Println(prettyJSON.String())
	}
}

// cmdBluetooth gets bluetooth parameters
func cmdBluetooth(device bluetooth.Device) {
	ctx := setupAPI(device)

	resp, body, err := sendAPIRequest(ctx.WriteChar, ctx.NotifyChar, "GET", ctx.apiPath("/bt"), nil)
	if err != nil {
		log.Fatal("API request failed:", err)
	}

	if resp.StatusCode != 200 {
		fmt.Printf("Error: status %d\n", resp.StatusCode)
		fmt.Printf("Body: %s\n", string(body))
		return
	}

	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, body, "", "  "); err != nil {
		fmt.Printf("Body: %s\n", string(body))
	} else {
		fmt.Println(prettyJSON.String())
	}
}

// cmdFirmware gets firmware status
func cmdFirmware(device bluetooth.Device) {
	ctx := setupAPI(device)

	resp, body, err := sendAPIRequest(ctx.WriteChar, ctx.NotifyChar, "GET", ctx.apiPath("/fw"), nil)
	if err != nil {
		log.Fatal("API request failed:", err)
	}

	if resp.StatusCode != 200 {
		fmt.Printf("Error: status %d\n", resp.StatusCode)
		fmt.Printf("Body: %s\n", string(body))
		return
	}

	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, body, "", "  "); err != nil {
		fmt.Printf("Body: %s\n", string(body))
	} else {
		fmt.Println(prettyJSON.String())
	}
}

// cmdReboot reboots the device
func cmdReboot(device bluetooth.Device) {
	ctx := setupAPI(device)

	fmt.Println("Rebooting device...")

	resp, body, err := ctx.sendRequest("POST", ctx.apiPath("/reboot"), nil, 10*1000000000)
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
