package commands

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"sfpw-tool/internal/ble"

	"tinygo.org/x/bluetooth"
)

// SnapshotInfo gets info about the snapshot buffer
func SnapshotInfo(device bluetooth.Device) {
	ctx := ble.SetupAPI(device)

	fmt.Println("Getting snapshot info...")

	resp, body, err := ctx.SendRequest("GET", ctx.APIPath("/xsfp/sync/start"), nil, 10*time.Second)
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

	PrintJSON(body)
}

// SnapshotRead reads the snapshot buffer and saves to file
func SnapshotRead(device bluetooth.Device, filename string) {
	ctx := ble.SetupAPI(device)

	// Step 1: GET /xsfp/sync/start to initialize and get size
	fmt.Println("Initializing snapshot read...")
	resp, body, err := ctx.SendRequest("GET", ctx.APIPath("/xsfp/sync/start"), nil, 10*time.Second)
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
	resp, body, err = ctx.SendRequest("GET", ctx.APIPath("/xsfp/sync/data"), []byte(reqBody), 30*time.Second)
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
	DisplayEEPROMInfo(body)
}

// SnapshotWrite writes EEPROM data to the snapshot buffer
// Use device screen to apply snapshot to physical module
func SnapshotWrite(device bluetooth.Device, filename string) {
	ctx := ble.SetupAPI(device)

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
	DisplayEEPROMInfo(eepromData)

	fmt.Println()
	fmt.Println("This will write to the snapshot buffer.")
	fmt.Println("Use the device screen to apply snapshot to module.")
	if !ConfirmAction("Type 'yes' to continue: ") {
		fmt.Println("Aborted.")
		return
	}

	// Step 1: POST /xsfp/sync/start with size
	fmt.Println("\nInitializing snapshot write...")
	startBody := fmt.Sprintf(`{"size":%d}`, len(eepromData))
	resp, body, err := ctx.SendRequest("POST", ctx.APIPath("/xsfp/sync/start"), []byte(startBody), 10*time.Second)
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
	resp, body, err = ctx.SendRawBodyRequest("POST", ctx.APIPath("/xsfp/sync/data"), eepromData, 30*time.Second)
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
		PrintJSON(body)
	}

	fmt.Println("\nUse the device screen to apply snapshot to module.")
}
