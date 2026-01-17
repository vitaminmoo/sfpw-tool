package commands

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/vitaminmoo/sfpw-tool/internal/ble"

	"tinygo.org/x/bluetooth"
)

// PrintJSON pretty-prints JSON data. If indentation fails, prints raw.
func PrintJSON(data []byte) {
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, data, "", "  "); err != nil {
		fmt.Printf("Body: %s\n", string(data))
	} else {
		fmt.Println(prettyJSON.String())
	}
}

// GetAndDisplayJSON fetches an endpoint and displays the response as pretty JSON.
// This is the most common pattern in the codebase.
func GetAndDisplayJSON(device bluetooth.Device, endpoint string) {
	ctx := ble.SetupAPI(device)

	resp, body, err := ble.SendAPIRequest(ctx.WriteChar, ctx.NotifyChar, "GET", ctx.APIPath(endpoint), nil)
	if err != nil {
		log.Fatal("API request failed:", err)
	}

	if resp.StatusCode != 200 {
		fmt.Printf("Error: status %d\n", resp.StatusCode)
		fmt.Printf("Body: %s\n", string(body))
		return
	}

	PrintJSON(body)
}

// DisplayEEPROMInfo shows a compact summary of SFP module info from EEPROM data.
func DisplayEEPROMInfo(data []byte) {
	if len(data) < 96 {
		return
	}

	vendorName := strings.TrimSpace(string(data[20:36]))
	vendorPN := strings.TrimSpace(string(data[40:56]))
	vendorSN := strings.TrimSpace(string(data[68:84]))
	fmt.Printf("\nModule info:\n")
	fmt.Printf("  Vendor: %s\n", vendorName)
	fmt.Printf("  Part:   %s\n", vendorPN)
	fmt.Printf("  S/N:    %s\n", vendorSN)
}

// FetchAndSaveData performs the common start/data fetch pattern and saves to file.
// Used by module-read and snapshot-read.
func FetchAndSaveData(ctx *ble.APIContext, startEndpoint, dataEndpoint, filename string) ([]byte, error) {
	// Step 1: GET start endpoint to initialize and get size
	fmt.Println("Initializing read...")
	resp, body, err := ctx.SendRequest("GET", ctx.APIPath(startEndpoint), nil, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize: %w", err)
	}

	if resp.StatusCode != 200 {
		if len(body) > 0 {
			return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
		}
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	fmt.Printf("Info: %s\n", string(body))

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

	// Step 2: GET data endpoint to read the data
	fmt.Println("Reading data...")
	reqBody := fmt.Sprintf(`{"offset":0,"chunk":%d}`, startResp.Size)
	resp, body, err = ctx.SendRequest("GET", ctx.APIPath(dataEndpoint), []byte(reqBody), 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to read data: %w", err)
	}

	if resp.StatusCode != 200 {
		if len(body) > 0 {
			return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
		}
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	fmt.Printf("Received %d bytes\n", len(body))

	// Save to file
	if err := os.WriteFile(filename, body, 0o644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}
	fmt.Printf("Saved to: %s\n", filename)

	return body, nil
}

// AbortSIFIfRunning checks SIF status and aborts if an operation is in progress.
func AbortSIFIfRunning(ctx *ble.APIContext) error {
	resp, body, err := ctx.SendRequest("GET", ctx.APIPath("/sif/info/"), nil, 10*time.Second)
	if err != nil {
		return fmt.Errorf("failed to get SIF status: %w", err)
	}

	var statusResp struct {
		Status string `json:"status"`
		Offset int    `json:"offset"`
	}

	if resp.StatusCode == 200 {
		if err := json.Unmarshal(body, &statusResp); err == nil {
			// Only abort if actively in progress (not finished/complete/idle)
			if statusResp.Status == "inprogress" || statusResp.Status == "ready" || statusResp.Status == "continue" {
				fmt.Printf("SIF operation in progress (status=%s), aborting...\n", statusResp.Status)
				resp, _, err := ctx.SendRequest("POST", ctx.APIPath("/sif/abort"), nil, 10*time.Second)
				if err != nil {
					return fmt.Errorf("failed to abort SIF: %w", err)
				}
				if resp.StatusCode != 200 {
					return fmt.Errorf("failed to abort SIF: status %d", resp.StatusCode)
				}
				fmt.Println("Previous SIF operation aborted")
				time.Sleep(500 * time.Millisecond)
			}
		}
	}

	return nil
}

// ConfirmAction prompts the user to type 'yes' to continue.
// Returns true if confirmed, false otherwise.
func ConfirmAction(prompt string) bool {
	fmt.Print(prompt)

	reader := bufio.NewReader(os.Stdin)
	confirm, _ := reader.ReadString('\n')
	confirm = strings.TrimSpace(confirm)

	return confirm == "yes"
}
