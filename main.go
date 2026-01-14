package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"sfpl-flasher/pb"

	"google.golang.org/protobuf/proto"
	"tinygo.org/x/bluetooth"
)

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

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

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
		if len(os.Args) < 3 {
			log.Fatal("Usage: sfpl-flasher write-eeprom <file.bin>")
		}
		cmdWriteEEPROM(device, os.Args[2])
	case "erase-eeprom":
		cmdEraseEEPROM(device)
	case "stop":
		cmdStop(device)
	case "firmware-update":
		if len(os.Args) < 3 {
			log.Fatal("Usage: sfpl-flasher firmware-update <firmware.bin>")
		}
		cmdFirmwareUpdate(device, os.Args[2])
	default:
		fmt.Printf("Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("SFP Wizard Flasher - BLE Command Tool")
	fmt.Println("\nUsage: sfpl-flasher <command> [args]")
	fmt.Println("\nSFP EEPROM Commands (Text-based BLE API):")
	fmt.Println("  version              Get firmware version")
	fmt.Println("  status               Get device status (battery, SFP presence, etc.)")
	fmt.Println("  read-eeprom          Read SFP module EEPROM and save to eeprom.bin")
	fmt.Println("  write-eeprom <file>  Write binary file to SFP module EEPROM")
	fmt.Println("  erase-eeprom         Erase SFP module EEPROM (WARNING: destructive!)")
	fmt.Println("  stop                 Stop current SFP operation")
	fmt.Println("\nFirmware Commands (Protobuf-based BLE API):")
	fmt.Println("  firmware-update <file>  Update device firmware (NOT YET IMPLEMENTED)")
	fmt.Println("\nExamples:")
	fmt.Println("  sfpl-flasher version")
	fmt.Println("  sfpl-flasher status")
	fmt.Println("  sfpl-flasher read-eeprom")
	fmt.Println("  sfpl-flasher write-eeprom backup.bin")
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
		if name == "SFP-Wizard" || name == "SFP Wizard" || strings.Contains(name, "SFP") {
			deviceResult = result
			found = true
			adapter.StopScan()
		}
	})

	if !found {
		log.Fatal("SFP Wizard device not found")
	}

	address, _ := deviceResult.Address.MarshalText()
	fmt.Printf("Found device: %s\n", string(address))
	fmt.Println("Connecting...")

	device, err := adapter.Connect(deviceResult.Address, bluetooth.ConnectionParams{})
	if err != nil {
		log.Fatal("Failed to connect:", err)
	}

	fmt.Println("Connected!")
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
func getSFPCharacteristics(device bluetooth.Device) (writeChar, notifyChar bluetooth.DeviceCharacteristic, err error) {
	sfpSvcUUID, err := bluetooth.ParseUUID(SFPServiceUUID)
	if err != nil {
		return writeChar, notifyChar, fmt.Errorf("failed to parse SFP service UUID: %v", err)
	}

	sfpSrvs, err := device.DiscoverServices([]bluetooth.UUID{sfpSvcUUID})
	if err != nil || len(sfpSrvs) == 0 {
		return writeChar, notifyChar, fmt.Errorf("SFP service not found: %v", err)
	}

	sfpWriteUUID, _ := bluetooth.ParseUUID(SFPWriteCharUUID)
	sfpNotifyUUID, _ := bluetooth.ParseUUID(SFPNotifyCharUUID)

	sfpChars, err := sfpSrvs[0].DiscoverCharacteristics([]bluetooth.UUID{
		sfpWriteUUID,
		sfpNotifyUUID,
	})
	if err != nil {
		return writeChar, notifyChar, fmt.Errorf("failed to discover SFP characteristics: %v", err)
	}

	for _, c := range sfpChars {
		uuid := c.UUID().String()
		if uuid == SFPWriteCharUUID {
			writeChar = c
		} else if uuid == SFPNotifyCharUUID {
			notifyChar = c
		}
	}

	return writeChar, notifyChar, nil
}

func cmdVersion(device bluetooth.Device) {
	writeChar, notifyChar, err := getSFPCharacteristics(device)
	if err != nil {
		log.Fatal(err)
	}

	responseChan := make(chan string, 1)

	err = notifyChar.EnableNotifications(func(data []byte) {
		if isTextData(data) {
			responseChan <- string(data)
		}
	})
	if err != nil {
		log.Fatal("Failed to enable notifications:", err)
	}

	sendTextCommand(writeChar, "/api/1.0/version")

	select {
	case response := <-responseChan:
		fmt.Println(response)
	case <-time.After(3 * time.Second):
		log.Fatal("Timeout waiting for version response")
	}
}

func cmdStatus(device bluetooth.Device) {
	writeChar, notifyChar, err := getSFPCharacteristics(device)
	if err != nil {
		log.Fatal(err)
	}

	responseChan := make(chan string, 1)

	err = notifyChar.EnableNotifications(func(data []byte) {
		if isTextData(data) {
			responseChan <- string(data)
		}
	})
	if err != nil {
		log.Fatal("Failed to enable notifications:", err)
	}

	sendTextCommand(writeChar, "[GET] /stats")

	select {
	case response := <-responseChan:
		fmt.Println(response)
		parseAndPrintStatus(response)
	case <-time.After(3 * time.Second):
		log.Fatal("Timeout waiting for status response")
	}
}

func cmdReadEEPROM(device bluetooth.Device) {
	writeChar, notifyChar, err := getSFPCharacteristics(device)
	if err != nil {
		log.Fatal(err)
	}

	var eepromData []byte
	ackReceived := false
	dataChan := make(chan bool, 1)

	err = notifyChar.EnableNotifications(func(data []byte) {
		if isTextData(data) {
			msg := string(data)
			fmt.Println("Device:", msg)
			if strings.Contains(msg, "SIF start") {
				ackReceived = true
			}
		} else {
			// Binary EEPROM data
			if ackReceived {
				eepromData = append(eepromData, data...)
				fmt.Printf("Received %d bytes (total: %d bytes)\n", len(data), len(eepromData))
				// Signal we got data
				select {
				case dataChan <- true:
				default:
				}
			}
		}
	})
	if err != nil {
		log.Fatal("Failed to enable notifications:", err)
	}

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

	writeChar, notifyChar, err := getSFPCharacteristics(device)
	if err != nil {
		log.Fatal(err)
	}

	statusChan := make(chan string, 10)

	err = notifyChar.EnableNotifications(func(data []byte) {
		if isTextData(data) {
			msg := string(data)
			statusChan <- msg
		}
	})
	if err != nil {
		log.Fatal("Failed to enable notifications:", err)
	}

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
	writeChar, notifyChar, err := getSFPCharacteristics(device)
	if err != nil {
		log.Fatal(err)
	}

	statusChan := make(chan string, 10)

	err = notifyChar.EnableNotifications(func(data []byte) {
		if isTextData(data) {
			statusChan <- string(data)
		}
	})
	if err != nil {
		log.Fatal("Failed to enable notifications:", err)
	}

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
	writeChar, notifyChar, err := getSFPCharacteristics(device)
	if err != nil {
		log.Fatal(err)
	}

	responseChan := make(chan string, 1)

	err = notifyChar.EnableNotifications(func(data []byte) {
		if isTextData(data) {
			responseChan <- string(data)
		}
	})
	if err != nil {
		log.Fatal("Failed to enable notifications:", err)
	}

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
