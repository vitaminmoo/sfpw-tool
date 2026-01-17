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
