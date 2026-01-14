package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"sfpl-flasher/pb"

	"google.golang.org/protobuf/proto"
	"tinygo.org/x/bluetooth"
)

var verbose bool

//go:generate protoc --go_out=. --go_opt=paths=source_relative pb/comm_v4.proto

const (
	// Protobuf-based firmware update service (reverse engineered from app)
	ServiceUUID    = "fb349212-0000-1000-8000-00805f9b34fb"
	WriteCharUUID  = "fb349213-0000-1000-8000-00805f9b34fb"
	NotifyCharUUID = "fb349214-0000-1000-8000-00805f9b34fb"

	// Text-based SFP EEPROM service (from BLE spec doc)
	SFPServiceUUID         = "8E60F02E-F699-4865-B83F-F40501752184"
	SFPWriteCharUUID       = "9280F26C-A56F-43EA-B769-D5D732E1AC67"
	SFPNotifyCharUUID      = "DC272A22-43F2-416B-8FA5-63A071542FAC"
	SFPSecondaryNotifyUUID = "D587C47F-AC6E-4388-A31C-E6CD380BA043"
)

// FWStatus represents the firmware update status response (protobuf-based)
type FWStatus struct {
	Status    string `json:"status"`
	Offset    int    `json:"offset"`
	ChunkSize int    `json:"size"`
}

// Keep imports available for firmware update skeleton
var (
	_ = json.Marshal
	_ = pb.FromDevice{}
)

// DeviceInfo represents the JSON response from the device (API v1.0+)
type DeviceInfo struct {
	ID         string `json:"id"`
	FWVersion  string `json:"fwv"`
	APIVersion string `json:"apiVersion"`
	Voltage    string `json:"voltage"`
	Level      string `json:"level"`
}

func debugf(format string, args ...interface{}) {
	if verbose {
		fmt.Printf("[DEBUG] "+format+"\n", args...)
	}
}

