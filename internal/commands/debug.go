package commands

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/vitaminmoo/sfpw-tool/internal/eeprom"
	"github.com/vitaminmoo/sfpw-tool/internal/protocol"
)

// TestEncode tests the encoding without connecting to device
func TestEncode() {
	// Use the same JSON as captured from official app
	// {"type":"httpRequest","id":"00000000-0000-0000-0000-000000000003","timestamp":1768449224138,"method":"POST","path":"/api/1.0/deadbeefcafe/sif/start","headers":{}}

	req := protocol.APIRequest{
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
	encoded, err := protocol.BinmeEncode(jsonData, nil, 5)
	if err != nil {
		log.Fatal("Encode failed: ", err)
	}

	fmt.Printf("Encoded (%d bytes):\n%X\n\n", len(encoded), encoded)

	// Now decode it back
	headerJSON, bodyData, err := protocol.BinmeDecode(encoded)
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

	headerJSON, bodyData, err = protocol.BinmeDecode(byteArray)
	if err != nil {
		log.Fatal("Decode failed:", err)
	}
	fmt.Printf("Decoded header (%d bytes): %s\n", len(headerJSON), string(headerJSON))
	fmt.Printf("Decoded body (%d bytes): %X\n\n", len(bodyData), bodyData)
}

// TestPackets reads packets from a TSV file and decodes each one
func TestPackets(filename string) {
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
		headerJSON, bodyData, err := protocol.BinmeDecode(data)
		if err != nil {
			fmt.Printf("Frame %s [%s]: decode error: %v\n", frameNum, direction, err)
			failCount++
			continue
		}

		// Parse the header JSON to get type and path/status
		var envelope map[string]any
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

// ParseEEPROM parses and displays SFP/QSFP EEPROM data from a file
func ParseEEPROM(filename string) {
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
		fmt.Println("=== SFP/SFP+ Module (SFF-8472) ===")
		fmt.Println()
		eeprom.ParseSFPDetailed(data)
	case 0x0c:
		fmt.Println("=== QSFP Module (SFF-8436) ===")
		fmt.Println()
		eeprom.ParseQSFPDetailed(data)
	case 0x0d:
		fmt.Println("=== QSFP+ Module (SFF-8636) ===")
		fmt.Println()
		eeprom.ParseQSFPDetailed(data)
	case 0x11:
		fmt.Println("=== QSFP28 Module (SFF-8636) ===")
		fmt.Println()
		eeprom.ParseQSFPDetailed(data)
	default:
		fmt.Printf("=== Unknown Module Type (identifier: 0x%02X) ===\n\n", identifier)
		// Try SFP parsing anyway
		eeprom.ParseSFPDetailed(data)
	}
}
