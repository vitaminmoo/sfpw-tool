package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"tinygo.org/x/bluetooth"
)

var verbose bool

const (
	// SFP Service (from BLE spec)
	SFPServiceUUID         = "8E60F02E-F699-4865-B83F-F40501752184"
	SFPWriteCharUUID       = "9280F26C-A56F-43EA-B769-D5D732E1AC67"
	SFPNotifyCharUUID      = "DC272A22-43F2-416B-8FA5-63A071542FAC"
	SFPSecondaryNotifyUUID = "D587C47F-AC6E-4388-A31C-E6CD380BA043"
)

// DeviceInfo represents the JSON response from the device
type DeviceInfo struct {
	ID         string `json:"id"`
	FWVersion  string `json:"fwv"`
	APIVersion string `json:"apiVersion"`
	Voltage    string `json:"voltage"`
	Level      string `json:"level"`
}

func debugf(format string, args ...any) {
	if verbose {
		fmt.Printf("[DEBUG] "+format+"\n", args...)
	}
}

func main() {
	fs := flag.NewFlagSet("sfpl-flasher", flag.ContinueOnError)
	fs.BoolVar(&verbose, "verbose", false, "Enable verbose debug output")
	fs.BoolVar(&verbose, "v", false, "Enable verbose debug output (shorthand)")

	args := os.Args[1:]
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	// Find where the command is (skip flags)
	commandIdx := 0
	for i, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			commandIdx = i
			break
		}
	}

	if commandIdx > 0 {
		fs.Parse(args[:commandIdx])
	}

	if commandIdx >= len(args) {
		printUsage()
		os.Exit(1)
	}

	command := args[commandIdx]

	switch command {
	case "version":
		// Safe: only reads from characteristic, no writes
		device := connectToDevice()
		defer device.Disconnect()
		cmdVersion(device)
	case "explore":
		// Safe: only discovers services, no writes
		device := connectToDevice()
		defer device.Disconnect()
		cmdExplore(device)
	default:
		fmt.Printf("Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("SFP Wizard Flasher - BLE Command Tool")
	fmt.Println()
	fmt.Println("WARNING: This tool is under development. The device may enter")
	fmt.Println("a bad state requiring reboot after some operations.")
	fmt.Println()
	fmt.Println("Usage: sfpl-flasher [flags] <command>")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -v, --verbose    Enable verbose debug output")
	fmt.Println()
	fmt.Println("Safe Commands (read-only):")
	fmt.Println("  version          Read device info (firmware version, battery, etc.)")
	fmt.Println("  explore          List all BLE services and characteristics")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  sfpl-flasher version")
	fmt.Println("  sfpl-flasher -v explore")
}

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

func isTextData(data []byte) bool {
	for _, b := range data {
		if b < 32 && b != 9 && b != 10 && b != 13 || b > 126 {
			return false
		}
	}
	return true
}
