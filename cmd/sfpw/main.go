package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"sfpw-tool/internal/ble"
	"sfpw-tool/internal/commands"
	"sfpw-tool/internal/config"
)

func main() {
	fs := flag.NewFlagSet("sfpw-tool", flag.ContinueOnError)
	fs.BoolVar(&config.Verbose, "verbose", false, "Enable verbose debug output")
	fs.BoolVar(&config.Verbose, "v", false, "Enable verbose debug output (shorthand)")

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
		device := ble.Connect()
		defer device.Disconnect()
		commands.Version(device)
	case "explore":
		// Safe: only discovers services, no writes
		device := ble.Connect()
		defer device.Disconnect()
		commands.Explore(device)
	case "api-version":
		// Get firmware/API version via API
		device := ble.Connect()
		defer device.Disconnect()
		commands.APIVersion(device)
	case "stats":
		// Get device statistics (battery, signal, uptime)
		device := ble.Connect()
		defer device.Disconnect()
		commands.Stats(device)
	case "info":
		// Get device info via API
		device := ble.Connect()
		defer device.Disconnect()
		commands.Info(device)
	case "settings":
		// Get device settings
		device := ble.Connect()
		defer device.Disconnect()
		commands.Settings(device)
	case "bt":
		// Get bluetooth parameters
		device := ble.Connect()
		defer device.Disconnect()
		commands.Bluetooth(device)
	case "fw":
		// Get firmware status
		device := ble.Connect()
		defer device.Disconnect()
		commands.Firmware(device)
	case "support-dump":
		// Dump support info archive (syslog, module database)
		device := ble.Connect()
		defer device.Disconnect()
		commands.SupportDump(device)
	case "logs":
		// Show device syslog
		device := ble.Connect()
		defer device.Disconnect()
		commands.Logs(device)
	case "reboot":
		// Reboot the device
		device := ble.Connect()
		defer device.Disconnect()
		commands.Reboot(device)
	case "module-info":
		// Get current module details
		device := ble.Connect()
		defer device.Disconnect()
		commands.ModuleInfo(device)
	case "module-read":
		// Read EEPROM from physical module
		if commandIdx+1 >= len(args) {
			fmt.Println("Usage: sfpw-tool module-read <output.bin>")
			fmt.Println("  Reads the physical SFP module EEPROM and saves to file")
			os.Exit(1)
		}
		device := ble.Connect()
		defer device.Disconnect()
		commands.ModuleRead(device, args[commandIdx+1])
	case "snapshot-info":
		// Get snapshot buffer info
		device := ble.Connect()
		defer device.Disconnect()
		commands.SnapshotInfo(device)
	case "snapshot-read":
		// Read snapshot buffer data
		if commandIdx+1 >= len(args) {
			fmt.Println("Usage: sfpw-tool snapshot-read <output.bin>")
			fmt.Println("  Reads the snapshot buffer and saves to file")
			os.Exit(1)
		}
		device := ble.Connect()
		defer device.Disconnect()
		commands.SnapshotRead(device, args[commandIdx+1])
	case "snapshot-write":
		// Write EEPROM data to snapshot buffer
		if commandIdx+1 >= len(args) {
			fmt.Println("Usage: sfpw-tool snapshot-write <eeprom.bin>")
			fmt.Println("  Writes a 512-byte (SFP) or 640-byte (QSFP) EEPROM dump to the snapshot")
			fmt.Println("  Use the device screen to apply snapshot to physical module")
			os.Exit(1)
		}
		device := ble.Connect()
		defer device.Disconnect()
		commands.SnapshotWrite(device, args[commandIdx+1])
	case "parse-eeprom":
		// Parse and display SFP EEPROM data from a file (no device connection)
		if commandIdx+1 >= len(args) {
			fmt.Println("Usage: sfpw-tool parse-eeprom <eeprom.bin>")
			fmt.Println("  Parses a 512-byte (SFP) or 640-byte (QSFP) EEPROM dump and displays info")
			os.Exit(1)
		}
		commands.ParseEEPROM(args[commandIdx+1])
	case "fw-update":
		// Update device firmware from file
		if commandIdx+1 >= len(args) {
			fmt.Println("Usage: sfpw-tool fw-update <firmware.bin>")
			fmt.Println("  Upload and install firmware update from file")
			os.Exit(1)
		}
		device := ble.Connect()
		defer device.Disconnect()
		commands.FirmwareUpdate(device, args[commandIdx+1])
	case "fw-abort":
		// Abort an in-progress firmware update
		device := ble.Connect()
		defer device.Disconnect()
		commands.FirmwareAbort(device)
	case "fw-status":
		// Get detailed firmware status
		device := ble.Connect()
		defer device.Disconnect()
		commands.FirmwareStatusCmd(device)
	case "test-encode":
		// Test encoding without connecting - for debugging protocol
		commands.TestEncode()
	case "test-packets":
		// Test decoding packets from packets.csv
		if commandIdx+1 >= len(args) {
			fmt.Println("Usage: sfpw-tool test-packets <file.csv>")
			os.Exit(1)
		}
		commands.TestPackets(args[commandIdx+1])
	default:
		fmt.Printf("Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("SFP Wizard Flasher - BLE Command Tool")
	fmt.Println()
	fmt.Println("Usage: sfpw-tool [flags] <command>")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -v, --verbose    Enable verbose debug output")
	fmt.Println()
	fmt.Println("Device info:")
	fmt.Println("  version           Read device info from BLE characteristic")
	fmt.Println("  api-version       Get firmware/API version via API")
	fmt.Println("  info              Get device info via API")
	fmt.Println("  stats             Get device statistics (battery, signal, uptime)")
	fmt.Println("  settings          Get device settings")
	fmt.Println("  bt                Get bluetooth parameters")
	fmt.Println("  fw                Get firmware status")
	fmt.Println()
	fmt.Println("Module operations:")
	fmt.Println("  module-info       Get details about the inserted SFP module")
	fmt.Println("  module-read FILE  Read EEPROM from physical module to file")
	fmt.Println()
	fmt.Println("Snapshot operations:")
	fmt.Println("  snapshot-info       Get snapshot buffer status")
	fmt.Println("  snapshot-read FILE  Read snapshot buffer to file")
	fmt.Println("  snapshot-write FILE Write EEPROM file to snapshot buffer")
	fmt.Println("                      (use device screen to apply to module)")
	fmt.Println()
	fmt.Println("Firmware operations:")
	fmt.Println("  fw-update FILE    Upload and install firmware from file")
	fmt.Println("  fw-status         Get detailed firmware update status")
	fmt.Println("  fw-abort          Abort an in-progress firmware update")
	fmt.Println()
	fmt.Println("Other:")
	fmt.Println("  logs              Show device syslog")
	fmt.Println("  support-dump      Download support info archive (syslog, module DB)")
	fmt.Println("  reboot            Reboot the device")
	fmt.Println("  explore           List all BLE services and characteristics")
	fmt.Println()
	fmt.Println("Offline tools:")
	fmt.Println("  parse-eeprom FILE Parse and display SFP/QSFP EEPROM data from file")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  sfpw-tool version")
	fmt.Println("  sfpw-tool -v api-version")
}