func main() {
	// Define flags
	fs := flag.NewFlagSet("sfpl-flasher", flag.ContinueOnError)
	fs.BoolVar(&verbose, "verbose", false, "Enable verbose debug output")
	fs.BoolVar(&verbose, "v", false, "Enable verbose debug output (shorthand)")

	// Parse flags from os.Args[1:] if available
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

	// Parse flags up to the command
	if commandIdx > 0 {
		fs.Parse(args[:commandIdx])
	}

	if commandIdx >= len(args) {
		printUsage()
		os.Exit(1)
	}

	command := args[commandIdx]
	commandArgs := args[commandIdx+1:]

	// Connect to device
	device := connectToDevice()
	defer device.Disconnect()

	// Execute command
	switch command {
	case "version":
		cmdVersion(device)
	case "status":
		cmdStatus(device)
	case "read-eeprom":
		cmdReadEEPROM(device)
	case "write-eeprom":
		if len(commandArgs) < 1 {
			log.Fatal("Usage: sfpl-flasher write-eeprom <file.bin>")
		}
		cmdWriteEEPROM(device, commandArgs[0])
	case "erase-eeprom":
		cmdEraseEEPROM(device)
	case "stop":
		cmdStop(device)
	case "firmware-update":
		if len(commandArgs) < 1 {
			log.Fatal("Usage: sfpl-flasher firmware-update <firmware.bin>")
		}
		cmdFirmwareUpdate(device, commandArgs[0])
	case "explore":
		cmdExplore(device)
	case "test-api":
		cmdTestAPI(device)
	default:
		fmt.Printf("Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("SFP Wizard Flasher - BLE Command Tool")
	fmt.Println("\nUsage: sfpl-flasher [flags] <command> [args]")
	fmt.Println("\nFlags:")
	fmt.Println("  -v, --verbose    Enable verbose debug output")
	fmt.Println("\nSFP EEPROM Commands (Text-based BLE API):")
	fmt.Println("  version              Get firmware version")
	fmt.Println("  status               Get device status (battery, SFP presence, etc.)")
	fmt.Println("  read-eeprom          Read SFP module EEPROM and save to eeprom.bin")
	fmt.Println("  write-eeprom <file>  Write binary file to SFP module EEPROM")
	fmt.Println("  erase-eeprom         Erase SFP module EEPROM (WARNING: destructive!)")
	fmt.Println("  stop                 Stop current SFP operation")
	fmt.Println("\nFirmware Commands (Protobuf-based BLE API):")
	fmt.Println("  firmware-update <file>  Update device firmware (NOT YET IMPLEMENTED)")
	fmt.Println("\nDebug Commands:")
	fmt.Println("  explore              Show all BLE services and characteristics")
	fmt.Println("  test-api             Test various API endpoints to see what the device supports")
	fmt.Println("\nExamples:")
	fmt.Println("  sfpl-flasher version")
	fmt.Println("  sfpl-flasher -v status")
	fmt.Println("  sfpl-flasher read-eeprom")
	fmt.Println("  sfpl-flasher write-eeprom backup.bin")
	fmt.Println("  sfpl-flasher --verbose explore")
}

func connectToDevice() bluetooth.Device {
	adapter := bluetooth.DefaultAdapter
	err := adapter.Enable()
	if err != nil {
		log.Fatal("Failed to enable Bluetooth:", err)
	}

	fmt.Println("Scanning for BLE devices...")
	fmt.Println("Looking for: 'SFP-Wizard', 'SFP Wizard', or any device containing 'SFP'")
	fmt.Println("---")

	var deviceResult bluetooth.ScanResult
	var found bool
	deviceCount := 0

	err = adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
		deviceCount++
		name := result.LocalName()
		address, _ := result.Address.MarshalText()

		// Print every device found
		if name != "" {
			fmt.Printf("[%d] Found: '%s' (%s)\n", deviceCount, name, string(address))
		} else {
			fmt.Printf("[%d] Found: <no name> (%s)\n", deviceCount, string(address))
		}

		// Check if it matches our target (case-insensitive)
		nameLower := strings.ToLower(name)
		if nameLower == "sfp-wizard" || nameLower == "sfp wizard" || strings.Contains(nameLower, "sfp") {
			fmt.Printf(">>> MATCH! Connecting to: %s\n", name)
			deviceResult = result
			found = true
			adapter.StopScan()
		}
	})

	if err != nil {
		log.Fatal("Scan error:", err)
	}

	fmt.Println("---")
	fmt.Printf("Scan complete. Found %d devices total.\n", deviceCount)

	if !found {
		fmt.Println("\nERROR: SFP Wizard device not found!")
		fmt.Println("\nTroubleshooting:")
		fmt.Println("  1. Make sure the device is powered on")
		fmt.Println("  2. Check that Bluetooth is enabled on the device")
		fmt.Println("  3. Try power cycling the SFP Wizard")
		fmt.Println("  4. Make sure no other app is connected to it")
		fmt.Println("  5. Check if the device name appears in the list above")
		os.Exit(1)
	}

	address, _ := deviceResult.Address.MarshalText()
	fmt.Printf("\nConnecting to %s...\n", string(address))

	device, err := adapter.Connect(deviceResult.Address, bluetooth.ConnectionParams{})
	if err != nil {
		log.Fatal("Failed to connect:", err)
	}

	fmt.Println("Connected successfully!")
	return device
}

// ============================================================================
// PROTOBUF-BASED FIRMWARE UPDATE FUNCTIONS (for future use)
// ============================================================================

// frame wraps a protobuf message with a 2-byte length header for BLE transmission
func frame(msg proto.Message) ([]byte, error) {
	b, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 2+len(b))
	binary.BigEndian.PutUint16(buf[0:2], uint16(len(b)))
	copy(buf[2:], b)
	return buf, nil
}

