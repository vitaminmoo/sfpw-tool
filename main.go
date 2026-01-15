package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tinygo.org/x/bluetooth"
)

var verbose bool

const (
	// SFP Service (from BLE spec) - original service
	SFPServiceUUID         = "8E60F02E-F699-4865-B83F-F40501752184"
	SFPWriteCharUUID       = "9280F26C-A56F-43EA-B769-D5D732E1AC67"
	SFPNotifyCharUUID      = "DC272A22-43F2-416B-8FA5-63A071542FAC"
	SFPSecondaryNotifyUUID = "D587C47F-AC6E-4388-A31C-E6CD380BA043"

	// Secondary service (v1.1.1) - has duplicate characteristic UUIDs
	SFPService2UUID = "0B9676EE-8352-440A-BF80-61541D578FCF"
)

// DeviceInfo represents the JSON response from the device
type DeviceInfo struct {
	ID         string `json:"id"`
	FWVersion  string `json:"fwv"`
	APIVersion string `json:"apiVersion"`
	Voltage    string `json:"voltage"`
	Level      string `json:"level"`
}

// APIContext holds the BLE characteristics needed for API communication
type APIContext struct {
	WriteChar  *bluetooth.DeviceCharacteristic
	NotifyChar *bluetooth.DeviceCharacteristic
	MAC        string // lowercase, no separators (e.g., "deadbeefcafe")

	// For handling responses
	responseMu     sync.Mutex
	responseBuf    bytes.Buffer
	expectedLen    int
	responseChan   chan bool
	notifyEnabled  bool
}

// requestCounter is used to generate incrementing request IDs
var requestCounter uint64

// APIRequest is the JSON envelope for API requests
// The firmware requires "type": "httpRequest" to route to the API handler
type APIRequest struct {
	Type      string   `json:"type"`
	ID        string   `json:"id"`
	Timestamp int64    `json:"timestamp"`
	Method    string   `json:"method"` // HTTP method: GET or POST
	Path      string   `json:"path"`   // API endpoint path
	Headers   struct{} `json:"headers"`
}

// nextRequestID returns the next incrementing request ID in UUID format and sequence number
func nextRequestID() (string, uint16) {
	id := atomic.AddUint64(&requestCounter, 1)
	return fmt.Sprintf("00000000-0000-0000-0000-%012d", id), uint16(id)
}

// APIResponse is the JSON envelope for API responses
// The firmware sends "type": "httpResponse" for API responses
type APIResponse struct {
	Type       string          `json:"type"`
	ID         string          `json:"id"`
	Timestamp  int64           `json:"timestamp"`
	StatusCode int             `json:"statusCode"`
	Headers    struct{}        `json:"headers"`
	Body       json.RawMessage `json:"body"`
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
	fmt.Println("Other:")
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

// zlibCompress compresses data using zlib
func zlibCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, err := w.Write(data)
	if err != nil {
		return nil, err
	}
	err = w.Close()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// zlibDecompress decompresses zlib data
func zlibDecompress(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// binmeEncode wraps JSON data in the binme binary envelope format with zlib compression.
// Format:
//
//	[Outer Header - 4 bytes]
//	  bytes 0-1: total message length (big-endian)
//	  bytes 2-3: flags (00 03 for requests)
//	[Header Section - 9 bytes + zlib data]
//	  byte 0: marker (0x03 = header section)
//	  byte 1: format (0x01 = JSON)
//	  byte 2: compression (0x01 = zlib)
//	  byte 3: flags (0x01)
//	  bytes 4-7: decompressed length (big-endian)
//	  byte 8: compressed length (for short messages)
//	  bytes 9+: zlib compressed JSON
//	[Body Section - 8 bytes + zlib data]
//	  byte 0: marker (0x02 = body section)
//	  byte 1: format (0x01 = JSON)
//	  byte 2: compression (0x01 = zlib)
//	  byte 3: reserved (0x00)
//	  bytes 4-7: compressed length (big-endian)
//	  bytes 8+: zlib compressed body
func binmeEncode(jsonData []byte, bodyData []byte, seqNum uint16) ([]byte, error) {
	// Compress header JSON
	compressedHeader, err := zlibCompress(jsonData)
	if err != nil {
		return nil, fmt.Errorf("failed to compress header: %w", err)
	}

	// Compress body
	compressedBody, err := zlibCompress(bodyData)
	if err != nil {
		return nil, fmt.Errorf("failed to compress body: %w", err)
	}

	// Build the message
	var buf bytes.Buffer

	// We'll write the content first, then prepend the outer header

	// Header section: 9 bytes header + compressed data
	headerSection := make([]byte, 9+len(compressedHeader))
	headerSection[0] = 0x03 // marker: header section
	headerSection[1] = 0x01 // format: JSON
	headerSection[2] = 0x01 // compression: zlib
	headerSection[3] = 0x01 // flags
	// bytes 4-7: always 00 00 00 00 in captured traffic
	headerSection[4] = 0x00
	headerSection[5] = 0x00
	headerSection[6] = 0x00
	headerSection[7] = 0x00
	// Compressed length (single byte)
	headerSection[8] = byte(len(compressedHeader))
	copy(headerSection[9:], compressedHeader)

	// Body section: 8 bytes header + compressed data
	bodySection := make([]byte, 8+len(compressedBody))
	bodySection[0] = 0x02 // marker: body section
	bodySection[1] = 0x01 // format: JSON
	bodySection[2] = 0x01 // compression: zlib
	bodySection[3] = 0x00 // reserved
	// Compressed length (big-endian)
	binary.BigEndian.PutUint32(bodySection[4:8], uint32(len(compressedBody)))
	copy(bodySection[8:], compressedBody)

	// Total message length (excluding outer header)
	totalLen := len(headerSection) + len(bodySection)

	// Write outer header
	outerHeader := make([]byte, 4)
	binary.BigEndian.PutUint16(outerHeader[0:2], uint16(totalLen+4)) // total including header
	binary.BigEndian.PutUint16(outerHeader[2:4], seqNum)             // sequence number matches request ID

	buf.Write(outerHeader)
	buf.Write(headerSection)
	buf.Write(bodySection)

	return buf.Bytes(), nil
}

// binmeDecode extracts JSON data from a binme binary envelope with zlib decompression.
// Returns the header JSON and body data.
func binmeDecode(data []byte) (headerJSON []byte, bodyData []byte, err error) {
	if len(data) < 4 {
		return nil, nil, fmt.Errorf("binme data too short: %d bytes", len(data))
	}

	// Skip outer header (4 bytes)
	// totalLen := binary.BigEndian.Uint16(data[0:2])
	// flags := binary.BigEndian.Uint16(data[2:4])
	pos := 4

	if len(data) < pos+9 {
		return nil, nil, fmt.Errorf("binme data too short for header section")
	}

	// Parse header section
	headerMarker := data[pos]
	if headerMarker != 0x03 {
		return nil, nil, fmt.Errorf("expected header marker 0x03, got 0x%02x", headerMarker)
	}
	// headerFormat := data[pos+1]
	headerCompressed := data[pos+2]
	// headerFlags := data[pos+3]
	// decompressedLen := binary.BigEndian.Uint32(data[pos+4 : pos+8])
	compressedHeaderLen := int(data[pos+8])

	pos += 9
	if len(data) < pos+compressedHeaderLen {
		return nil, nil, fmt.Errorf("binme header data truncated")
	}

	compressedHeader := data[pos : pos+compressedHeaderLen]
	pos += compressedHeaderLen

	// Decompress header if needed - check for zlib magic byte (0x78)
	// Response may have compression=01 but actually send raw JSON
	// Zlib headers: 78 01 (none), 78 5e (fast), 78 9c (default), 78 da (best)
	if headerCompressed == 0x01 && len(compressedHeader) >= 2 && compressedHeader[0] == 0x78 {
		headerJSON, err = zlibDecompress(compressedHeader)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decompress header: %w", err)
		}
	} else {
		// Raw data (not actually compressed despite flag)
		headerJSON = compressedHeader
	}

	// Parse body section if present
	if len(data) < pos+8 {
		// No body section
		return headerJSON, nil, nil
	}

	bodyMarker := data[pos]
	if bodyMarker != 0x02 {
		return nil, nil, fmt.Errorf("expected body marker 0x02, got 0x%02x", bodyMarker)
	}
	// bodyFormat := data[pos+1]
	bodyCompressed := data[pos+2]
	// bodyReserved := data[pos+3]
	compressedBodyLen := int(binary.BigEndian.Uint32(data[pos+4 : pos+8]))

	pos += 8
	if len(data) < pos+compressedBodyLen {
		return nil, nil, fmt.Errorf("binme body data truncated")
	}

	compressedBody := data[pos : pos+compressedBodyLen]

	// Decompress body if needed - check for zlib magic byte (0x78)
	// Zlib headers: 78 01 (none), 78 5e (fast), 78 9c (default), 78 da (best)
	if bodyCompressed == 0x01 && compressedBodyLen >= 2 && compressedBody[0] == 0x78 {
		bodyData, err = zlibDecompress(compressedBody)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decompress body: %w", err)
		}
	} else {
		bodyData = compressedBody
	}

	return headerJSON, bodyData, nil
}

