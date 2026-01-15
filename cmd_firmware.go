package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"tinygo.org/x/bluetooth"
)

// FirmwareStatus represents the response from GET /fw
type FirmwareStatus struct {
	HWVersion       int    `json:"hwv"`
	FWVersion       string `json:"fwv"`
	IsUpdating      bool   `json:"isUPdating"` // Note: typo in API
	Status          string `json:"status"`
	ProgressPercent int    `json:"progressPercent"`
	RemainingTime   int    `json:"remainingTime"`
}

// cmdFirmwareUpdate uploads and installs new firmware
func cmdFirmwareUpdate(device bluetooth.Device, filename string) {
	ctx := setupAPI(device)

	// Read the firmware file
	fwData, err := os.ReadFile(filename)
	if err != nil {
		log.Fatalf("Failed to read firmware file: %v", err)
	}

	fmt.Printf("Loaded firmware file: %d bytes from %s\n", len(fwData), filename)

	// Get current firmware status
	fmt.Println("Checking current firmware status...")
	status, err := getFirmwareStatus(ctx)
	if err != nil {
		log.Fatalf("Failed to get firmware status: %v", err)
	}

	fmt.Printf("Current firmware: v%s (hw: %d)\n", status.FWVersion, status.HWVersion)
	fmt.Printf("Update status: %s\n", status.Status)

	if status.IsUpdating {
		fmt.Println("WARNING: A firmware update is already in progress!")
		fmt.Print("Abort existing update and start new one? (yes/no): ")
		reader := bufio.NewReader(os.Stdin)
		confirm, _ := reader.ReadString('\n')
		confirm = strings.TrimSpace(confirm)
		if confirm != "yes" {
			fmt.Println("Aborted.")
			return
		}

		// Abort existing update
		fmt.Println("Aborting existing update...")
		if err := abortFirmwareUpdate(ctx); err != nil {
			log.Fatalf("Failed to abort existing update: %v", err)
		}
		time.Sleep(1 * time.Second)
	}

	fmt.Println()
	fmt.Println("WARNING: Firmware update is a potentially dangerous operation!")
	fmt.Println("Do not disconnect power or BLE during the update.")
	fmt.Printf("File size: %d bytes\n", len(fwData))
	fmt.Print("Type 'yes' to start firmware update: ")

	reader := bufio.NewReader(os.Stdin)
	confirm, _ := reader.ReadString('\n')
	confirm = strings.TrimSpace(confirm)
	if confirm != "yes" {
		fmt.Println("Aborted.")
		return
	}

	// Step 1: Start firmware update
	fmt.Println("\nStarting firmware update...")
	startResp, err := startFirmwareUpdate(ctx, len(fwData))
	if err != nil {
		log.Fatalf("Failed to start firmware update: %v", err)
	}
	debugf("Start response: %+v", startResp)

	// Determine chunk size (use what device tells us, or default)
	chunkSize := 512 // default
	if startResp.Chunk > 0 {
		chunkSize = startResp.Chunk
	}
	debugf("Using chunk size: %d bytes", chunkSize)

	// Step 2: Send firmware data in chunks
	fmt.Printf("Uploading firmware (%d bytes in %d-byte chunks)...\n", len(fwData), chunkSize)

	totalChunks := (len(fwData) + chunkSize - 1) / chunkSize
	for offset := 0; offset < len(fwData); offset += chunkSize {
		end := offset + chunkSize
		if end > len(fwData) {
			end = len(fwData)
		}
		chunk := fwData[offset:end]
		chunkNum := (offset / chunkSize) + 1

		// Send chunk
		err := sendFirmwareChunk(ctx, chunk, offset)
		if err != nil {
			fmt.Printf("\nFailed to send chunk at offset %d: %v\n", offset, err)
			fmt.Println("Aborting firmware update...")
			abortFirmwareUpdate(ctx)
			return
		}

		// Progress bar
		progress := float64(offset+len(chunk)) / float64(len(fwData)) * 100
		fmt.Printf("\r  Chunk %d/%d: %d-%d bytes (%.1f%%)", chunkNum, totalChunks, offset, end, progress)

		// Small delay between chunks
		time.Sleep(20 * time.Millisecond)
	}
	fmt.Println()

	// Step 3: Monitor update progress
	fmt.Println("Firmware uploaded. Monitoring installation progress...")

	for {
		time.Sleep(2 * time.Second)

		status, err := getFirmwareStatus(ctx)
		if err != nil {
			// Connection may drop during update - that's often expected
			fmt.Printf("Status check failed (connection may have dropped): %v\n", err)
			fmt.Println("The device may be rebooting. Please check device status manually.")
			return
		}

		fmt.Printf("\r  Status: %s, Progress: %d%%, Remaining: %ds    ",
			status.Status, status.ProgressPercent, status.RemainingTime)

		if !status.IsUpdating {
			fmt.Println()
			if status.Status == "finished" || status.Status == "complete" {
				fmt.Printf("Firmware update complete! New version: v%s\n", status.FWVersion)
			} else {
				fmt.Printf("Update finished with status: %s\n", status.Status)
			}
			break
		}
	}
}