// getProtoCharacteristics discovers and returns the protobuf service characteristics
func getProtoCharacteristics(device bluetooth.Device) (writeChar, notifyChar bluetooth.DeviceCharacteristic, err error) {
	protoSvcUUID, err := bluetooth.ParseUUID(ServiceUUID)
	if err != nil {
		return writeChar, notifyChar, fmt.Errorf("failed to parse protobuf service UUID: %v", err)
	}

	srvs, err := device.DiscoverServices([]bluetooth.UUID{protoSvcUUID})
	if err != nil || len(srvs) == 0 {
		return writeChar, notifyChar, fmt.Errorf("protobuf service not found: %v", err)
	}

	protoWriteUUID, _ := bluetooth.ParseUUID(WriteCharUUID)
	protoNotifyUUID, _ := bluetooth.ParseUUID(NotifyCharUUID)

	chars, err := srvs[0].DiscoverCharacteristics([]bluetooth.UUID{
		protoWriteUUID,
		protoNotifyUUID,
	})
	if err != nil {
		return writeChar, notifyChar, fmt.Errorf("failed to discover protobuf characteristics: %v", err)
	}

	for _, c := range chars {
		uuid := c.UUID().String()
		if uuid == WriteCharUUID {
			writeChar = c
		} else if uuid == NotifyCharUUID {
			notifyChar = c
		}
	}

	return writeChar, notifyChar, nil
}

// cmdFirmwareUpdate is a skeleton for firmware update functionality
// TODO: Implement when ready to test firmware updates
func cmdFirmwareUpdate(device bluetooth.Device, firmwareFile string) {
	fmt.Println("WARNING: Firmware update not yet implemented")
	fmt.Println("This is a skeleton function for future development")

	// Example usage (commented out for safety):
	/*
		writeChar, notifyChar, err := getProtoCharacteristics(device)
		if err != nil {
			log.Fatal(err)
		}

		// Setup notification listener
		notifyChar.EnableNotifications(func(data []byte) {
			if len(data) < 2 {
				return
			}

			// Strip 2-byte length header
			protoBytes := data[2:]
			var resp pb.FromDevice
			if err := proto.Unmarshal(protoBytes, &resp); err == nil {
				var status FWStatus
				json.Unmarshal(resp.GetResponse().Payload, &status)
				fmt.Printf("Device Status: %s | Next Offset: %d | Suggested Chunk: %d\n",
					status.Status, status.Offset, status.ChunkSize)
			}
		})

		// Send "Update Start"
		fmt.Println("Starting firmware update process...")
		startPayload, _ := json.Marshal(map[string]int{"size": 1024 * 1024}) // Example size

		msg := &pb.ToDevice{
			RequestId: 1,
			Message: &pb.ToDevice_Request{
				Request: &pb.Request{
					Method:  1, // POST
					Path:    "/api/1.0/mgmt/fw/start",
					Payload: startPayload,
				},
			},
		}

		framed, _ := frame(msg)
		writeChar.WriteWithoutResponse(framed)

		// TODO: Add firmware chunking and upload logic here
	*/
}

// ============================================================================
// TEXT-BASED SFP EEPROM FUNCTIONS
// ============================================================================

// getSFPCharacteristics discovers and returns the SFP service characteristics
func getSFPCharacteristics(device bluetooth.Device) (writeChar, notifyChar, secondaryNotifyChar bluetooth.DeviceCharacteristic, err error) {
	debugf("Discovering services...")

	// First, discover all services to see what's available
	allServices, err := device.DiscoverServices(nil)
	if err != nil {
		return writeChar, notifyChar, secondaryNotifyChar, fmt.Errorf("failed to discover services: %v", err)
	}

	debugf("Found %d services", len(allServices))
	for i, svc := range allServices {
		debugf("  [%d] Service UUID: %s", i+1, svc.UUID().String())
	}

	// Try to find the SFP service (case-insensitive comparison)
	var sfpService *bluetooth.DeviceService
	for i := range allServices {
		if strings.EqualFold(allServices[i].UUID().String(), SFPServiceUUID) {
			sfpService = &allServices[i]
			debugf("✓ Found SFP service: %s", SFPServiceUUID)
			break
		}
	}

	if sfpService == nil {
		return writeChar, notifyChar, secondaryNotifyChar, fmt.Errorf("SFP service (%s) not found on device", SFPServiceUUID)
	}

	// Discover characteristics
	debugf("Discovering characteristics...")
	sfpChars, err := sfpService.DiscoverCharacteristics(nil)
	if err != nil {
		return writeChar, notifyChar, secondaryNotifyChar, fmt.Errorf("failed to discover SFP characteristics: %v", err)
	}

	debugf("Found %d characteristics", len(sfpChars))
	for i, char := range sfpChars {
		debugf("  [%d] Characteristic UUID: %s", i+1, char.UUID().String())
	}

	// Find our specific characteristics (case-insensitive)
	for _, c := range sfpChars {
		uuid := c.UUID().String()
		if strings.EqualFold(uuid, SFPWriteCharUUID) {
			writeChar = c
			debugf("✓ Found Write characteristic")
		} else if strings.EqualFold(uuid, SFPNotifyCharUUID) {
			notifyChar = c
			debugf("✓ Found Primary Notify characteristic")
		} else if strings.EqualFold(uuid, SFPSecondaryNotifyUUID) {
			secondaryNotifyChar = c
			debugf("✓ Found Secondary Notify characteristic")
		}
	}

	// Verify all were found
	var missing []string
	if writeChar.UUID().String() == "00000000-0000-0000-0000-000000000000" {
		missing = append(missing, "Write")
	}
	if notifyChar.UUID().String() == "00000000-0000-0000-0000-000000000000" {
		missing = append(missing, "Notify")
	}
	if secondaryNotifyChar.UUID().String() == "00000000-0000-0000-0000-000000000000" {
		missing = append(missing, "Secondary Notify")
	}

	if len(missing) > 0 {
		return writeChar, notifyChar, secondaryNotifyChar, fmt.Errorf("missing required characteristics: %v", missing)
	}

	debugf("✓ All required characteristics found")
	return writeChar, notifyChar, secondaryNotifyChar, nil
}

