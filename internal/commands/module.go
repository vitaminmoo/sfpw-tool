package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"sfpw-tool/internal/ble"

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

// ModuleRead reads EEPROM from the physical module and saves to file
func ModuleRead(device bluetooth.Device, filename string) {
	ctx := ble.SetupAPI(device)

	// Step 1: GET /xsfp/module/start to initialize read and get size
	fmt.Println("Initializing module read...")
	resp, body, err := ctx.SendRequest("GET", ctx.APIPath("/xsfp/module/start"), nil, 10*time.Second)
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
	resp, body, err = ctx.SendRequest("GET", ctx.APIPath("/xsfp/module/data"), []byte(reqBody), 30*time.Second)
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