// FirmwareStartResponse represents the response from POST /fw/start
type FirmwareStartResponse struct {
	Status string `json:"status"`
	Offset int    `json:"offset"`
	Chunk  int    `json:"chunk"`
	Size   int    `json:"size"`
}

// startFirmwareUpdate initiates a firmware update
func startFirmwareUpdate(ctx *APIContext, size int) (*FirmwareStartResponse, error) {
	// Send size as JSON body
	reqBody := fmt.Sprintf(`{"size":%d}`, size)

	resp, body, err := ctx.sendRequest("POST", ctx.apiPath("/fw/start"), []byte(reqBody), 10*time.Second)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var startResp FirmwareStartResponse
	if err := json.Unmarshal(body, &startResp); err != nil {
		// Return with defaults if we can't parse
		debugf("Could not parse start response: %v, body: %s", err, string(body))
		return &FirmwareStartResponse{Status: "ready"}, nil
	}

	return &startResp, nil
}

// sendFirmwareChunk sends a chunk of firmware data
func sendFirmwareChunk(ctx *APIContext, chunk []byte, offset int) error {
	resp, body, err := ctx.sendRawBodyRequest("POST", ctx.apiPath("/fw/data"), chunk, 30*time.Second)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	debugf("Chunk at offset %d sent successfully", offset)
	return nil
}

// getFirmwareStatus gets the current firmware update status
func getFirmwareStatus(ctx *APIContext) (*FirmwareStatus, error) {
	resp, body, err := ctx.sendRequest("GET", ctx.apiPath("/fw"), nil, 10*time.Second)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var status FirmwareStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, fmt.Errorf("failed to parse status: %w", err)
	}

	return &status, nil
}

// abortFirmwareUpdate aborts an in-progress firmware update
func abortFirmwareUpdate(ctx *APIContext) error {
	resp, body, err := ctx.sendRequest("POST", ctx.apiPath("/fw/abort"), nil, 10*time.Second)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// cmdFirmwareAbort aborts an in-progress firmware update
func cmdFirmwareAbort(device bluetooth.Device) {
	ctx := setupAPI(device)

	fmt.Println("Checking firmware status...")
	status, err := getFirmwareStatus(ctx)
	if err != nil {
		log.Fatalf("Failed to get firmware status: %v", err)
	}

	if !status.IsUpdating {
		fmt.Println("No firmware update in progress.")
		return
	}

	fmt.Printf("Update in progress: %d%% complete, status: %s\n", status.ProgressPercent, status.Status)
	fmt.Print("Abort update? (yes/no): ")

	reader := bufio.NewReader(os.Stdin)
	confirm, _ := reader.ReadString('\n')
	confirm = strings.TrimSpace(confirm)
	if confirm != "yes" {
		fmt.Println("Cancelled.")
		return
	}

	fmt.Println("Aborting firmware update...")
	if err := abortFirmwareUpdate(ctx); err != nil {
		log.Fatalf("Failed to abort: %v", err)
	}

	fmt.Println("Firmware update aborted.")
}

// cmdFirmwareStatus shows detailed firmware status
func cmdFirmwareStatus(device bluetooth.Device) {
	ctx := setupAPI(device)

	resp, body, err := ctx.sendRequest("GET", ctx.apiPath("/fw"), nil, 10*time.Second)
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