// responseData holds the parsed response envelope and body
type responseData struct {
	envelope APIResponse
	body     []byte
}

// sendAPIRequest sends a JSON API request and waits for the response.
// The request is wrapped in the binme binary envelope format with zlib compression.
// method is the HTTP method (GET or POST), path is the API endpoint.
func sendAPIRequest(writeChar, notifyChar *bluetooth.DeviceCharacteristic, method, path string, body []byte) (*APIResponse, []byte, error) {
	requestID, seqNum := nextRequestID()

	// Channel to receive the response
	responseChan := make(chan responseData, 1)
	var mu sync.Mutex

	// Enable notifications to receive response
	debugf("Enabling notifications on characteristic...")
	err := notifyChar.EnableNotifications(func(buf []byte) {
		debugf("Notification received: %d bytes", len(buf))
		debugf("Raw hex: %X", buf)

		// Decode binme envelope
		headerJSON, bodyData, err := binmeDecode(buf)
		if err != nil {
			debugf("Failed to decode binme: %v", err)
			return
		}

		debugf("Decoded binme header JSON: %s", string(headerJSON))
		if len(bodyData) > 0 {
			if isTextData(bodyData) {
				debugf("Decoded binme body: %s", string(bodyData))
			} else {
				debugf("Decoded binme body hex: %X", bodyData)
			}
		}

		// Parse as API response
		var resp APIResponse
		if err := json.Unmarshal(headerJSON, &resp); err != nil {
			debugf("Failed to parse as APIResponse: %v", err)
			return
		}

		debugf("Parsed response: type=%s, id=%s, status=%d", resp.Type, resp.ID, resp.StatusCode)

		// Check if this is our response
		if resp.ID == requestID {
			mu.Lock()
			select {
			case responseChan <- responseData{envelope: resp, body: bodyData}:
			default:
			}
			mu.Unlock()
		} else {
			debugf("Response ID mismatch: got %s, want %s", resp.ID, requestID)
		}
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to enable notifications: %w", err)
	}
	debugf("Notifications enabled successfully")

	// Small delay to let subscription settle
	time.Sleep(100 * time.Millisecond)

	// Build the request envelope
	req := APIRequest{
		Type:      "httpRequest",
		ID:        requestID,
		Timestamp: time.Now().UnixMilli(),
		Method:    method,
		Path:      path,
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	debugf("JSON request: %s", string(reqData))

	// Wrap in binme envelope with zlib compression
	dataToSend, err := binmeEncode(reqData, body, seqNum)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode binme: %w", err)
	}
	debugf("Binme encoded: %d bytes", len(dataToSend))
	debugf("Binme hex: %X", dataToSend)

	// Write request to characteristic
	// NOTE: tinygo bluetooth on Linux doesn't support Write with Response (only WriteWithoutResponse)
	// See: https://github.com/tinygo-org/bluetooth/issues/153
	// The official app uses Write Request (0x12), but we have to try WriteWithoutResponse
	debugf("Writing %d bytes to characteristic...", len(dataToSend))
	_, err = writeChar.WriteWithoutResponse(dataToSend)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write request: %w", err)
	}
	debugf("Write completed")

	// Wait for response with timeout
	select {
	case resp := <-responseChan:
		return &resp.envelope, resp.body, nil
	case <-time.After(5 * time.Second):
		return nil, nil, fmt.Errorf("timeout waiting for response (request ID: %s)", requestID)
	}
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

// apiPath builds an API path with the device MAC
func (ctx *APIContext) apiPath(endpoint string) string {
	return fmt.Sprintf("/api/1.0/%s%s", ctx.MAC, endpoint)
}

// enableNotifications sets up the notification handler for API responses
func (ctx *APIContext) enableNotifications() error {
	if ctx.notifyEnabled {
		return nil
	}

	ctx.responseChan = make(chan bool, 1)

	err := ctx.NotifyChar.EnableNotifications(func(buf []byte) {
		ctx.responseMu.Lock()
		defer ctx.responseMu.Unlock()

		debugf("Notification received: %d bytes (total so far: %d)", len(buf), ctx.responseBuf.Len())

		// First packet - parse outer header to get expected length
		if ctx.responseBuf.Len() == 0 && len(buf) >= 4 {
			ctx.expectedLen = int(binary.BigEndian.Uint16(buf[0:2]))
			debugf("Expected total length: %d bytes", ctx.expectedLen)
		}

		ctx.responseBuf.Write(buf)

		// Check if we have complete response
		if ctx.expectedLen > 0 && ctx.responseBuf.Len() >= ctx.expectedLen {
			debugf("Response complete: %d/%d bytes", ctx.responseBuf.Len(), ctx.expectedLen)
			select {
			case ctx.responseChan <- true:
			default:
			}
		}
	})
	if err != nil {
		return err
	}

	ctx.notifyEnabled = true
	time.Sleep(100 * time.Millisecond)
	return nil
}

// resetResponseBuffer clears the response buffer for a new request
func (ctx *APIContext) resetResponseBuffer() {
	ctx.responseMu.Lock()
	ctx.responseBuf.Reset()
	ctx.expectedLen = 0
	ctx.responseMu.Unlock()
	// Drain channel
	select {
	case <-ctx.responseChan:
	default:
	}
}

// waitForResponse waits for a complete response with timeout
func (ctx *APIContext) waitForResponse(timeout time.Duration) ([]byte, error) {
	select {
	case <-ctx.responseChan:
		ctx.responseMu.Lock()
		data := make([]byte, ctx.responseBuf.Len())
		copy(data, ctx.responseBuf.Bytes())
		ctx.responseMu.Unlock()
		return data, nil
	case <-time.After(timeout):
		ctx.responseMu.Lock()
		got := ctx.responseBuf.Len()
		expected := ctx.expectedLen
		ctx.responseMu.Unlock()
		return nil, fmt.Errorf("timeout (got %d/%d bytes)", got, expected)
	}
}

// sendRequest sends an API request and waits for response
func (ctx *APIContext) sendRequest(method, path string, body []byte, timeout time.Duration) (*APIResponse, []byte, error) {
	if err := ctx.enableNotifications(); err != nil {
		return nil, nil, fmt.Errorf("failed to enable notifications: %w", err)
	}

	ctx.resetResponseBuffer()

	requestID, seqNum := nextRequestID()

	req := APIRequest{
		Type:      "httpRequest",
		ID:        requestID,
		Timestamp: time.Now().UnixMilli(),
		Method:    method,
		Path:      path,
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	debugf("JSON request: %s", string(reqData))

	dataToSend, err := binmeEncode(reqData, body, seqNum)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode binme: %w", err)
	}

	debugf("Writing %d bytes...", len(dataToSend))
	_, err = ctx.WriteChar.WriteWithoutResponse(dataToSend)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Wait for response
	data, err := ctx.waitForResponse(timeout)
	if err != nil {
		return nil, nil, err
	}

	headerJSON, bodyData, err := binmeDecode(data)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var resp APIResponse
	if err := json.Unmarshal(headerJSON, &resp); err != nil {
		return nil, nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, bodyData, nil
}

// cmdTestEncode tests the encoding without connecting to device
func cmdTestEncode() {
	// Use the same JSON as captured from official app
	// {"type":"httpRequest","id":"00000000-0000-0000-0000-000000000003","timestamp":1768449224138,"method":"POST","path":"/api/1.0/deadbeefcafe/sif/start","headers":{}}

	req := APIRequest{
		Type:      "httpRequest",
		ID:        "00000000-0000-0000-0000-000000000005",
		Timestamp: 1768449227468,
		Method:    "GET",
		Path:      "/api/1.0/deadbeefcafe/stats",
	}
	/*
		{
			"type":"httpRequest",
			"id":"00000000-0000-0000-0000-000000000005",
			"timestamp":1768449227468,
			"method":"GET",
			"path":"/api/1.0/deadbeefcafe/stats",
			"headers":{}
		}
	*/

	jsonData, _ := json.Marshal(req)
	fmt.Printf("JSON (%d bytes): %s\n\n", len(jsonData), string(jsonData))

	// Use seqNum 5 to match the captured request ID
	encoded, err := binmeEncode(jsonData, nil, 5)
	if err != nil {
		log.Fatal("Encode failed: ", err)
	}

	fmt.Printf("Encoded (%d bytes):\n%X\n\n", len(encoded), encoded)

	// Now decode it back
	headerJSON, bodyData, err := binmeDecode(encoded)
	if err != nil {
		log.Fatal("Decode failed:", err)
	}

	fmt.Printf("Decoded header (%d bytes): %s\n", len(headerJSON), string(headerJSON))
	fmt.Printf("Decoded body (%d bytes): %X\n\n", len(bodyData), bodyData)

	// Print the captured packet for comparison
	fmt.Println("=== Captured from official app ===")
	captured := "009a000503010101000000007d789c6d8cb10ec2300c44ffc573214d1592901db157fc804b8d9221c210335455ff1d2375e48693eef4ee569085091264111ee9f5a126d04199b5ea771dfed8ae93b252aa8eb032241b7c74ee3c0cc1f9d84125c9cfdfd3f572539051b206835c8c3df6c6de3dda293c268ad1e883348532e14cef0669ddb62ffb322b7a0201010000000008789c030000000001"
	fmt.Printf("Captured: %s\n", captured)

	byteArray, err := hex.DecodeString(captured)
	if err != nil {
		log.Fatal(err)
	}

	headerJSON, bodyData, err = binmeDecode(byteArray)
	if err != nil {
		log.Fatal("Decode failed:", err)
	}
	fmt.Printf("Decoded header (%d bytes): %s\n", len(headerJSON), string(headerJSON))
	fmt.Printf("Decoded body (%d bytes): %X\n\n", len(bodyData), bodyData)

}

// cmdTestPackets reads packets from a TSV file and decodes each one
func cmdTestPackets(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatal("Failed to open file:", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Increase buffer size for large packets
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 64*1024)

	lineNum := 0
	successCount := 0
	failCount := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Parse TSV: frame_num \t src \t dst \t hex
		parts := strings.Split(line, "\t")
		if len(parts) < 4 {
			fmt.Printf("Line %d: invalid format (expected 4 columns, got %d)\n", lineNum, len(parts))
			failCount++
			continue
		}

		frameNum := parts[0]
		src := parts[1]
		dst := parts[2]
		hexData := parts[3]

		// Determine direction
		direction := "???"
		if strings.Contains(src, "Ubiquiti") {
			direction = "RSP"
		} else if strings.Contains(dst, "Ubiquiti") {
			direction = "REQ"
		}

		// Skip very short packets (like 0100ffff)
		if len(hexData) < 16 {
			fmt.Printf("Frame %s [%s]: too short (%d hex chars), skipping\n", frameNum, direction, len(hexData))
			continue
		}

		// Decode hex
		data, err := hex.DecodeString(hexData)
		if err != nil {
			fmt.Printf("Frame %s [%s]: hex decode error: %v\n", frameNum, direction, err)
			failCount++
			continue
		}

		// Try to decode as binme packet
		headerJSON, bodyData, err := binmeDecode(data)
		if err != nil {
			fmt.Printf("Frame %s [%s]: decode error: %v\n", frameNum, direction, err)
			failCount++
			continue
		}

		// Parse the header JSON to get type and path/status
		var envelope map[string]interface{}
		if err := json.Unmarshal(headerJSON, &envelope); err != nil {
			fmt.Printf("Frame %s [%s]: JSON parse error: %v\n", frameNum, direction, err)
			failCount++
			continue
		}

		// Extract relevant fields
		msgType, _ := envelope["type"].(string)
		id, _ := envelope["id"].(string)
		// Get last 4 chars of ID for display
		shortID := id
		if len(id) > 4 {
			shortID = id[len(id)-4:]
		}

		var summary string
		if msgType == "httpRequest" {
			method, _ := envelope["method"].(string)
			path, _ := envelope["path"].(string)
			summary = fmt.Sprintf("%s %s", method, path)
		} else if msgType == "httpResponse" {
			statusCode, _ := envelope["statusCode"].(float64)
			summary = fmt.Sprintf("status=%d", int(statusCode))
		} else {
			summary = msgType
		}

		// Format body
		bodyStr := ""
		if len(bodyData) > 0 {
			if len(bodyData) > 60 {
				bodyStr = fmt.Sprintf(" body=%s...", string(bodyData[:60]))
			} else {
				bodyStr = fmt.Sprintf(" body=%s", string(bodyData))
			}
		}

		fmt.Printf("Frame %s [%s] id=%s: %s%s\n", frameNum, direction, shortID, summary, bodyStr)
		successCount++
	}

	if err := scanner.Err(); err != nil {
		log.Fatal("Scanner error:", err)
	}

	fmt.Printf("\n--- Summary ---\n")
	fmt.Printf("Total lines: %d\n", lineNum)
	fmt.Printf("Success: %d\n", successCount)
	fmt.Printf("Failed: %d\n", failCount)
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

// sendLargeAPIRequest sends an API request and handles fragmented BLE responses.
// Large responses (like SIF data) are split across multiple BLE notifications.
func sendLargeAPIRequest(writeChar, notifyChar *bluetooth.DeviceCharacteristic, method, path string, body []byte, timeout time.Duration) (*APIResponse, []byte, error) {
	requestID, seqNum := nextRequestID()

	// Buffer to accumulate fragmented response
	var responseBuf bytes.Buffer
	var expectedLen int
	responseChan := make(chan bool, 1)
	var mu sync.Mutex
	var decodeErr error

	debugf("Enabling notifications for large response...")
	err := notifyChar.EnableNotifications(func(buf []byte) {
		mu.Lock()
		defer mu.Unlock()

		debugf("Notification received: %d bytes (total so far: %d)", len(buf), responseBuf.Len())

		// First packet - parse outer header to get expected length
		if responseBuf.Len() == 0 && len(buf) >= 4 {
			expectedLen = int(binary.BigEndian.Uint16(buf[0:2]))
			debugf("Expected total length: %d bytes", expectedLen)
		}

		responseBuf.Write(buf)

		// Check if we have complete response
		if expectedLen > 0 && responseBuf.Len() >= expectedLen {
			debugf("Response complete: %d/%d bytes", responseBuf.Len(), expectedLen)
			select {
			case responseChan <- true:
			default:
			}
		}
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to enable notifications: %w", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Build and send request
	req := APIRequest{
		Type:      "httpRequest",
		ID:        requestID,
		Timestamp: time.Now().UnixMilli(),
		Method:    method,
		Path:      path,
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	debugf("JSON request: %s", string(reqData))

	dataToSend, err := binmeEncode(reqData, body, seqNum)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode binme: %w", err)
	}

	debugf("Writing %d bytes...", len(dataToSend))
	_, err = writeChar.WriteWithoutResponse(dataToSend)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Wait for complete response
	select {
	case <-responseChan:
		mu.Lock()
		data := responseBuf.Bytes()
		mu.Unlock()

		if decodeErr != nil {
			return nil, nil, decodeErr
		}

		headerJSON, bodyData, err := binmeDecode(data)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decode response: %w", err)
		}

		var resp APIResponse
		if err := json.Unmarshal(headerJSON, &resp); err != nil {
			return nil, nil, fmt.Errorf("failed to parse response: %w", err)
		}

		return &resp, bodyData, nil

	case <-time.After(timeout):
		mu.Lock()
		got := responseBuf.Len()
		mu.Unlock()
		return nil, nil, fmt.Errorf("timeout waiting for response (got %d/%d bytes)", got, expectedLen)
	}
}

// cmdSupportDump downloads support info archive via SIF protocol
// Contains syslog, module database entries, and cached EEPROM snapshots
func cmdSupportDump(device bluetooth.Device) {
	ctx := setupAPI(device)

	// Step 0: Check current SIF status and abort if in progress
	fmt.Println("Checking SIF status...")
	resp, body, err := ctx.sendRequest("GET", ctx.apiPath("/sif/info/"), nil, 10*time.Second)
	if err != nil {
		log.Fatal("Failed to get SIF status:", err)
	}

	var statusResp struct {
		Status string `json:"status"`
		Offset int    `json:"offset"`
	}
	if resp.StatusCode == 200 {
		if err := json.Unmarshal(body, &statusResp); err == nil {
			debugf("Current SIF status: %s (offset=%d)", statusResp.Status, statusResp.Offset)
			// Only abort if actively in progress (not finished/complete/idle)
			if statusResp.Status == "inprogress" || statusResp.Status == "ready" || statusResp.Status == "continue" {
				fmt.Printf("SIF operation in progress (status=%s), aborting...\n", statusResp.Status)
				resp, _, err := ctx.sendRequest("POST", ctx.apiPath("/sif/abort"), nil, 10*time.Second)
				if err != nil {
					log.Fatal("Failed to abort SIF:", err)
				}
				if resp.StatusCode != 200 {
					log.Fatalf("Failed to abort SIF: status %d", resp.StatusCode)
				}
				fmt.Println("Previous SIF operation aborted")
				time.Sleep(500 * time.Millisecond)
			}
		}
	}

	fmt.Println("Starting SIF read operation...")

	// Step 1: POST /sif/start to initiate
	resp, body, err = ctx.sendRequest("POST", ctx.apiPath("/sif/start"), nil, 10*time.Second)
	if err != nil {
		log.Fatal("Failed to start SIF read:", err)
	}

	if resp.StatusCode != 200 {
		fmt.Printf("Error starting SIF: status %d\n", resp.StatusCode)
		fmt.Printf("Body: %s\n", string(body))
		return
	}

	var startResp struct {
		Status string `json:"status"`
		Offset int    `json:"offset"`
		Chunk  int    `json:"chunk"`
		Size   int    `json:"size"`
	}
	if err := json.Unmarshal(body, &startResp); err != nil {
		log.Fatal("Failed to parse start response:", err)
	}

	fmt.Printf("SIF started: size=%d bytes, chunk=%d\n", startResp.Size, startResp.Chunk)

	// Allocate buffer for full EEPROM data
	eepromData := make([]byte, 0, startResp.Size)
	offset := 0
	chunkSize := startResp.Chunk

	// Step 2: GET /sif/data/ in a loop to fetch chunks
	for offset < startResp.Size {
		remaining := startResp.Size - offset
		if remaining < chunkSize {
			chunkSize = remaining
		}

		fmt.Printf("Reading offset %d, chunk %d...\n", offset, chunkSize)

		// Request body specifies what we want
		reqBody := fmt.Sprintf(`{"status":"continue","offset":%d,"chunk":%d}`, offset, chunkSize)

		// Use longer timeout for data transfers (large responses)
		resp, body, err := ctx.sendRequest("GET", ctx.apiPath("/sif/data/"), []byte(reqBody), 30*time.Second)
		if err != nil {
			log.Fatal("Failed to read SIF data:", err)
		}

		if resp.StatusCode != 200 {
			fmt.Printf("Error reading SIF data: status %d\n", resp.StatusCode)
			fmt.Printf("Body: %s\n", string(body))
			return
		}

		// Handle end of data - device may return 0 bytes when done
		if len(body) == 0 {
			fmt.Printf("  Device returned 0 bytes, read complete\n")
			break
		}

		eepromData = append(eepromData, body...)
		offset += len(body)
		fmt.Printf("  Got %d bytes (total: %d/%d)\n", len(body), offset, startResp.Size)
	}

	// Step 3: GET /sif/info/ to verify completion
	resp, body, err = ctx.sendRequest("GET", ctx.apiPath("/sif/info/"), nil, 10*time.Second)
	if err != nil {
		log.Fatal("Failed to get SIF info:", err)
	}

	var infoResp struct {
		Status string `json:"status"`
		Offset int    `json:"offset"`
	}
	if err := json.Unmarshal(body, &infoResp); err == nil {
		fmt.Printf("SIF status: %s (offset=%d)\n", infoResp.Status, infoResp.Offset)
	}

	fmt.Printf("\nReceived %d bytes (expected %d)\n", len(eepromData), startResp.Size)

	// The SIF data is a tar archive - list contents and optionally save
	fmt.Println("\n=== SIF Archive Contents ===")
	listTarContents(eepromData)

	// Save to file
	filename := fmt.Sprintf("sif-dump-%s.tar", ctx.MAC)
	if err := os.WriteFile(filename, eepromData, 0644); err != nil {
		log.Fatal("Failed to save file:", err)
	}
	fmt.Printf("\nSaved to: %s\n", filename)
}

// binmeEncodeRawBody wraps JSON header with a raw binary body (format=0x03).
// Used for XSFP write operations that send binary EEPROM data.
func binmeEncodeRawBody(jsonData []byte, bodyData []byte, seqNum uint16) ([]byte, error) {
	// Compress header JSON
	compressedHeader, err := zlibCompress(jsonData)
	if err != nil {
		return nil, fmt.Errorf("failed to compress header: %w", err)
	}

	// Build the message
	var buf bytes.Buffer

	// Header section: 9 bytes header + compressed data
	headerSection := make([]byte, 9+len(compressedHeader))
	headerSection[0] = 0x03 // marker: header section
	headerSection[1] = 0x01 // format: JSON
	headerSection[2] = 0x01 // compression: zlib
	headerSection[3] = 0x01 // flags
	headerSection[4] = 0x00
	headerSection[5] = 0x00
	headerSection[6] = 0x00
	headerSection[7] = 0x00
	headerSection[8] = byte(len(compressedHeader))
	copy(headerSection[9:], compressedHeader)

	// Body section: 8 bytes header + raw binary data (NOT compressed)
	bodySection := make([]byte, 8+len(bodyData))
	bodySection[0] = 0x02 // marker: body section
	bodySection[1] = 0x03 // format: raw binary (0x03)
	bodySection[2] = 0x00 // compression: none
	bodySection[3] = 0x00 // reserved
	binary.BigEndian.PutUint32(bodySection[4:8], uint32(len(bodyData)))
	copy(bodySection[8:], bodyData)

	// Total message length (excluding outer header)
	totalLen := len(headerSection) + len(bodySection)

	// Write outer header
	outerHeader := make([]byte, 4)
	binary.BigEndian.PutUint16(outerHeader[0:2], uint16(totalLen+4))
	binary.BigEndian.PutUint16(outerHeader[2:4], seqNum)

	buf.Write(outerHeader)
	buf.Write(headerSection)
	buf.Write(bodySection)

	return buf.Bytes(), nil
}

// sendRawBodyRequest sends an API request with a raw binary body (for XSFP writes)
// Large packets are fragmented across multiple BLE writes.
func (ctx *APIContext) sendRawBodyRequest(method, path string, body []byte, timeout time.Duration) (*APIResponse, []byte, error) {
	if err := ctx.enableNotifications(); err != nil {
		return nil, nil, fmt.Errorf("failed to enable notifications: %w", err)
	}

	ctx.resetResponseBuffer()

	requestID, seqNum := nextRequestID()

	req := APIRequest{
		Type:      "httpRequest",
		ID:        requestID,
		Timestamp: time.Now().UnixMilli(),
		Method:    method,
		Path:      path,
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	debugf("JSON request: %s", string(reqData))
	debugf("Body: %d bytes of binary data", len(body))

	// Use raw body encoding for binary data
	dataToSend, err := binmeEncodeRawBody(reqData, body, seqNum)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode binme: %w", err)
	}

	debugf("Total packet size: %d bytes", len(dataToSend))

	// Fragment into BLE MTU-sized chunks (244 bytes is typical for BLE 4.2+)
	const bleMTU = 244
	for offset := 0; offset < len(dataToSend); offset += bleMTU {
		end := offset + bleMTU
		if end > len(dataToSend) {
			end = len(dataToSend)
		}
		chunk := dataToSend[offset:end]

		debugf("Writing chunk %d-%d (%d bytes)", offset, end, len(chunk))
		_, err = ctx.WriteChar.WriteWithoutResponse(chunk)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to write chunk at offset %d: %w", offset, err)
		}

		// Small delay between chunks to let device process
		if end < len(dataToSend) {
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Wait for response
	data, err := ctx.waitForResponse(timeout)
	if err != nil {
		return nil, nil, err
	}

	headerJSON, bodyData, err := binmeDecode(data)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var resp APIResponse
	if err := json.Unmarshal(headerJSON, &resp); err != nil {
		return nil, nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &resp, bodyData, nil
}

// cmdSnapshotWrite writes EEPROM data to the snapshot buffer
// Use device screen to apply snapshot to physical module
func cmdSnapshotWrite(device bluetooth.Device, filename string) {
	ctx := setupAPI(device)

	// Read the EEPROM file
	eepromData, err := os.ReadFile(filename)
	if err != nil {
		log.Fatalf("Failed to read file: %v", err)
	}

	// Validate size
	if len(eepromData) != 512 && len(eepromData) != 640 {
		log.Fatalf("Invalid EEPROM size: %d bytes (expected 512 for SFP or 640 for QSFP)", len(eepromData))
	}

	moduleType := "SFP"
	if len(eepromData) == 640 {
		moduleType = "QSFP"
	}

	fmt.Printf("Loaded %s EEPROM data: %d bytes from %s\n", moduleType, len(eepromData), filename)

	// Parse and display what we're about to write
	if len(eepromData) >= 96 {
		vendorName := strings.TrimSpace(string(eepromData[20:36]))
		vendorPN := strings.TrimSpace(string(eepromData[40:56]))
		vendorSN := strings.TrimSpace(string(eepromData[68:84]))
		fmt.Printf("  Vendor: %s\n", vendorName)
		fmt.Printf("  Part:   %s\n", vendorPN)
		fmt.Printf("  S/N:    %s\n", vendorSN)
	}

	fmt.Println()
	fmt.Println("This will write to the snapshot buffer.")
	fmt.Println("Use the device screen to apply snapshot to module.")
	fmt.Print("Type 'yes' to continue: ")

	reader := bufio.NewReader(os.Stdin)
	confirm, _ := reader.ReadString('\n')
	confirm = strings.TrimSpace(confirm)
	if confirm != "yes" {
		fmt.Println("Aborted.")
		return
	}

	// Step 1: POST /xsfp/sync/start with size
	fmt.Println("\nInitializing snapshot write...")
	startBody := fmt.Sprintf(`{"size":%d}`, len(eepromData))
	resp, body, err := ctx.sendRequest("POST", ctx.apiPath("/xsfp/sync/start"), []byte(startBody), 10*time.Second)
	if err != nil {
		log.Fatalf("Failed to initialize snapshot: %v", err)
	}

	if resp.StatusCode != 200 {
		fmt.Printf("Error initializing snapshot: status %d\n", resp.StatusCode)
		if len(body) > 0 {
			fmt.Printf("Response: %s\n", string(body))
		}
		return
	}

	fmt.Printf("Snapshot initialized: %s\n", string(body))

	// Step 2: POST /xsfp/sync/data with binary EEPROM data
	fmt.Printf("Writing %d bytes to snapshot...\n", len(eepromData))
	resp, body, err = ctx.sendRawBodyRequest("POST", ctx.apiPath("/xsfp/sync/data"), eepromData, 30*time.Second)
	if err != nil {
		log.Fatalf("Failed to write snapshot data: %v", err)
	}

	if resp.StatusCode != 200 {
		fmt.Printf("Error writing snapshot data: status %d\n", resp.StatusCode)
		if len(body) > 0 {
			fmt.Printf("Response: %s\n", string(body))
		}
		return
	}

	fmt.Printf("Snapshot write complete!\n")
	if len(body) > 0 {
		var prettyJSON bytes.Buffer
		if err := json.Indent(&prettyJSON, body, "", "  "); err != nil {
			fmt.Printf("Response: %s\n", string(body))
		} else {
			fmt.Println(prettyJSON.String())
		}
	}

	fmt.Println("\nUse the device screen to apply snapshot to module.")
}

// cmdSnapshotInfo gets info about the snapshot buffer
func cmdSnapshotInfo(device bluetooth.Device) {
	ctx := setupAPI(device)

	fmt.Println("Getting snapshot info...")

	resp, body, err := ctx.sendRequest("GET", ctx.apiPath("/xsfp/sync/start"), nil, 10*time.Second)
	if err != nil {
		log.Fatal("API request failed:", err)
	}

	if resp.StatusCode != 200 {
		fmt.Printf("Error: status %d\n", resp.StatusCode)
		if len(body) > 0 {
			fmt.Printf("Body: %s\n", string(body))
		}
		return
	}

	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, body, "", "  "); err != nil {
		fmt.Printf("Body: %s\n", string(body))
	} else {
		fmt.Println(prettyJSON.String())
	}
}

// cmdSnapshotRead reads the snapshot buffer and saves to file
func cmdSnapshotRead(device bluetooth.Device, filename string) {
	ctx := setupAPI(device)

	// Step 1: GET /xsfp/sync/start to initialize and get size
	fmt.Println("Initializing snapshot read...")
	resp, body, err := ctx.sendRequest("GET", ctx.apiPath("/xsfp/sync/start"), nil, 10*time.Second)
	if err != nil {
		log.Fatal("Failed to initialize:", err)
	}

	if resp.StatusCode != 200 {
		fmt.Printf("Error initializing: status %d\n", resp.StatusCode)
		if len(body) > 0 {
			fmt.Printf("Body: %s\n", string(body))
		}
		return
	}

	fmt.Printf("Snapshot info: %s\n", string(body))

	// Parse to get size info
	var startResp struct {
		Size  int `json:"size"`
		Chunk int `json:"chunk"`
	}
	if err := json.Unmarshal(body, &startResp); err != nil {
		// Try reading without size info
		startResp.Size = 512
		startResp.Chunk = 512
	}

	// Step 2: GET /xsfp/sync/data to read data
	fmt.Println("Reading snapshot data...")
	reqBody := fmt.Sprintf(`{"offset":0,"chunk":%d}`, startResp.Size)
	resp, body, err = ctx.sendRequest("GET", ctx.apiPath("/xsfp/sync/data"), []byte(reqBody), 30*time.Second)
	if err != nil {
		log.Fatal("Failed to read data:", err)
	}

	if resp.StatusCode != 200 {
		fmt.Printf("Error reading data: status %d\n", resp.StatusCode)
		if len(body) > 0 {
			fmt.Printf("Body: %s\n", string(body))
		}
		return
	}

	fmt.Printf("Received %d bytes\n", len(body))

	// Save to file
	if err := os.WriteFile(filename, body, 0644); err != nil {
		log.Fatalf("Failed to write file: %v", err)
	}
	fmt.Printf("Saved to: %s\n", filename)

	// Display info about the data
	if len(body) >= 96 {
		vendorName := strings.TrimSpace(string(body[20:36]))
		vendorPN := strings.TrimSpace(string(body[40:56]))
		vendorSN := strings.TrimSpace(string(body[68:84]))
		fmt.Printf("\nModule info:\n")
		fmt.Printf("  Vendor: %s\n", vendorName)
		fmt.Printf("  Part:   %s\n", vendorPN)
		fmt.Printf("  S/N:    %s\n", vendorSN)
	}
}

// cmdModuleRead reads EEPROM from the physical module and saves to file
func cmdModuleRead(device bluetooth.Device, filename string) {
	ctx := setupAPI(device)

	// Step 1: GET /xsfp/module/start to initialize read and get size
	fmt.Println("Initializing module read...")
	resp, body, err := ctx.sendRequest("GET", ctx.apiPath("/xsfp/module/start"), nil, 10*time.Second)
	if err != nil {
		log.Fatal("Failed to initialize:", err)
	}

	if resp.StatusCode != 200 {
		fmt.Printf("Error initializing: status %d\n", resp.StatusCode)
		if len(body) > 0 {
			fmt.Printf("Body: %s\n", string(body))
		}
		return
	}

	fmt.Printf("Module info: %s\n", string(body))

	// Parse to get size info
	var startResp struct {
		Size  int `json:"size"`
		Chunk int `json:"chunk"`
	}
	if err := json.Unmarshal(body, &startResp); err != nil {
		// Default to SFP size
		startResp.Size = 512
		startResp.Chunk = 512
	}
	if startResp.Size == 0 {
		startResp.Size = 512
	}

	// Step 2: GET /xsfp/module/data to read the data
	fmt.Println("Reading module data...")
	reqBody := fmt.Sprintf(`{"offset":0,"chunk":%d}`, startResp.Size)
	resp, body, err = ctx.sendRequest("GET", ctx.apiPath("/xsfp/module/data"), []byte(reqBody), 30*time.Second)
	if err != nil {
		log.Fatal("Failed to read module data:", err)
	}

	if resp.StatusCode != 200 {
		fmt.Printf("Error reading module data: status %d\n", resp.StatusCode)
		if len(body) > 0 {
			fmt.Printf("Body: %s\n", string(body))
		}
		return
	}

	fmt.Printf("Received %d bytes\n", len(body))

	// Save to file
	if err := os.WriteFile(filename, body, 0644); err != nil {
		log.Fatalf("Failed to write file: %v", err)
	}
	fmt.Printf("Saved to: %s\n", filename)

	// Display info about the data
	if len(body) >= 96 {
		vendorName := strings.TrimSpace(string(body[20:36]))
		vendorPN := strings.TrimSpace(string(body[40:56]))
		vendorSN := strings.TrimSpace(string(body[68:84]))
		fmt.Printf("\nModule info:\n")
		fmt.Printf("  Vendor: %s\n", vendorName)
		fmt.Printf("  Part:   %s\n", vendorPN)
		fmt.Printf("  S/N:    %s\n", vendorSN)
	}
}

// cmdModuleInfo gets details about the inserted SFP module
func cmdModuleInfo(device bluetooth.Device) {
	ctx := setupAPI(device)

	fmt.Println("Getting module details...")

	// The XSFP endpoints use the same base path structure
	resp, body, err := ctx.sendRequest("GET", ctx.apiPath("/xsfp/module/details"), nil, 10*time.Second)
	if err != nil {
		log.Fatal("API request failed:", err)
	}

	if resp.StatusCode != 200 {
		fmt.Printf("Error: status %d\n", resp.StatusCode)
		if len(body) > 0 {
			fmt.Printf("Body: %s\n", string(body))
		}
		return
	}

	// Pretty print the JSON response
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, body, "", "  "); err != nil {
		fmt.Printf("Body (raw): %s\n", string(body))
	} else {
		fmt.Println(prettyJSON.String())
	}
}

// cmdParseEEPROM parses and displays SFP/QSFP EEPROM data from a file
func cmdParseEEPROM(filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		log.Fatalf("Failed to read file: %v", err)
	}

	fmt.Printf("File: %s (%d bytes)\n\n", filename, len(data))

	// Check for empty/invalid data
	if len(data) == 0 {
		fmt.Println("ERROR: File is empty")
		return
	}

	// Check if all 0xff (no module)
	allFF := true
	for _, b := range data {
		if b != 0xff {
			allFF = false
			break
		}
	}
	if allFF {
		fmt.Println("WARNING: File contains all 0xFF bytes (no module data)")
		return
	}

	// Determine module type by size and identifier
	if len(data) < 128 {
		fmt.Printf("ERROR: File too small for SFP EEPROM (need at least 128 bytes, got %d)\n", len(data))
		return
	}

	identifier := data[0]
	switch identifier {
	case 0x03:
		fmt.Println("=== SFP/SFP+ Module (SFF-8472) ===\n")
		parseSFPEEPROMDetailed(data)
	case 0x0c:
		fmt.Println("=== QSFP Module (SFF-8436) ===\n")
		parseQSFPEEPROMDetailed(data)
	case 0x0d:
		fmt.Println("=== QSFP+ Module (SFF-8636) ===\n")
		parseQSFPEEPROMDetailed(data)
	case 0x11:
		fmt.Println("=== QSFP28 Module (SFF-8636) ===\n")
		parseQSFPEEPROMDetailed(data)
	default:
		fmt.Printf("=== Unknown Module Type (identifier: 0x%02X) ===\n\n", identifier)
		// Try SFP parsing anyway
		parseSFPEEPROMDetailed(data)
	}
}

// parseSFPEEPROMDetailed parses SFP EEPROM data per SFF-8472
func parseSFPEEPROMDetailed(data []byte) {
	// === Basic Info (A0h page) ===
	fmt.Println("--- Basic Info ---")

	// Byte 0: Identifier
	identStr := "Unknown"
	switch data[0] {
	case 0x01:
		identStr = "GBIC"
	case 0x02:
		identStr = "Module soldered to motherboard"
	case 0x03:
		identStr = "SFP/SFP+"
	case 0x04:
		identStr = "300 pin XBI"
	case 0x05:
		identStr = "XENPAK"
	case 0x06:
		identStr = "XFP"
	case 0x07:
		identStr = "XFF"
	case 0x08:
		identStr = "XFP-E"
	case 0x09:
		identStr = "XPAK"
	case 0x0A:
		identStr = "X2"
	}
	fmt.Printf("Identifier:       0x%02X (%s)\n", data[0], identStr)

	// Byte 1: Extended Identifier
	fmt.Printf("Ext Identifier:   0x%02X\n", data[1])

	// Byte 2: Connector Type
	connStr := getConnectorType(data[2])
	fmt.Printf("Connector:        0x%02X (%s)\n", data[2], connStr)

	// Bytes 3-10: Transceiver compliance codes
	fmt.Println("\n--- Transceiver Compliance ---")
	parseTransceiverCodes(data[3:11])

	// Byte 11: Encoding
	encStr := getEncodingType(data[11])
	fmt.Printf("Encoding:         0x%02X (%s)\n", data[11], encStr)

	// Byte 12: Nominal Bit Rate (units of 100 MBd)
	bitrate := int(data[12]) * 100
	fmt.Printf("Nominal Bitrate:  %d MBd\n", bitrate)

	// Byte 13: Rate Identifier
	fmt.Printf("Rate Identifier:  0x%02X\n", data[13])

	// Bytes 14-19: Link length
	fmt.Println("\n--- Link Length ---")
	if data[14] > 0 {
		fmt.Printf("Single Mode (km): %d km\n", int(data[14]))
	}
	if data[15] > 0 {
		fmt.Printf("Single Mode (m):  %d00 m\n", int(data[15]))
	}
	if data[16] > 0 {
		fmt.Printf("50um OM2:         %d0 m\n", int(data[16]))
	}
	if data[17] > 0 {
		fmt.Printf("62.5um OM1:       %d0 m\n", int(data[17]))
	}
	if data[18] > 0 {
		fmt.Printf("Copper/OM4:       %d m\n", int(data[18]))
	}
	if data[19] > 0 {
		fmt.Printf("OM3:              %d0 m\n", int(data[19]))
	}

	// Vendor info
	fmt.Println("\n--- Vendor Info ---")
	vendorName := strings.TrimSpace(string(data[20:36]))
	fmt.Printf("Vendor Name:      %s\n", vendorName)

	// Bytes 37-39: Vendor OUI
	oui := fmt.Sprintf("%02X:%02X:%02X", data[37], data[38], data[39])
	fmt.Printf("Vendor OUI:       %s\n", oui)

	vendorPN := strings.TrimSpace(string(data[40:56]))
	fmt.Printf("Part Number:      %s\n", vendorPN)

	vendorRev := strings.TrimSpace(string(data[56:60]))
	fmt.Printf("Revision:         %s\n", vendorRev)

	// Bytes 60-61: Wavelength (in nm) or copper cable attenuation
	wavelength := int(data[60])<<8 | int(data[61])
	if wavelength > 0 && wavelength < 2000 {
		fmt.Printf("Wavelength:       %d nm\n", wavelength)
	} else if wavelength > 0 {
		// Might be copper cable attenuation
		fmt.Printf("Cable Atten:      %d (raw value)\n", wavelength)
	}

	// Byte 62: Unallocated
	// Byte 63: CC_BASE (checksum)

	vendorSN := strings.TrimSpace(string(data[68:84]))
	fmt.Printf("Serial Number:    %s\n", vendorSN)

	// Bytes 84-91: Date code (YYMMDDLL)
	dateCode := string(data[84:92])
	if len(dateCode) >= 6 {
		year := dateCode[0:2]
		month := dateCode[2:4]
		day := dateCode[4:6]
		lot := ""
		if len(dateCode) >= 8 {
			lot = dateCode[6:8]
		}
		fmt.Printf("Date Code:        20%s-%s-%s (Lot: %s)\n", year, month, day, lot)
	}

	// Diagnostic Monitoring Type (byte 92)
	fmt.Println("\n--- Diagnostic Monitoring ---")
	dmType := data[92]
	fmt.Printf("Diag Type:        0x%02X\n", dmType)
	if dmType&0x40 != 0 {
		fmt.Println("  - Digital diagnostics implemented")
	}
	if dmType&0x20 != 0 {
		fmt.Println("  - Internally calibrated")
	}
	if dmType&0x10 != 0 {
		fmt.Println("  - Externally calibrated")
	}
	if dmType&0x08 != 0 {
		fmt.Println("  - Received power measurement: average")
	} else {
		fmt.Println("  - Received power measurement: OMA")
	}
	if dmType&0x04 != 0 {
		fmt.Println("  - Address change required")
	}

	// Enhanced Options (byte 93)
	enhOpts := data[93]
	fmt.Printf("Enhanced Opts:    0x%02X\n", enhOpts)

	// SFF-8472 Compliance (byte 94)
	compliance := data[94]
	compStr := "Unknown"
	switch compliance {
	case 0:
		compStr = "Not specified"
	case 1:
		compStr = "SFF-8472 Rev 9.3"
	case 2:
		compStr = "SFF-8472 Rev 9.5"
	case 3:
		compStr = "SFF-8472 Rev 10.2"
	case 4:
		compStr = "SFF-8472 Rev 10.4"
	case 5:
		compStr = "SFF-8472 Rev 11.0"
	case 6:
		compStr = "SFF-8472 Rev 11.3"
	case 7:
		compStr = "SFF-8472 Rev 11.4"
	case 8:
		compStr = "SFF-8472 Rev 12.0"
	}
	fmt.Printf("SFF-8472 Rev:     0x%02X (%s)\n", compliance, compStr)

	// Checksum validation
	fmt.Println("\n--- Checksums ---")
	// CC_BASE covers bytes 0-62
	var ccBase byte
	for i := 0; i < 63; i++ {
		ccBase += data[i]
	}
	storedCCBase := data[63]
	if ccBase == storedCCBase {
		fmt.Printf("CC_BASE:          0x%02X (VALID)\n", storedCCBase)
	} else {
		fmt.Printf("CC_BASE:          0x%02X (INVALID - calculated 0x%02X)\n", storedCCBase, ccBase)
	}

	// CC_EXT covers bytes 64-94
	var ccExt byte
	for i := 64; i < 95; i++ {
		ccExt += data[i]
	}
	storedCCExt := data[95]
	if ccExt == storedCCExt {
		fmt.Printf("CC_EXT:           0x%02X (VALID)\n", storedCCExt)
	} else {
		fmt.Printf("CC_EXT:           0x%02X (INVALID - calculated 0x%02X)\n", storedCCExt, ccExt)
	}

	// A2h page (if present - bytes 256-511)
	if len(data) >= 512 {
		fmt.Println("\n--- A2h Page (Diagnostic Data) ---")
		a2 := data[256:]

		// Alarm and warning thresholds (bytes 0-55)
		// Real-time diagnostics (bytes 96-105)
		if len(a2) >= 106 {
			// Temperature: bytes 96-97 (signed, 1/256 degree C)
			tempRaw := int16(a2[96])<<8 | int16(a2[97])
			temp := float64(tempRaw) / 256.0
			fmt.Printf("Temperature:      %.1f C\n", temp)

			// Vcc: bytes 98-99 (unsigned, 100 uV)
			vccRaw := uint16(a2[98])<<8 | uint16(a2[99])
			vcc := float64(vccRaw) / 10000.0
			fmt.Printf("Supply Voltage:   %.2f V\n", vcc)

			// TX Bias: bytes 100-101 (unsigned, 2 uA)
			biasRaw := uint16(a2[100])<<8 | uint16(a2[101])
			bias := float64(biasRaw) * 2 / 1000.0
			fmt.Printf("TX Bias Current:  %.1f mA\n", bias)

			// TX Power: bytes 102-103 (unsigned, 0.1 uW)
			txPowerRaw := uint16(a2[102])<<8 | uint16(a2[103])
			txPowerMw := float64(txPowerRaw) / 10000.0
			txPowerDbm := 10 * (log10(txPowerMw))
			fmt.Printf("TX Power:         %.2f mW (%.1f dBm)\n", txPowerMw, txPowerDbm)

			// RX Power: bytes 104-105 (unsigned, 0.1 uW)
			rxPowerRaw := uint16(a2[104])<<8 | uint16(a2[105])
			rxPowerMw := float64(rxPowerRaw) / 10000.0
			rxPowerDbm := 10 * (log10(rxPowerMw))
			fmt.Printf("RX Power:         %.2f mW (%.1f dBm)\n", rxPowerMw, rxPowerDbm)
		}
	}
}

// log10 returns log base 10, handling zero
func log10(x float64) float64 {
	if x <= 0 {
		return -40.0 // Return a very low dBm for zero power
	}
	return math.Log10(x)
}

// parseQSFPEEPROMDetailed parses QSFP EEPROM data per SFF-8636
func parseQSFPEEPROMDetailed(data []byte) {
	// QSFP has different layout - Page 00h starts at byte 128
	if len(data) < 256 {
		fmt.Printf("ERROR: Insufficient data for QSFP parsing (need 256+ bytes)\n")
		return
	}

	fmt.Println("--- Basic Info ---")

	// Byte 128: Identifier
	identStr := "Unknown"
	switch data[128] {
	case 0x0c:
		identStr = "QSFP"
	case 0x0d:
		identStr = "QSFP+"
	case 0x11:
		identStr = "QSFP28"
	}
	fmt.Printf("Identifier:       0x%02X (%s)\n", data[128], identStr)

	// Connector type at byte 130
	connStr := getConnectorType(data[130])
	fmt.Printf("Connector:        0x%02X (%s)\n", data[130], connStr)

	// Vendor info
	fmt.Println("\n--- Vendor Info ---")
	vendorName := strings.TrimSpace(string(data[148:164]))
	fmt.Printf("Vendor Name:      %s\n", vendorName)

	vendorPN := strings.TrimSpace(string(data[168:184]))
	fmt.Printf("Part Number:      %s\n", vendorPN)

	vendorRev := strings.TrimSpace(string(data[184:186]))
	fmt.Printf("Revision:         %s\n", vendorRev)

	vendorSN := strings.TrimSpace(string(data[196:212]))
	fmt.Printf("Serial Number:    %s\n", vendorSN)

	// Date code (bytes 212-219)
	dateCode := string(data[212:220])
	if len(dateCode) >= 6 {
		year := dateCode[0:2]
		month := dateCode[2:4]
		day := dateCode[4:6]
		fmt.Printf("Date Code:        20%s-%s-%s\n", year, month, day)
	}

	// Real-time monitoring data is in lower page (bytes 22-33 for temps, voltages, etc)
	fmt.Println("\n--- Real-time Diagnostics ---")
	// Temperature at bytes 22-23
	if len(data) >= 24 {
		tempRaw := int16(data[22])<<8 | int16(data[23])
		temp := float64(tempRaw) / 256.0
		fmt.Printf("Temperature:      %.1f C\n", temp)
	}

	// Vcc at bytes 26-27
	if len(data) >= 28 {
		vccRaw := uint16(data[26])<<8 | uint16(data[27])
		vcc := float64(vccRaw) / 10000.0
		fmt.Printf("Supply Voltage:   %.2f V\n", vcc)
	}
}

// getConnectorType returns a string description for connector type code
func getConnectorType(code byte) string {
	switch code {
	case 0x00:
		return "Unknown"
	case 0x01:
		return "SC"
	case 0x02:
		return "FC Style 1"
	case 0x03:
		return "FC Style 2"
	case 0x04:
		return "BNC/TNC"
	case 0x05:
		return "FC coax"
	case 0x06:
		return "Fiber Jack"
	case 0x07:
		return "LC"
	case 0x08:
		return "MT-RJ"
	case 0x09:
		return "MU"
	case 0x0A:
		return "SG"
	case 0x0B:
		return "Optical Pigtail"
	case 0x0C:
		return "MPO 1x12"
	case 0x0D:
		return "MPO 2x16"
	case 0x20:
		return "HSSDC II"
	case 0x21:
		return "Copper Pigtail"
	case 0x22:
		return "RJ45"
	case 0x23:
		return "No separable connector"
	case 0x24:
		return "MXC 2x16"
	default:
		return "Vendor specific"
	}
}

// getEncodingType returns a string description for encoding type code
func getEncodingType(code byte) string {
	switch code {
	case 0x00:
		return "Unspecified"
	case 0x01:
		return "8B/10B"
	case 0x02:
		return "4B/5B"
	case 0x03:
		return "NRZ"
	case 0x04:
		return "Manchester"
	case 0x05:
		return "SONET Scrambled"
	case 0x06:
		return "64B/66B"
	default:
		return "Unknown"
	}
}

// parseTransceiverCodes prints transceiver compliance codes
func parseTransceiverCodes(codes []byte) {
	// Byte 3: 10G Ethernet / Infiniband
	if codes[0]&0x80 != 0 {
		fmt.Println("  - 10G Base-ER")
	}
	if codes[0]&0x40 != 0 {
		fmt.Println("  - 10G Base-LRM")
	}
	if codes[0]&0x20 != 0 {
		fmt.Println("  - 10G Base-LR")
	}
	if codes[0]&0x10 != 0 {
		fmt.Println("  - 10G Base-SR")
	}

	// Byte 6: Gigabit Ethernet
	if codes[3]&0x08 != 0 {
		fmt.Println("  - 1000BASE-T")
	}
	if codes[3]&0x04 != 0 {
		fmt.Println("  - 1000BASE-CX")
	}
	if codes[3]&0x02 != 0 {
		fmt.Println("  - 1000BASE-LX")
	}
	if codes[3]&0x01 != 0 {
		fmt.Println("  - 1000BASE-SX")
	}

	// Byte 8: SFP+ Cable Technology
	if codes[5]&0x08 != 0 {
		fmt.Println("  - Active Cable")
	}
	if codes[5]&0x04 != 0 {
		fmt.Println("  - Passive Cable")
	}

	// Byte 9: Fibre Channel transmission media
	if codes[6]&0x80 != 0 {
		fmt.Println("  - Twin Axial Pair (TW)")
	}
	if codes[6]&0x40 != 0 {
		fmt.Println("  - Twisted Pair (TP)")
	}
	if codes[6]&0x20 != 0 {
		fmt.Println("  - Miniature Coax (MI)")
	}
	if codes[6]&0x10 != 0 {
		fmt.Println("  - Video Coax (TV)")
	}
	if codes[6]&0x08 != 0 {
		fmt.Println("  - Multi-mode 62.5um (M6)")
	}
	if codes[6]&0x04 != 0 {
		fmt.Println("  - Multi-mode 50um (M5)")
	}
	if codes[6]&0x02 != 0 {
		fmt.Println("  - Single Mode (SM)")
	}
}

// cmdReboot reboots the device
func cmdReboot(device bluetooth.Device) {
	ctx := setupAPI(device)

	fmt.Println("Rebooting device...")

	resp, body, err := ctx.sendRequest("POST", ctx.apiPath("/reboot"), nil, 10*time.Second)
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

// listTarContents lists the files in a tar archive
func listTarContents(data []byte) {
	tr := tar.NewReader(bytes.NewReader(data))

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("Error reading tar: %v\n", err)
			return
		}

		// Format size nicely
		sizeStr := fmt.Sprintf("%6d", hdr.Size)
		if hdr.Size >= 1024 {
			sizeStr = fmt.Sprintf("%3dKB", hdr.Size/1024)
		}

		fmt.Printf("  %s  %s\n", sizeStr, hdr.Name)

		// If it's an EEPROM bin file, try to parse it
		if strings.HasSuffix(hdr.Name, ".bin") {
			eepromData, err := io.ReadAll(tr)
			if err != nil {
				continue
			}
			// Skip files that are all 0xff (no module present)
			if len(eepromData) > 0 && eepromData[0] == 0xff {
				fmt.Println("           (no module)")
				continue
			}
			if len(eepromData) >= 256 {
				parseSFPInfo(eepromData)
			}
		}
	}
}

// printHexDump prints data in hex dump format
func printHexDump(data []byte) {
	for i := 0; i < len(data); i += 16 {
		// Address
		fmt.Printf("%04x  ", i)

		// Hex bytes
		for j := 0; j < 16; j++ {
			if i+j < len(data) {
				fmt.Printf("%02x ", data[i+j])
			} else {
				fmt.Print("   ")
			}
			if j == 7 {
				fmt.Print(" ")
			}
		}

		// ASCII
		fmt.Print(" |")
		for j := 0; j < 16 && i+j < len(data); j++ {
			b := data[i+j]
			if b >= 32 && b < 127 {
				fmt.Printf("%c", b)
			} else {
				fmt.Print(".")
			}
		}
		fmt.Println("|")
	}
}

// parseSFPInfo extracts and displays SFP module information from EEPROM data
func parseSFPInfo(data []byte) {
	// A0h page (first 256 bytes of EEPROM)
	// Based on SFF-8472 specification

	if len(data) < 96 {
		fmt.Println("           (insufficient data)")
		return
	}

	// Byte 0: Identifier (03 = SFP)
	identifier := data[0]
	idStr := "Unknown"
	switch identifier {
	case 0x03:
		idStr = "SFP/SFP+"
	case 0x0d:
		idStr = "QSFP+"
	case 0x11:
		idStr = "QSFP28"
	}

	// Bytes 20-35: Vendor Name (16 bytes ASCII)
	vendorName := strings.TrimSpace(string(data[20:36]))

	// Bytes 40-55: Vendor Part Number (16 bytes ASCII)
	vendorPN := strings.TrimSpace(string(data[40:56]))

	// Bytes 68-83: Vendor Serial Number (16 bytes ASCII)
	vendorSN := strings.TrimSpace(string(data[68:84]))

	// Byte 12: Nominal Bit Rate (units of 100 MBd)
	bitrate := int(data[12]) * 100

	// Bytes 60-61: Wavelength (in nm)
	wavelength := 0
	if len(data) >= 62 {
		wavelength = int(data[60])<<8 | int(data[61])
	}

	// Compact single-line output
	fmt.Printf("           %s: %s %s (S/N: %s) %dMBd %dnm\n",
		idStr, vendorName, vendorPN, vendorSN, bitrate, wavelength)
}