func cmdVersion(device bluetooth.Device) {
	_, notifyChar, secondaryNotifyChar, err := getSFPCharacteristics(device)
	if err != nil {
		log.Fatal(err)
	}

	responseChan := make(chan string, 10)

	debugf("Enabling primary notifications...")
	err = notifyChar.EnableNotifications(func(data []byte) {
		debugf("[PRIMARY] Received %d bytes: %X", len(data), data)
		if isTextData(data) {
			text := string(data)
			debugf("[PRIMARY TEXT] %s", text)
			responseChan <- text
		} else {
			debugf("[PRIMARY BINARY] Not text data")
		}
	})
	if err != nil {
		log.Fatal("Failed to enable primary notifications:", err)
	}
	debugf("✓ Primary notifications enabled")

	debugf("Enabling secondary notifications...")
	err = secondaryNotifyChar.EnableNotifications(func(data []byte) {
		debugf("[SECONDARY] Received %d bytes: %X", len(data), data)
		if isTextData(data) {
			text := string(data)
			debugf("[SECONDARY TEXT] %s", text)
			responseChan <- text
		} else {
			debugf("[SECONDARY BINARY] Not text data")
		}
	})
	if err != nil {
		log.Fatal("Failed to enable secondary notifications:", err)
	}
	debugf("✓ Secondary notifications enabled")

	// Trigger device response by reading from notify characteristic
	debugf("Reading from notify characteristic to trigger device response...")
	buf := make([]byte, 512)
	_, _ = notifyChar.Read(buf) // Ignore errors, this is just to trigger

	// Give BLE stack time to deliver notification
	time.Sleep(300 * time.Millisecond)

	// Check if we got a response
	select {
	case response := <-responseChan:
		// Parse JSON response
		var info DeviceInfo
		if err := json.Unmarshal([]byte(response), &info); err == nil {
			// Pretty print the info
			fmt.Printf("Device ID:       %s\n", info.ID)
			fmt.Printf("Firmware:        v%s\n", info.FWVersion)
			fmt.Printf("API Version:     %s\n", info.APIVersion)

			// Parse voltage (in millivolts to volts)
			if info.Voltage != "" {
				fmt.Printf("Battery Voltage: %s mV\n", info.Voltage)
			}
			if info.Level != "" {
				fmt.Printf("Battery Level:   %s%%\n", info.Level)
			}
		} else {
			// Fallback to raw output if not JSON
			fmt.Println(response)
		}
	case <-time.After(3 * time.Second):
		log.Fatal("Timeout waiting for device response")
	}
}

