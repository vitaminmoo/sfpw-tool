package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

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
	case "api-version":
		// Get firmware/API version via API
		device := connectToDevice()
		defer device.Disconnect()
		cmdAPIVersion(device)
	case "stats":
		// Get device statistics (battery, signal, uptime)
		device := connectToDevice()
		defer device.Disconnect()
		cmdStats(device)
	case "info":
		// Get device info via API
		device := connectToDevice()
		defer device.Disconnect()
		cmdInfo(device)
	case "settings":
		// Get device settings
		device := connectToDevice()
		defer device.Disconnect()
		cmdSettings(device)
	case "bt":
		// Get bluetooth parameters
		device := connectToDevice()
		defer device.Disconnect()
		cmdBluetooth(device)
	case "fw":
		// Get firmware status
		device := connectToDevice()
		defer device.Disconnect()
		cmdFirmware(device)
	case "support-dump":
		// Dump support info archive (syslog, module database)
		device := connectToDevice()
		defer device.Disconnect()
		cmdSupportDump(device)
	case "logs":
		// Show device syslog
		device := connectToDevice()
		defer device.Disconnect()
		cmdLogs(device)
	case "reboot":
		// Reboot the device
		device := connectToDevice()
		defer device.Disconnect()
		cmdReboot(device)
	case "module-info":
		// Get current module details
		device := connectToDevice()
		defer device.Disconnect()
		cmdModuleInfo(device)
	case "module-read":
		// Read EEPROM from physical module
		if commandIdx+1 >= len(args) {
			fmt.Println("Usage: sfpl-flasher module-read <output.bin>")
			fmt.Println("  Reads the physical SFP module EEPROM and saves to file")
			os.Exit(1)
		}
		device := connectToDevice()
		defer device.Disconnect()
		cmdModuleRead(device, args[commandIdx+1])
	case "snapshot-info":
		// Get snapshot buffer info
		device := connectToDevice()
		defer device.Disconnect()
		cmdSnapshotInfo(device)
	case "snapshot-read":
		// Read snapshot buffer data
		if commandIdx+1 >= len(args) {
			fmt.Println("Usage: sfpl-flasher snapshot-read <output.bin>")
			fmt.Println("  Reads the snapshot buffer and saves to file")
			os.Exit(1)
		}
		device := connectToDevice()
		defer device.Disconnect()
		cmdSnapshotRead(device, args[commandIdx+1])
	case "snapshot-write":
		// Write EEPROM data to snapshot buffer
		if commandIdx+1 >= len(args) {
			fmt.Println("Usage: sfpl-flasher snapshot-write <eeprom.bin>")
			fmt.Println("  Writes a 512-byte (SFP) or 640-byte (QSFP) EEPROM dump to the snapshot")
			fmt.Println("  Use the device screen to apply snapshot to physical module")
			os.Exit(1)
		}
		device := connectToDevice()
		defer device.Disconnect()
		cmdSnapshotWrite(device, args[commandIdx+1])
	case "parse-eeprom":
		// Parse and display SFP EEPROM data from a file (no device connection)
		if commandIdx+1 >= len(args) {
			fmt.Println("Usage: sfpl-flasher parse-eeprom <eeprom.bin>")
			fmt.Println("  Parses a 512-byte (SFP) or 640-byte (QSFP) EEPROM dump and displays info")
			os.Exit(1)
		}
		cmdParseEEPROM(args[commandIdx+1])
	case "fw-update":
		// Update device firmware from file
		if commandIdx+1 >= len(args) {
			fmt.Println("Usage: sfpl-flasher fw-update <firmware.bin>")
			fmt.Println("  Upload and install firmware update from file")
			os.Exit(1)
		}
		device := connectToDevice()
		defer device.Disconnect()
		cmdFirmwareUpdate(device, args[commandIdx+1])
	case "fw-abort":
		// Abort an in-progress firmware update
		device := connectToDevice()
		defer device.Disconnect()
		cmdFirmwareAbort(device)
	case "fw-status":
		// Get detailed firmware status
		device := connectToDevice()
		defer device.Disconnect()
		cmdFirmwareStatus(device)
	case "test-encode":
		// Test encoding without connecting - for debugging protocol
		cmdTestEncode()
	case "test-packets":
		// Test decoding packets from packets.csv
		if commandIdx+1 >= len(args) {
			fmt.Println("Usage: sfpl-flasher test-packets <file.csv>")
			os.Exit(1)
		}
		cmdTestPackets(args[commandIdx+1])
	default:
		fmt.Printf("Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("SFP Wizard Flasher - BLE Command Tool")
	fmt.Println()
	fmt.Println("Usage: sfpl-flasher [flags] <command>")
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
	fmt.Println("  sfpl-flasher version")
	fmt.Println("  sfpl-flasher -v api-version")
}
