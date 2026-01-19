package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/vitaminmoo/sfpw-tool/internal/ble"
	"github.com/vitaminmoo/sfpw-tool/internal/store"

	"tinygo.org/x/bluetooth"
)

// ModuleInfo gets details about the inserted SFP module
func ModuleInfo(device bluetooth.Device) {
	ctx := ble.SetupAPI(device)

	fmt.Println("Getting module details...")

	// The XSFP endpoints use the same base path structure
	resp, body, err := ctx.SendRequest("GET", ctx.APIPath("/xsfp/module/details"), nil, 10*time.Second)
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

// ModuleRead reads EEPROM from the physical module and saves to store.
// If filename is not empty, also saves to that file.
func ModuleRead(device bluetooth.Device, filename string) {
	ctx := ble.SetupAPI(device)
	data, err := ModuleReadData(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Always save to store
	s, err := store.OpenDefault()
	if err != nil {
		log.Fatalf("Failed to open store: %v", err)
	}

	source := store.Source{
		DeviceMAC: ctx.MAC,
		Timestamp: time.Now(),
		Method:    "module_read",
		Filename:  filename,
	}

	hash, isNew, err := s.Import(data, source)
	if err != nil {
		log.Fatalf("Failed to save to store: %v", err)
	}

	shortHash := store.ShortHash(hash)
	if isNew {
		fmt.Printf("Saved to store: %s (new)\n", shortHash)
	} else {
		fmt.Printf("Saved to store: %s (existing profile)\n", shortHash)
	}

	// Optionally save to file
	if filename != "" {
		if err := os.WriteFile(filename, data, 0o644); err != nil {
			log.Fatalf("Failed to write file: %v", err)
		}
		fmt.Printf("Saved to file: %s\n", filename)
	}

	// Display info about the data
	DisplayEEPROMInfo(data)
}

// ModuleReadData reads EEPROM from the physical module and returns the data.
// This is the low-level function used by both CLI and TUI.
func ModuleReadData(ctx *ble.APIContext) ([]byte, error) {
	// Step 1: GET /xsfp/module/start to initialize read and get size
	resp, body, err := ctx.SendRequest("GET", ctx.APIPath("/xsfp/module/start"), nil, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize: %w", err)
	}

	if resp.StatusCode != 200 {
		if len(body) > 0 {
			return nil, fmt.Errorf("error initializing: status %d: %s", resp.StatusCode, string(body))
		}
		return nil, fmt.Errorf("error initializing: status %d", resp.StatusCode)
	}

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
	reqBody := fmt.Sprintf(`{"offset":0,"chunk":%d}`, startResp.Size)
	resp, body, err = ctx.SendRequest("GET", ctx.APIPath("/xsfp/module/data"), []byte(reqBody), 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to read module data: %w", err)
	}

	if resp.StatusCode != 200 {
		if len(body) > 0 {
			return nil, fmt.Errorf("error reading module data: status %d: %s", resp.StatusCode, string(body))
		}
		return nil, fmt.Errorf("error reading module data: status %d", resp.StatusCode)
	}

	return body, nil
}

// DDMStart calls /ddm/start and /ddm/data endpoints to fetch DDM data.
// This is experimental - the response format is being explored.
func DDMStart(device bluetooth.Device) {
	ctx := ble.SetupAPI(device)

	fmt.Println("Calling /ddm/start...")

	resp, body, err := ctx.SendRequest("GET", ctx.APIPath("/ddm/start"), nil, 10*time.Second)
	if err != nil {
		log.Fatal("API request failed:", err)
	}

	fmt.Printf("Status: %d\n", resp.StatusCode)

	if resp.StatusCode != 200 {
		if len(body) > 0 {
			fmt.Printf("Body: %s\n", string(body))
		}
		return
	}

	// Parse start response
	var startResp struct {
		Size  int `json:"size"`
		Chunk int `json:"chunk"`
	}
	if err := json.Unmarshal(body, &startResp); err != nil {
		fmt.Printf("Start response (raw): %s\n", string(body))
		return
	}

	fmt.Printf("Start response: size=%d, chunk=%d\n", startResp.Size, startResp.Chunk)

	// Determine how much to request - use chunk size if size is 0
	requestSize := startResp.Size
	if requestSize == 0 {
		requestSize = startResp.Chunk
	}

	// Now fetch data from /ddm/data
	fmt.Println("\nCalling /ddm/data...")
	reqBody := fmt.Sprintf(`{"offset":0,"chunk":%d}`, requestSize)
	resp, body, err = ctx.SendRequest("GET", ctx.APIPath("/ddm/data"), []byte(reqBody), 60*time.Second)
	if err != nil {
		log.Fatal("API request failed:", err)
	}

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Received %d bytes\n", len(body))

	if len(body) == 0 {
		fmt.Println("(empty response body)")
		return
	}

	// Try to pretty print as JSON first
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, body, "", "  "); err != nil {
		// Not JSON - check if it's text (CSV)
		if isTextData(body) {
			fmt.Printf("\n%s", string(body))
		} else {
			// Binary data - show hex dump
			fmt.Printf("Body (hex):\n")
			for i := 0; i < len(body); i += 16 {
				end := i + 16
				if end > len(body) {
					end = len(body)
				}
				fmt.Printf("%04x: % x\n", i, body[i:end])
			}
		}
	} else {
		fmt.Println(prettyJSON.String())
	}
}

// isTextData checks if the data appears to be printable text.
func isTextData(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	textChars := 0
	for _, b := range data {
		// Allow printable ASCII, newlines, tabs, and UTF-8 continuation bytes
		if (b >= 0x20 && b <= 0x7e) || b == '\n' || b == '\r' || b == '\t' || b >= 0x80 {
			textChars++
		}
	}
	// Consider it text if >90% of bytes are text-like
	return float64(textChars)/float64(len(data)) > 0.9
}
