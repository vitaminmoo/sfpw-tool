package commands

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/vitaminmoo/sfpw-tool/internal/ble"
	"github.com/vitaminmoo/sfpw-tool/internal/eeprom"

	"tinygo.org/x/bluetooth"
)

// SupportDump downloads support info archive via SIF protocol
// Contains syslog, module database entries, and cached EEPROM snapshots
func SupportDump(device bluetooth.Device) {
	ctx := ble.SetupAPI(device)

	// Step 0: Check current SIF status and abort if in progress
	fmt.Println("Checking SIF status...")
	if err := AbortSIFIfRunning(ctx); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Starting SIF read operation...")

	// Step 1: POST /sif/start to initiate
	resp, body, err := ctx.SendRequest("POST", ctx.APIPath("/sif/start"), nil, 10*time.Second)
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
		resp, body, err := ctx.SendRequest("GET", ctx.APIPath("/sif/data/"), []byte(reqBody), 30*time.Second)
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
	resp, body, err = ctx.SendRequest("GET", ctx.APIPath("/sif/info/"), nil, 10*time.Second)
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
	if err := os.WriteFile(filename, eepromData, 0o644); err != nil {
		log.Fatal("Failed to save file:", err)
	}
	fmt.Printf("\nSaved to: %s\n", filename)
}

// Logs downloads the support archive and outputs the syslog to stdout
func Logs(device bluetooth.Device) {
	ctx := ble.SetupAPI(device)

	// Check current SIF status and abort if in progress
	if err := AbortSIFIfRunning(ctx); err != nil {
		log.Fatal(err)
	}

	// Start SIF read
	resp, body, err := ctx.SendRequest("POST", ctx.APIPath("/sif/start"), nil, 10*time.Second)
	if err != nil {
		log.Fatal("Failed to start SIF read:", err)
	}

	if resp.StatusCode != 200 {
		log.Fatalf("Error starting SIF: status %d", resp.StatusCode)
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

	// Read all data
	archiveData := make([]byte, 0, startResp.Size)
	offset := 0
	chunkSize := startResp.Chunk

	for offset < startResp.Size {
		remaining := startResp.Size - offset
		if remaining < chunkSize {
			chunkSize = remaining
		}

		reqBody := fmt.Sprintf(`{"status":"continue","offset":%d,"chunk":%d}`, offset, chunkSize)
		resp, body, err := ctx.SendRequest("GET", ctx.APIPath("/sif/data/"), []byte(reqBody), 30*time.Second)
		if err != nil {
			log.Fatal("Failed to read SIF data:", err)
		}

		if resp.StatusCode != 200 || len(body) == 0 {
			break
		}

		archiveData = append(archiveData, body...)
		offset += len(body)
	}

	// Extract syslog from tar archive
	tr := tar.NewReader(bytes.NewReader(archiveData))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal("Error reading tar:", err)
		}

		if hdr.Name == "syslog" {
			syslogData, err := io.ReadAll(tr)
			if err != nil {
				log.Fatal("Error reading syslog:", err)
			}
			fmt.Print(string(syslogData))
			return
		}
	}

	fmt.Println("No syslog found in archive")
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
				eeprom.ParseInfo(eepromData)
			}
		}
	}
}
