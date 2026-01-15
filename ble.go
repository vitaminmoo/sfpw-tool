package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"tinygo.org/x/bluetooth"
)

const (
	// SFP Service (from BLE spec) - original service
	SFPServiceUUID         = "8E60F02E-F699-4865-B83F-F40501752184"
	SFPWriteCharUUID       = "9280F26C-A56F-43EA-B769-D5D732E1AC67"
	SFPNotifyCharUUID      = "DC272A22-43F2-416B-8FA5-63A071542FAC"
	SFPSecondaryNotifyUUID = "D587C47F-AC6E-4388-A31C-E6CD380BA043"

	// Secondary service (v1.1.1) - has duplicate characteristic UUIDs
	SFPService2UUID = "0B9676EE-8352-440A-BF80-61541D578FCF"
)

func connectToDevice() bluetooth.Device {
	adapter := bluetooth.DefaultAdapter
	err := adapter.Enable()
	if err != nil {
		log.Fatal("Failed to enable Bluetooth:", err)
	}

	fmt.Println("Scanning for SFP Wizard...")

	var deviceResult bluetooth.ScanResult
	var found bool

	err = adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
		name := result.LocalName()
		nameLower := strings.ToLower(name)

		if verbose && name != "" {
			address, _ := result.Address.MarshalText()
			fmt.Printf("  Found: '%s' (%s)\n", name, string(address))
		}

		if nameLower == "sfp-wizard" || nameLower == "sfp wizard" || strings.Contains(nameLower, "sfp") {
			deviceResult = result
			found = true
			adapter.StopScan()
		}
	})

	if err != nil {
		log.Fatal("Scan error:", err)
	}

	if !found {
		fmt.Println("ERROR: SFP Wizard device not found!")
		os.Exit(1)
	}

	address, _ := deviceResult.Address.MarshalText()
	fmt.Printf("Connecting to %s...\n", string(address))

	device, err := adapter.Connect(deviceResult.Address, bluetooth.ConnectionParams{})
	if err != nil {
		log.Fatal("Failed to connect:", err)
	}

	fmt.Println("Connected!")
	return device
}

// setupAPI discovers services/characteristics and gets device MAC for API calls
func setupAPI(device bluetooth.Device) *APIContext {
	debugf("Discovering services...")

	allServices, err := device.DiscoverServices(nil)
	if err != nil {
		log.Fatal("Failed to discover services:", err)
	}

	// Find primary SFP service
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

	ctx := &APIContext{}

	// Find write and notify characteristics
	// Write: 9280f26c (handle 0x10)
	// Notify: d587c47f (handle 0x15)
	var infoChar *bluetooth.DeviceCharacteristic
	for i := range chars {
		uuidStr := chars[i].UUID().String()
		debugf("Found characteristic: %s", uuidStr)
		if strings.EqualFold(uuidStr, SFPWriteCharUUID) {
			ctx.WriteChar = &chars[i]
		}
		if strings.EqualFold(uuidStr, SFPSecondaryNotifyUUID) {
			ctx.NotifyChar = &chars[i]
		}
		if strings.EqualFold(uuidStr, SFPNotifyCharUUID) {
			infoChar = &chars[i]
		}
	}

	if ctx.WriteChar == nil {
		log.Fatal("Write characteristic not found")
	}
	if ctx.NotifyChar == nil {
		log.Fatal("Notify characteristic (d587c47f) not found")
	}

	// Read device info to get MAC address
	if infoChar != nil {
		buf := make([]byte, 256)
		n, err := infoChar.Read(buf)
		if err == nil && n > 0 {
			var info DeviceInfo
			if err := json.Unmarshal(buf[:n], &info); err == nil {
				ctx.MAC = strings.ToLower(info.ID)
				debugf("Device MAC: %s", ctx.MAC)
			}
		}
	}

	if ctx.MAC == "" {
		log.Fatal("Could not determine device MAC address")
	}

	return ctx
}
