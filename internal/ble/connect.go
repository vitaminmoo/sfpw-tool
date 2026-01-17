package ble

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/vitaminmoo/sfpw-tool/internal/config"
	"github.com/vitaminmoo/sfpw-tool/internal/protocol"

	"tinygo.org/x/bluetooth"
)

// Connect scans for and connects to the SFP Wizard device
func Connect() bluetooth.Device {
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

		if config.Verbose && name != "" {
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

// SetupAPI discovers services/characteristics and gets device MAC for API calls
func SetupAPI(device bluetooth.Device) *APIContext {
	config.Debugf("Discovering services...")

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

	ctx := &APIContext{}

	// Find write and notify characteristics
	// Write: 9280f26c (handle 0x10)
	// Notify: d587c47f (handle 0x15)
	var infoChar *bluetooth.DeviceCharacteristic
	for i := range chars {
		uuidStr := chars[i].UUID().String()
		config.Debugf("Found characteristic: %s", uuidStr)
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
			var info protocol.DeviceInfo
			if err := json.Unmarshal(buf[:n], &info); err == nil {
				ctx.MAC = strings.ToLower(info.ID)
				config.Debugf("Device MAC: %s", ctx.MAC)
			}
		}
	}

	if ctx.MAC == "" {
		log.Fatal("Could not determine device MAC address")
	}

	return ctx
}