func cmdStatus(device bluetooth.Device) {
	writeChar, notifyChar, secondaryNotifyChar, err := getSFPCharacteristics(device)
	if err != nil {
		log.Fatal(err)
	}

	responseChan := make(chan string, 10)

	debugf("Enabling primary notifications...")
	err = notifyChar.EnableNotifications(func(data []byte) {
		debugf("[PRIMARY] Received %d bytes: %X", len(data), data)
		if isTextData(data) {
			text := string(data)
			debugf("[PRIMARY TEXT] %s", text)
			responseChan <- text
		}
	})
	if err != nil {
		log.Fatal("Failed to enable notifications:", err)
	}

	debugf("Enabling secondary notifications...")
	err = secondaryNotifyChar.EnableNotifications(func(data []byte) {
		debugf("[SECONDARY] Received %d bytes: %X", len(data), data)
		if isTextData(data) {
			text := string(data)
			debugf("[SECONDARY TEXT] %s", text)
			responseChan <- text
		}
	})
	if err != nil {
		log.Fatal("Failed to enable secondary notifications:", err)
	}

	// Trigger device response by reading
	debugf("Reading to trigger device response...")
	buf := make([]byte, 512)
	_, _ = notifyChar.Read(buf)

	time.Sleep(300 * time.Millisecond)

	// Try sending status command
	debugf("Sending status command...")
	sendTextCommand(writeChar, "[GET] /stats")

	select {
	case response := <-responseChan:
		// The device returns JSON with device info (same as version)
		var info DeviceInfo
		if err := json.Unmarshal([]byte(response), &info); err == nil {
			fmt.Printf("Device ID:       %s\n", info.ID)
			fmt.Printf("Firmware:        v%s\n", info.FWVersion)
			fmt.Printf("API Version:     %s\n", info.APIVersion)
			if info.Voltage != "" {
				fmt.Printf("Battery Voltage: %s mV\n", info.Voltage)
			}
			if info.Level != "" {
				fmt.Printf("Battery Level:   %s%%\n", info.Level)
			}
		} else {
			// Fallback to raw output if not JSON
			fmt.Println(response)
		}
	case <-time.After(3 * time.Second):
		log.Fatal("Timeout waiting for status response")
	}
}

func cmdReadEEPROM(device bluetooth.Device) {
	writeChar, notifyChar, secondaryNotifyChar, err := getSFPCharacteristics(device)
	if err != nil {
		log.Fatal(err)
	}

	var eepromData []byte
	ackReceived := false
	dataChan := make(chan bool, 1)

	debugf("Enabling primary notifications...")
	err = notifyChar.EnableNotifications(func(data []byte) {
		if isTextData(data) {
			msg := string(data)
			debugf("[PRIMARY TEXT] %s", msg)

			// Skip initial device info JSON
			if strings.Contains(msg, `"id"`) && strings.Contains(msg, `"fwv"`) {
				debugf("Skipping device info JSON")
				return
			}

			fmt.Println("Device:", msg)
			if strings.Contains(strings.ToLower(msg), "sif") && strings.Contains(strings.ToLower(msg), "start") {
				ackReceived = true
				debugf("Acknowledgment received!")
			}
		} else {
			debugf("[PRIMARY BINARY] Received %d bytes", len(data))
			// Binary EEPROM data
			if ackReceived {
				eepromData = append(eepromData, data...)
				fmt.Printf("Received %d bytes (total: %d bytes)\n", len(data), len(eepromData))
				// Signal we got data
				select {
				case dataChan <- true:
				default:
				}
			} else {
				debugf("Ignoring binary data - no ack yet")
			}
		}
	})
	if err != nil {
		log.Fatal("Failed to enable primary notifications:", err)
	}

	debugf("Enabling secondary notifications...")
	err = secondaryNotifyChar.EnableNotifications(func(data []byte) {
		if isTextData(data) {
			msg := string(data)
			debugf("[SECONDARY TEXT] %s", msg)

			// Skip initial device info JSON
			if strings.Contains(msg, `"id"`) && strings.Contains(msg, `"fwv"`) {
				debugf("Skipping device info JSON")
				return
			}

			if strings.Contains(strings.ToLower(msg), "sif") && strings.Contains(strings.ToLower(msg), "start") {
				ackReceived = true
				debugf("Acknowledgment received!")
			}
		} else {
			debugf("[SECONDARY BINARY] Received %d bytes", len(data))
			if ackReceived {
				eepromData = append(eepromData, data...)
				fmt.Printf("Received %d bytes (total: %d bytes)\n", len(data), len(eepromData))
				select {
				case dataChan <- true:
				default:
				}
			} else {
				debugf("Ignoring binary data - no ack yet")
			}
		}
	})
	if err != nil {
		log.Fatal("Failed to enable secondary notifications:", err)
	}

	// Trigger device by reading
	debugf("Reading to trigger device...")
	buf := make([]byte, 512)
	_, _ = notifyChar.Read(buf)

	time.Sleep(300 * time.Millisecond)

	fmt.Println("Starting EEPROM read...")
	sendTextCommand(writeChar, "[POST] /sif/start")

	// Wait for acknowledgment
	time.Sleep(500 * time.Millisecond)

	// Wait for binary data
	select {
	case <-dataChan:
		time.Sleep(1 * time.Second) // Give time for all chunks
	case <-time.After(5 * time.Second):
		log.Fatal("Timeout waiting for EEPROM data")
	}

	if len(eepromData) == 0 {
		log.Fatal("No EEPROM data received")
	}

	filename := "eeprom.bin"
	err = os.WriteFile(filename, eepromData, 0644)
	if err != nil {
		log.Fatal("Failed to write EEPROM file:", err)
	}

	fmt.Printf("\nEEPROM data saved to %s (%d bytes)\n", filename, len(eepromData))
}

func cmdWriteEEPROM(device bluetooth.Device, filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		log.Fatal("Failed to read file:", err)
	}

	fmt.Printf("Loaded %d bytes from %s\n", len(data), filename)

	writeChar, notifyChar, secondaryNotifyChar, err := getSFPCharacteristics(device)
	if err != nil {
		log.Fatal(err)
	}

	statusChan := make(chan string, 10)

	debugf("Enabling primary notifications...")
	err = notifyChar.EnableNotifications(func(data []byte) {
		if isTextData(data) {
			msg := string(data)
			debugf("[PRIMARY TEXT] %s", msg)
			statusChan <- msg
		}
	})
	if err != nil {
		log.Fatal("Failed to enable primary notifications:", err)
	}

	debugf("Enabling secondary notifications...")
	err = secondaryNotifyChar.EnableNotifications(func(data []byte) {
		if isTextData(data) {
			msg := string(data)
			debugf("[SECONDARY TEXT] %s", msg)
			statusChan <- msg
		}
	})
	if err != nil {
		log.Fatal("Failed to enable secondary notifications:", err)
	}

	// Trigger device
	debugf("Reading to trigger device...")
	buf := make([]byte, 512)
	_, _ = notifyChar.Read(buf)
	time.Sleep(300 * time.Millisecond)

	fmt.Println("Initiating write mode...")
	sendTextCommand(writeChar, "[POST] /sif/write")

	// Wait for "SIF write start"
	select {
	case msg := <-statusChan:
		fmt.Println("Device:", msg)
		if !strings.Contains(msg, "SIF write start") {
			log.Fatal("Unexpected response:", msg)
		}
	case <-time.After(3 * time.Second):
		log.Fatal("Timeout waiting for write acknowledgment")
	}

	// Send data in chunks
	chunkSize := 20 // Conservative for BLE compatibility
	totalChunks := (len(data) + chunkSize - 1) / chunkSize

	fmt.Printf("Writing EEPROM in %d chunks of %d bytes...\n", totalChunks, chunkSize)

	for i := 0; i < len(data); i += chunkSize {
		end := i + chunkSize
		if end > len(data) {
			end = len(data)
		}
		chunk := data[i:end]

		_, err := writeChar.WriteWithoutResponse(chunk)
		if err != nil {
			log.Fatal("Failed to write chunk:", err)
		}

		chunkNum := i/chunkSize + 1
		if chunkNum%10 == 0 || chunkNum == totalChunks {
			fmt.Printf("  Progress: %d/%d chunks\n", chunkNum, totalChunks)
		}

		time.Sleep(10 * time.Millisecond) // Small delay between chunks
	}

	// Wait for completion
	fmt.Println("Waiting for write completion...")
	select {
	case msg := <-statusChan:
		fmt.Println("Device:", msg)
	case <-time.After(5 * time.Second):
		fmt.Println("Warning: Timeout waiting for completion message")
	}

	fmt.Println("Write complete!")
}

func cmdEraseEEPROM(device bluetooth.Device) {
	writeChar, notifyChar, secondaryNotifyChar, err := getSFPCharacteristics(device)
	if err != nil {
		log.Fatal(err)
	}

	statusChan := make(chan string, 10)

	debugf("Enabling primary notifications...")
	err = notifyChar.EnableNotifications(func(data []byte) {
		if isTextData(data) {
			msg := string(data)
			debugf("[PRIMARY TEXT] %s", msg)
			statusChan <- msg
		}
	})
	if err != nil {
		log.Fatal("Failed to enable primary notifications:", err)
	}

	debugf("Enabling secondary notifications...")
	err = secondaryNotifyChar.EnableNotifications(func(data []byte) {
		if isTextData(data) {
			msg := string(data)
			debugf("[SECONDARY TEXT] %s", msg)
			statusChan <- msg
		}
	})
	if err != nil {
		log.Fatal("Failed to enable secondary notifications:", err)
	}

	// Trigger device
	debugf("Reading to trigger device...")
	buf := make([]byte, 512)
	_, _ = notifyChar.Read(buf)
	time.Sleep(300 * time.Millisecond)

	fmt.Println("WARNING: This will erase the SFP module EEPROM!")
	fmt.Println("Starting erase...")
	sendTextCommand(writeChar, "[POST] /sif/erase")

	// Wait for start message
	select {
	case msg := <-statusChan:
		fmt.Println("Device:", msg)
	case <-time.After(3 * time.Second):
		log.Fatal("Timeout waiting for erase start")
	}

	// Wait for completion
	select {
	case msg := <-statusChan:
		fmt.Println("Device:", msg)
	case <-time.After(10 * time.Second):
		log.Fatal("Timeout waiting for erase completion")
	}

	fmt.Println("Erase complete!")
}

func cmdStop(device bluetooth.Device) {
	writeChar, notifyChar, secondaryNotifyChar, err := getSFPCharacteristics(device)
	if err != nil {
		log.Fatal(err)
	}

	responseChan := make(chan string, 10)

	debugf("Enabling primary notifications...")
	err = notifyChar.EnableNotifications(func(data []byte) {
		if isTextData(data) {
			msg := string(data)
			debugf("[PRIMARY TEXT] %s", msg)
			responseChan <- msg
		}
	})
	if err != nil {
		log.Fatal("Failed to enable primary notifications:", err)
	}

	debugf("Enabling secondary notifications...")
	err = secondaryNotifyChar.EnableNotifications(func(data []byte) {
		if isTextData(data) {
			msg := string(data)
			debugf("[SECONDARY TEXT] %s", msg)
			responseChan <- msg
		}
	})
	if err != nil {
		log.Fatal("Failed to enable secondary notifications:", err)
	}

	// Trigger device
	debugf("Reading to trigger device...")
	buf := make([]byte, 512)
	_, _ = notifyChar.Read(buf)
	time.Sleep(300 * time.Millisecond)

	sendTextCommand(writeChar, "[POST] /sif/stop")

	select {
	case response := <-responseChan:
		fmt.Println(response)
	case <-time.After(3 * time.Second):
		fmt.Println("Stop command sent (no response)")
	}
}

func sendTextCommand(char bluetooth.DeviceCharacteristic, command string) {
	data := []byte(command)
	_, err := char.WriteWithoutResponse(data)
	if err != nil {
		log.Printf("Failed to write command '%s': %v", command, err)
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

func cmdTestAPI(device bluetooth.Device) {
	writeChar, notifyChar, secondaryNotifyChar, err := getSFPCharacteristics(device)
	if err != nil {
		log.Fatal(err)
	}

	responseChan := make(chan string, 10)
	allResponses := []string{}

	debugf("Enabling notifications...")
	notifyChar.EnableNotifications(func(data []byte) {
		if isTextData(data) {
			text := string(data)
			responseChan <- text
		}
	})
	secondaryNotifyChar.EnableNotifications(func(data []byte) {
		if isTextData(data) {
			text := string(data)
			responseChan <- text
		}
	})

	// Trigger device and wait for initial JSON
	buf := make([]byte, 512)
	notifyChar.Read(buf)

	// Wait for and drain the initial device info JSON
	fmt.Println("Waiting for initial device info...")
	select {
	case response := <-responseChan:
		fmt.Printf("Got initial response: %s\n", response[:min(50, len(response))])
	case <-time.After(1 * time.Second):
		fmt.Println("No initial response")
	}

	time.Sleep(500 * time.Millisecond)

	// Try various API endpoints (test both with and without initial read)
	testCommands := []string{
		"/api/1.0/version",
		"[GET] /stats",
		"[GET] /api/1.0/sfp/status",
		"[POST] /api/1.0/sfp/read",
		"[GET] /sfp/status",
		"[POST] /sfp/read",
		"[POST] /sif/start",
		"/api/1.0/sif/start",
		"[GET] /api/1.0/sfp",
		"[POST] /api/1.0/sfp/eeprom/read",
		"READ",
		"STATUS",
	}

	fmt.Println("Testing API endpoints...")
	fmt.Println("========================")

	for _, cmd := range testCommands {
		fmt.Printf("\nTesting: %s\n", cmd)
		sendTextCommand(writeChar, cmd)

		select {
		case response := <-responseChan:
			if !strings.Contains(response, `"id"`) || !strings.Contains(response, `"fwv"`) {
				fmt.Printf("  ✓ Response: %s\n", response)
				allResponses = append(allResponses, fmt.Sprintf("%s -> %s", cmd, response))
			} else {
				fmt.Println("  • Device info (same as version)")
			}
		case <-time.After(1 * time.Second):
			fmt.Println("  ✗ No response")
		}
	}

	if len(allResponses) > 0 {
		fmt.Println("\n\nSuccessful Commands:")
		fmt.Println("====================")
		for _, r := range allResponses {
			fmt.Println(r)
		}
	}
}

func cmdExplore(device bluetooth.Device) {
	fmt.Println("Exploring all services and characteristics...")

	allServices, err := device.DiscoverServices(nil)
	if err != nil {
		log.Fatal("Failed to discover services:", err)
	}

	fmt.Printf("\nFound %d services:\n\n", len(allServices))

	for i, svc := range allServices {
		fmt.Printf("Service #%d: %s\n", i+1, svc.UUID().String())

		chars, err := svc.DiscoverCharacteristics(nil)
		if err != nil {
			fmt.Printf("  Error discovering characteristics: %v\n\n", err)
			continue
		}

		fmt.Printf("  Characteristics (%d):\n", len(chars))
		for j, char := range chars {
			fmt.Printf("    [%d] %s\n", j+1, char.UUID().String())
		}
		fmt.Println()
	}
}

func parseAndPrintStatus(status string) {
	// Parse the status string to extract key information
	// Example: "sysmon: ver:1.0.10, bat:[x]|^|35%, sfp:[x], ..."

	fmt.Println("\nParsed Status:")

	if strings.Contains(status, "ver:") {
		start := strings.Index(status, "ver:") + 4
		end := strings.Index(status[start:], ",")
		if end > 0 {
			version := strings.TrimSpace(status[start : start+end])
			fmt.Printf("  Firmware Version: %s\n", version)
		}
	}

	if strings.Contains(status, "bat:") {
		start := strings.Index(status, "bat:") + 4
		end := strings.Index(status[start:], ",")
		if end > 0 {
			battery := strings.TrimSpace(status[start : start+end])
			fmt.Printf("  Battery: %s\n", battery)
		}
	}

	if strings.Contains(status, "sfp:[x]") {
		fmt.Println("  SFP Module: Present")
	} else if strings.Contains(status, "sfp:[ ]") {
		fmt.Println("  SFP Module: Not Present")
	}

	if strings.Contains(status, "ble:[x]") {
		fmt.Println("  Bluetooth: Enabled")
	}

	if strings.Contains(status, "mac:") {
		start := strings.Index(status, "mac:") + 4
		end := start + 17 // MAC address length
		if end <= len(status) {
			mac := status[start:end]
			fmt.Printf("  MAC Address: %s\n", mac)
		}
	}
}
