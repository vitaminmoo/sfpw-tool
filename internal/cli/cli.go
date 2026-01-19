package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/vitaminmoo/sfpw-tool/internal/ble"
	"github.com/vitaminmoo/sfpw-tool/internal/commands"
	"github.com/vitaminmoo/sfpw-tool/internal/config"
	"github.com/vitaminmoo/sfpw-tool/internal/firmware"
	"github.com/vitaminmoo/sfpw-tool/internal/store"
	"github.com/vitaminmoo/sfpw-tool/internal/tui"
)

// CLI is the root command structure for sfpw.
type CLI struct {
	Verbose bool `short:"v" help:"Enable verbose debug output"`

	// TUI command (work in progress)
	Tui TuiCmd `cmd:"" help:"Launch interactive TUI (work in progress)"`

	Device   DeviceCmd   `cmd:"" help:"Device info and control"`
	Module   ModuleCmd   `cmd:"" help:"SFP module operations"`
	Snapshot SnapshotCmd `cmd:"" help:"Snapshot buffer operations"`
	Fw       FwCmd       `cmd:"" help:"Firmware operations"`
	Support  SupportCmd  `cmd:"" help:"Support and diagnostics"`
	Store    StoreCmd    `cmd:"" help:"Module profile store"`
	Debug    DebugCmd    `cmd:"" help:"Debug and development tools"`

	// Legacy commands for backwards compatibility (hidden)
	Version        VersionCmd             `cmd:"" hidden:""`
	APIVersion     APIVersionCmd          `cmd:"" name:"api-version" hidden:""`
	Stats          StatsCmd               `cmd:"" hidden:""`
	Info           InfoCmd                `cmd:"" hidden:""`
	Settings       SettingsCmd            `cmd:"" hidden:""`
	Bt             BtCmd                  `cmd:"" hidden:""`
	Logs           LogsCmd                `cmd:"" hidden:""`
	Reboot         RebootCmd              `cmd:"" hidden:""`
	Explore        ExploreCmd             `cmd:"" hidden:""`
	ModuleInfo     ModuleInfoLegacyCmd    `cmd:"" name:"module-info" hidden:""`
	ModuleRead     ModuleReadLegacyCmd    `cmd:"" name:"module-read" hidden:""`
	SnapshotInfo   SnapshotInfoLegacyCmd  `cmd:"" name:"snapshot-info" hidden:""`
	SnapshotRead   SnapshotReadLegacyCmd  `cmd:"" name:"snapshot-read" hidden:""`
	SnapshotWrite  SnapshotWriteLegacyCmd `cmd:"" name:"snapshot-write" hidden:""`
	FwUpdate       FwUpdateLegacyCmd      `cmd:"" name:"fw-update" hidden:""`
	FwAbort        FwAbortLegacyCmd       `cmd:"" name:"fw-abort" hidden:""`
	FwStatusLegacy FwStatusLegacyCmd      `cmd:"" name:"fw-status" hidden:""`
	SupportDump    SupportDumpLegacyCmd   `cmd:"" name:"support-dump" hidden:""`
	ParseEeprom    ParseEepromLegacyCmd   `cmd:"" name:"parse-eeprom" hidden:""`
	TestEncode     TestEncodeLegacyCmd    `cmd:"" name:"test-encode" hidden:""`
	TestPackets    TestPacketsLegacyCmd   `cmd:"" name:"test-packets" hidden:""`
}

// --- TUI Command ---

type TuiCmd struct{}

func (c *TuiCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	return tui.Run()
}

// --- Device Commands ---

type DeviceCmd struct {
	Info     DeviceInfoCmd     `cmd:"" help:"Get device info"`
	Stats    DeviceStatsCmd    `cmd:"" help:"Get device statistics (battery, signal, uptime)"`
	Settings DeviceSettingsCmd `cmd:"" help:"Get device settings"`
	Bt       DeviceBtCmd       `cmd:"" help:"Get bluetooth parameters"`
	Version  DeviceVersionCmd  `cmd:"" help:"Read device info from BLE characteristic"`
	Reboot   DeviceRebootCmd   `cmd:"" help:"Reboot the device"`
}

type DeviceInfoCmd struct{}

func (c *DeviceInfoCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.Info(device)
	return nil
}

type DeviceStatsCmd struct{}

func (c *DeviceStatsCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.Stats(device)
	return nil
}

type DeviceSettingsCmd struct{}

func (c *DeviceSettingsCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.Settings(device)
	return nil
}

type DeviceBtCmd struct{}

func (c *DeviceBtCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.Bluetooth(device)
	return nil
}

type DeviceVersionCmd struct{}

func (c *DeviceVersionCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.Version(device)
	return nil
}

type DeviceRebootCmd struct{}

func (c *DeviceRebootCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.Reboot(device)
	return nil
}

// --- Module Commands ---

type ModuleCmd struct {
	Info ModuleInfoCmd   `cmd:"" help:"Get details about the inserted SFP module"`
	Read ModuleReadCmd   `cmd:"" help:"Read EEPROM from physical module to file"`
	Ddm  ModuleDdmCmd    `cmd:"" help:"Read DDM (Digital Diagnostic Monitoring) data"`
}

type ModuleInfoCmd struct{}

func (c *ModuleInfoCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.ModuleInfo(device)
	return nil
}

type ModuleReadCmd struct {
	Output string `arg:"" optional:"" help:"Output file path (optional; always saves to store)"`
}

func (c *ModuleReadCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.ModuleRead(device, c.Output)
	return nil
}

type ModuleDdmCmd struct{}

func (c *ModuleDdmCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.DDM(device)
	return nil
}

// --- Snapshot Commands ---

type SnapshotCmd struct {
	Info  SnapshotInfoCmd  `cmd:"" help:"Get snapshot buffer status"`
	Read  SnapshotReadCmd  `cmd:"" help:"Read snapshot buffer to file"`
	Write SnapshotWriteCmd `cmd:"" help:"Write EEPROM file to snapshot buffer"`
}

type SnapshotInfoCmd struct{}

func (c *SnapshotInfoCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.SnapshotInfo(device)
	return nil
}

type SnapshotReadCmd struct {
	Output string `arg:"" optional:"" help:"Output file path (optional; always saves to store)"`
}

func (c *SnapshotReadCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.SnapshotRead(device, c.Output)
	return nil
}

type SnapshotWriteCmd struct {
	FileOrProfile string `arg:"" help:"EEPROM file path or store profile hash"`
}

func (c *SnapshotWriteCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose

	// Check if it's a file path or a store profile hash
	filePath := c.FileOrProfile
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// Not a file, try to find in store
		s, err := store.OpenDefault()
		if err != nil {
			return fmt.Errorf("failed to open store: %w", err)
		}

		// Look for matching profile hash
		profiles, err := s.ListWithHashes()
		if err != nil {
			return fmt.Errorf("failed to list profiles: %w", err)
		}

		var fullHash string
		for hash := range profiles {
			if hash == c.FileOrProfile || store.ShortHash(hash) == c.FileOrProfile || hash[7:] == c.FileOrProfile {
				fullHash = hash
				break
			}
		}

		if fullHash == "" {
			return fmt.Errorf("not found: %s (not a file or store profile)", c.FileOrProfile)
		}

		// Export to temp file
		tmpFile, err := os.CreateTemp("", "sfpw-eeprom-*.bin")
		if err != nil {
			return fmt.Errorf("failed to create temp file: %w", err)
		}
		tmpPath := tmpFile.Name()
		tmpFile.Close()
		defer os.Remove(tmpPath)

		if err := s.Export(fullHash, tmpPath); err != nil {
			return fmt.Errorf("failed to export profile: %w", err)
		}

		entry := profiles[fullHash]
		fmt.Printf("Using store profile: %s (%s %s)\n", store.ShortHash(fullHash), entry.VendorName, entry.PartNumber)
		filePath = tmpPath
	}

	device := ble.Connect()
	defer device.Disconnect()
	commands.SnapshotWrite(device, filePath)
	return nil
}

// --- Firmware Commands ---

type FwCmd struct {
	Status   FwStatusCmd   `cmd:"" help:"Get detailed firmware status"`
	Update   FwUpdateCmd   `cmd:"" help:"Upload and install firmware (from file or downloaded version)"`
	Abort    FwAbortCmd    `cmd:"" help:"Abort an in-progress firmware update"`
	Download FwDownloadCmd `cmd:"" help:"Download all available firmware versions from the internet"`
	List     FwListCmd     `cmd:"" help:"List downloaded firmware files"`
	Path     FwPathCmd     `cmd:"" help:"Show firmware storage directory path"`
	Passdb   FwPassdbCmd   `cmd:"" help:"Extract password database from firmware image"`
}

type FwStatusCmd struct{}

func (c *FwStatusCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.FirmwareStatusCmd(device)
	return nil
}

type FwUpdateCmd struct {
	FileOrVersion string `arg:"" help:"Firmware file path or downloaded version (e.g., v1.1.1)"`
}

func (c *FwUpdateCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose

	// Check if it's a file path or a version string
	filePath := c.FileOrVersion
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// Not a file, try to find in firmware store
		store, err := firmware.NewFirmwareStore()
		if err != nil {
			return fmt.Errorf("failed to open firmware store: %w", err)
		}

		// Look for matching version
		entries, err := store.List()
		if err != nil {
			return fmt.Errorf("failed to list firmware: %w", err)
		}

		version := c.FileOrVersion
		// Try with and without 'v' prefix
		for _, e := range entries {
			if e.Version == version || e.Version == "v"+version || "v"+e.Version == version {
				filePath = e.Path
				fmt.Printf("Using downloaded firmware: %s\n", e.Version)
				break
			}
		}

		if filePath == c.FileOrVersion {
			return fmt.Errorf("firmware not found: %s (not a file or downloaded version)", c.FileOrVersion)
		}
	}

	device := ble.Connect()
	defer device.Disconnect()
	commands.FirmwareUpdate(device, filePath)
	return nil
}

type FwAbortCmd struct{}

func (c *FwAbortCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.FirmwareAbort(device)
	return nil
}

type FwListCmd struct{}

func (c *FwListCmd) Run(globals *CLI) error {
	store, err := firmware.NewFirmwareStore()
	if err != nil {
		return err
	}

	entries, err := store.List()
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		fmt.Println("No firmware files downloaded.")
		fmt.Println("Download firmware with: sfpw fw download")
		return nil
	}

	fmt.Printf("Downloaded firmware files (%d):\n\n", len(entries))
	for _, e := range entries {
		fmt.Printf("  %-12s  %-10s  %s\n",
			e.Version,
			humanizeBytes(e.FileSize),
			e.Downloaded.Format("2006-01-02 15:04"))
	}

	return nil
}

type FwPathCmd struct{}

func (c *FwPathCmd) Run(globals *CLI) error {
	store, err := firmware.NewFirmwareStore()
	if err != nil {
		return err
	}
	fmt.Println(store.Path())
	return nil
}

type FwDownloadCmd struct{}

func (c *FwDownloadCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose

	manifest := firmware.NewManifestClient()
	versions, err := manifest.GetAvailable(firmware.DefaultSFPWizardFilter())
	if err != nil {
		return err
	}

	if len(versions) == 0 {
		fmt.Println("No firmware versions available from cloud.")
		return nil
	}

	fmt.Printf("Found %d firmware version(s) available:\n\n", len(versions))

	store, err := firmware.NewFirmwareStore()
	if err != nil {
		return err
	}

	downloaded := 0
	skipped := 0
	for _, v := range versions {
		// Check if already downloaded
		if store.Has(v.Version, v.SHA256) {
			fmt.Printf("  %s: already downloaded\n", v.Version)
			skipped++
			continue
		}

		fmt.Printf("  %s: downloading...", v.Version)
		progressBar := &CLIProgressBar{width: 30}
		_, err := store.Download(v, func(current, total int64, desc string) {
			progressBar.Update(current, total, "")
		})
		progressBar.Complete()

		if err != nil {
			fmt.Printf(" error: %v\n", err)
			continue
		}
		downloaded++
	}

	fmt.Printf("\nDownloaded %d, skipped %d (already present)\n", downloaded, skipped)
	return nil
}

type FwPassdbCmd struct {
	FileOrVersion string `arg:"" help:"Firmware file path or downloaded version (e.g., v1.1.1)"`
	JSON          bool   `help:"Output as JSON" short:"j"`
	Search        string `help:"Emulate firmware lookup for part number (exact match, shows passwords that would be tried)" short:"s"`
}

func (c *FwPassdbCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose

	// Check if it's a file path or a version string
	filePath := c.FileOrVersion
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// Not a file, try to find in firmware store
		store, err := firmware.NewFirmwareStore()
		if err != nil {
			return fmt.Errorf("failed to open firmware store: %w", err)
		}

		// Look for matching version
		entries, err := store.List()
		if err != nil {
			return fmt.Errorf("failed to list firmware: %w", err)
		}

		version := c.FileOrVersion
		// Try with and without 'v' prefix
		for _, e := range entries {
			if e.Version == version || e.Version == "v"+version || "v"+e.Version == version {
				filePath = e.Path
				fmt.Printf("Using downloaded firmware: %s\n", e.Version)
				break
			}
		}

		if filePath == c.FileOrVersion {
			return fmt.Errorf("firmware not found: %s (not a file or downloaded version)", c.FileOrVersion)
		}
	}

	// Parse the firmware image
	img, err := firmware.ParseESP32Image(filePath)
	if err != nil {
		return fmt.Errorf("failed to parse firmware: %w", err)
	}

	// Extract password database
	db, err := firmware.ExtractPasswordDatabase(img)
	if err != nil {
		return fmt.Errorf("failed to extract password database: %w", err)
	}

	// Search mode: emulate firmware lookup
	if c.Search != "" {
		return c.runSearch(db)
	}

	// Full database listing
	entries := db.Entries

	if c.JSON {
		data, _ := json.MarshalIndent(struct {
			Version   string                   `json:"version"`
			EntrySize int                      `json:"entry_size"`
			Count     int                      `json:"count"`
			Entries   []firmware.PasswordEntry `json:"entries"`
		}{
			Version:   db.Version,
			EntrySize: db.EntrySize,
			Count:     len(entries),
			Entries:   entries,
		}, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// Table output
	fmt.Printf("Password Database: %s\n", db.Version)
	fmt.Printf("Entry size: %d bytes\n", db.EntrySize)
	fmt.Printf("Total entries: %d\n\n", len(entries))

	// Print unique passwords first
	fmt.Println("Unique Passwords:")
	fmt.Println(strings.Repeat("-", 50))
	for _, pw := range db.UniquePasswords() {
		ascii := ""
		allPrintable := true
		for _, b := range pw {
			if b < 0x20 || b > 0x7e {
				allPrintable = false
				break
			}
		}
		if allPrintable {
			ascii = fmt.Sprintf(" (%q)", string(pw[:]))
		}
		fmt.Printf("  %02x %02x %02x %02x%s\n", pw[0], pw[1], pw[2], pw[3], ascii)
	}
	fmt.Println()

	// Print all entries
	fmt.Println("Part Number Mapping:")
	fmt.Println(strings.Repeat("-", 115))
	fmt.Printf("  %-28s  %-11s  %-6s  %-6s  %-4s  %s\n", "Part Number", "Password", "ASCII", "Locked", "RO", "Writable Pages")
	fmt.Println(strings.Repeat("-", 115))

	for _, entry := range entries {
		locked := "No"
		if entry.Locked {
			locked = "Yes"
		}
		ro := "No"
		if entry.ReadOnly {
			ro = "Yes"
		}
		ascii := entry.FormatPasswordASCII()
		if ascii != "" {
			ascii = fmt.Sprintf("%q", ascii)
		}
		fmt.Printf("  %-28s  %-11s  %-6s  %-6s  %-4s  %s\n",
			entry.PartNumber, entry.FormatPassword(), ascii, locked, ro, entry.InterpretFlags())
	}

	return nil
}

// runSearch emulates the firmware's password lookup algorithm.
func (c *FwPassdbCmd) runSearch(db *firmware.PasswordDatabase) error {
	passwordsToTry := db.GetPasswordsToTry(c.Search)
	allMatches := db.FindByPartNumber(c.Search)

	// Count including default if present
	totalPasswords := len(passwordsToTry)
	if db.DefaultEntry != nil {
		totalPasswords++
	}

	if c.JSON {
		// Include default entry in passwords list for JSON
		allPasswords := passwordsToTry
		if db.DefaultEntry != nil {
			allPasswords = append(allPasswords, *db.DefaultEntry)
		}
		data, _ := json.MarshalIndent(struct {
			PartNumber    string                   `json:"part_number"`
			MatchCount    int                      `json:"match_count"`
			PasswordCount int                      `json:"password_count"`
			Passwords     []firmware.PasswordEntry `json:"passwords"`
		}{
			PartNumber:    c.Search,
			MatchCount:    len(allMatches),
			PasswordCount: totalPasswords,
			Passwords:     allPasswords,
		}, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("Firmware Password Lookup Emulation\n")
	fmt.Printf("Part number: %q (exact match)\n", c.Search)
	fmt.Println(strings.Repeat("-", 60))

	if len(allMatches) == 0 {
		fmt.Printf("\nNo entries found for %q\n", c.Search)
	} else {
		fmt.Printf("\nDatabase entries matching %q: %d\n", c.Search, len(allMatches))
		for i, entry := range allMatches {
			ro := ""
			if entry.ReadOnly {
				ro = " (read-only, skipped)"
			}
			pwStr := entry.FormatPassword()
			if ascii := entry.FormatPasswordASCII(); ascii != "" {
				pwStr = fmt.Sprintf("%s  %q", pwStr, ascii)
			}
			fmt.Printf("  %d. %s%s\n", i+1, pwStr, ro)
		}
	}

	fmt.Printf("\nPasswords that would be tried: %d\n", totalPasswords)
	fmt.Println(strings.Repeat("-", 60))

	if totalPasswords == 0 {
		fmt.Println("  (none)")
		return nil
	}

	idx := 1
	for _, entry := range passwordsToTry {
		pwStr := entry.FormatPassword()
		if ascii := entry.FormatPasswordASCII(); ascii != "" {
			pwStr = fmt.Sprintf("%s  %q", pwStr, ascii)
		}
		pages := entry.InterpretFlags()
		if pages != "" && pages != "none" {
			pages = fmt.Sprintf("  pages=%s", pages)
		} else {
			pages = ""
		}
		fmt.Printf("  %d. %s%s\n", idx, pwStr, pages)
		idx++
	}

	// Include default entry in the list
	if db.DefaultEntry != nil {
		pwStr := db.DefaultEntry.FormatPassword()
		if ascii := db.DefaultEntry.FormatPasswordASCII(); ascii != "" {
			pwStr = fmt.Sprintf("%s  %q", pwStr, ascii)
		}
		fmt.Printf("  %d. %s  (default)\n", idx, pwStr)
	}

	return nil
}

// CLIProgressBar renders a terminal-friendly progress bar.
type CLIProgressBar struct {
	width   int
	current int64
	total   int64
}

func (p *CLIProgressBar) Update(current, total int64, desc string) {
	p.current = current
	p.total = total
	if total == 0 {
		fmt.Printf("\r  %s...", desc)
		return
	}
	percent := float64(current) / float64(total) * 100
	filled := int(float64(p.width) * float64(current) / float64(total))
	if filled > p.width {
		filled = p.width
	}
	bar := strings.Repeat("=", filled) + strings.Repeat(" ", p.width-filled)
	fmt.Printf("\r  %s [%s] %.1f%%", desc, bar, percent)
}

func (p *CLIProgressBar) Complete() {
	fmt.Println()
}

func humanizeBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

// --- Support Commands ---

type SupportCmd struct {
	Dump SupportDumpCmd `cmd:"" help:"Download support info archive (syslog, module DB)"`
	Logs SupportLogsCmd `cmd:"" help:"Show device syslog"`
}

type SupportDumpCmd struct{}

func (c *SupportDumpCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.SupportDump(device)
	return nil
}

type SupportLogsCmd struct{}

func (c *SupportLogsCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.Logs(device)
	return nil
}

// --- Debug Commands ---

type DebugCmd struct {
	Explore     DebugExploreCmd     `cmd:"" help:"List all BLE services and characteristics"`
	DumpAll     DebugDumpAllCmd     `cmd:"" name:"dump-all" help:"Dump all read-only API responses as raw JSON"`
	TestEncode  DebugTestEncodeCmd  `cmd:"" name:"test-encode" help:"Test protocol encoding"`
	TestPackets DebugTestPacketsCmd `cmd:"" name:"test-packets" help:"Decode packets from TSV file"`
	ParseEeprom DebugParseEepromCmd `cmd:"" name:"parse-eeprom" help:"Parse SFP/QSFP EEPROM file"`
}

type DebugExploreCmd struct{}

func (c *DebugExploreCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.Explore(device)
	return nil
}

type DebugDumpAllCmd struct{}

func (c *DebugDumpAllCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.DumpAll(device)
	return nil
}

type DebugTestEncodeCmd struct{}

func (c *DebugTestEncodeCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	commands.TestEncode()
	return nil
}

type DebugTestPacketsCmd struct {
	File string `arg:"" help:"TSV file containing packet data"`
}

func (c *DebugTestPacketsCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	commands.TestPackets(c.File)
	return nil
}

type DebugParseEepromCmd struct {
	File string `arg:"" help:"EEPROM binary file to parse"`
}

func (c *DebugParseEepromCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	commands.ParseEEPROM(c.File)
	return nil
}

// --- Store Commands ---

type StoreCmd struct {
	List   StoreListCmd   `cmd:"" help:"List all stored module profiles"`
	Show   StoreShowCmd   `cmd:"" help:"Show details of a stored profile"`
	Import StoreImportCmd `cmd:"" help:"Import an EEPROM file into the store"`
	Export StoreExportCmd `cmd:"" help:"Export a profile to a file"`
}

type StoreListCmd struct{}

func (c *StoreListCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose

	s, err := store.OpenDefault()
	if err != nil {
		return fmt.Errorf("failed to open store: %w", err)
	}

	profiles, err := s.ListWithHashes()
	if err != nil {
		return fmt.Errorf("failed to list profiles: %w", err)
	}

	if len(profiles) == 0 {
		fmt.Println("No profiles in store.")
		fmt.Println("Import profiles with: sfpw store import <eeprom.bin>")
		return nil
	}

	fmt.Printf("%d profile(s)\n\n", len(profiles))
	for hash, entry := range profiles {
		shortHash := store.ShortHash(hash)
		wavelength := ""
		if entry.WavelengthNM > 0 {
			wavelength = fmt.Sprintf("%dnm", entry.WavelengthNM)
		}
		fmt.Printf("  %-12s  %-16s  %-16s  %s\n",
			shortHash,
			truncate(entry.VendorName, 16),
			truncate(entry.PartNumber, 16),
			wavelength)
	}

	return nil
}

type StoreShowCmd struct {
	Hash string `arg:"" help:"Profile hash (full or short)"`
}

func (c *StoreShowCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose

	s, err := store.OpenDefault()
	if err != nil {
		return fmt.Errorf("failed to open store: %w", err)
	}

	// Try to find matching hash
	profiles, err := s.ListWithHashes()
	if err != nil {
		return fmt.Errorf("failed to list profiles: %w", err)
	}

	var fullHash string
	for hash := range profiles {
		if hash == c.Hash || store.ShortHash(hash) == c.Hash || hash[7:] == c.Hash {
			fullHash = hash
			break
		}
	}

	if fullHash == "" {
		return fmt.Errorf("profile not found: %s", c.Hash)
	}

	entry := profiles[fullHash]
	meta, err := s.GetMetadata(fullHash)
	if err != nil {
		return fmt.Errorf("failed to get metadata: %w", err)
	}

	shortHash := store.ShortHash(fullHash)
	fmt.Printf("Hash:        %s\n", shortHash)
	fmt.Printf("Type:        %s\n", entry.ModuleType)
	fmt.Printf("Vendor:      %s\n", entry.VendorName)
	fmt.Printf("Part Number: %s\n", entry.PartNumber)
	fmt.Printf("Serial:      %s\n", entry.SerialNumber)

	if meta != nil {
		if meta.Identity.DateCode != "" {
			fmt.Printf("Date Code:   %s\n", meta.Identity.DateCode)
		}
		if meta.Specs.WavelengthNM > 0 {
			fmt.Printf("Wavelength:  %d nm\n", meta.Specs.WavelengthNM)
		}
		if meta.Specs.ConnectorType != "" {
			fmt.Printf("Connector:   %s\n", meta.Specs.ConnectorType)
		}
		fmt.Printf("Sources:     %d\n", len(meta.Sources))
	}

	fmt.Printf("\nExport: sfpw store export %s <file>\n", shortHash)

	return nil
}

type StoreImportCmd struct {
	File string `arg:"" help:"EEPROM file to import"`
}

func (c *StoreImportCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose

	data, err := os.ReadFile(c.File)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	s, err := store.OpenDefault()
	if err != nil {
		return fmt.Errorf("failed to open store: %w", err)
	}

	source := store.Source{
		Timestamp: time.Now(),
		Method:    "import",
		Filename:  c.File,
	}

	hash, isNew, err := s.Import(data, source)
	if err != nil {
		return fmt.Errorf("failed to import: %w", err)
	}

	if isNew {
		fmt.Printf("Imported new profile: %s\n", store.ShortHash(hash))
	} else {
		fmt.Printf("Profile already exists: %s (added source)\n", store.ShortHash(hash))
	}

	// Show summary
	meta, _ := s.GetMetadata(hash)
	if meta != nil {
		fmt.Printf("  Vendor: %s\n", meta.Identity.VendorName)
		fmt.Printf("  Part:   %s\n", meta.Identity.PartNumber)
		fmt.Printf("  S/N:    %s\n", meta.Identity.SerialNumber)
	}

	return nil
}

type StoreExportCmd struct {
	Hash   string `arg:"" help:"Profile hash (full or short)"`
	Output string `arg:"" help:"Output file path"`
}

func (c *StoreExportCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose

	s, err := store.OpenDefault()
	if err != nil {
		return fmt.Errorf("failed to open store: %w", err)
	}

	// Try to find matching hash
	profiles, err := s.ListWithHashes()
	if err != nil {
		return fmt.Errorf("failed to list profiles: %w", err)
	}

	var fullHash string
	for hash := range profiles {
		if hash == c.Hash || store.ShortHash(hash) == c.Hash || hash[7:] == c.Hash {
			fullHash = hash
			break
		}
	}

	if fullHash == "" {
		return fmt.Errorf("profile not found: %s", c.Hash)
	}

	if err := s.Export(fullHash, c.Output); err != nil {
		return fmt.Errorf("failed to export: %w", err)
	}

	fmt.Printf("Exported to: %s\n", c.Output)
	return nil
}

// --- Legacy Commands (hidden, for backwards compatibility) ---

type VersionCmd struct{}

func (c *VersionCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.Version(device)
	return nil
}

type APIVersionCmd struct{}

func (c *APIVersionCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.APIVersion(device)
	return nil
}

type StatsCmd struct{}

func (c *StatsCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.Stats(device)
	return nil
}

type InfoCmd struct{}

func (c *InfoCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.Info(device)
	return nil
}

type SettingsCmd struct{}

func (c *SettingsCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.Settings(device)
	return nil
}

type BtCmd struct{}

func (c *BtCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.Bluetooth(device)
	return nil
}

type LogsCmd struct{}

func (c *LogsCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.Logs(device)
	return nil
}

type RebootCmd struct{}

func (c *RebootCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.Reboot(device)
	return nil
}

// --- Additional Legacy Commands ---

type ExploreCmd struct{}

func (c *ExploreCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.Explore(device)
	return nil
}

type ModuleInfoLegacyCmd struct{}

func (c *ModuleInfoLegacyCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.ModuleInfo(device)
	return nil
}

type ModuleReadLegacyCmd struct {
	Output string `arg:"" help:"Output file path"`
}

func (c *ModuleReadLegacyCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.ModuleRead(device, c.Output)
	return nil
}

type SnapshotInfoLegacyCmd struct{}

func (c *SnapshotInfoLegacyCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.SnapshotInfo(device)
	return nil
}

type SnapshotReadLegacyCmd struct {
	Output string `arg:"" help:"Output file path"`
}

func (c *SnapshotReadLegacyCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.SnapshotRead(device, c.Output)
	return nil
}

type SnapshotWriteLegacyCmd struct {
	Input string `arg:"" help:"Input EEPROM file"`
}

func (c *SnapshotWriteLegacyCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.SnapshotWrite(device, c.Input)
	return nil
}

type FwUpdateLegacyCmd struct {
	File string `arg:"" help:"Firmware file"`
}

func (c *FwUpdateLegacyCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.FirmwareUpdate(device, c.File)
	return nil
}

type FwAbortLegacyCmd struct{}

func (c *FwAbortLegacyCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.FirmwareAbort(device)
	return nil
}

type FwStatusLegacyCmd struct{}

func (c *FwStatusLegacyCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.FirmwareStatusCmd(device)
	return nil
}

type SupportDumpLegacyCmd struct{}

func (c *SupportDumpLegacyCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.SupportDump(device)
	return nil
}

type ParseEepromLegacyCmd struct {
	File string `arg:"" help:"EEPROM file to parse"`
}

func (c *ParseEepromLegacyCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	commands.ParseEEPROM(c.File)
	return nil
}

type TestEncodeLegacyCmd struct{}

func (c *TestEncodeLegacyCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	commands.TestEncode()
	return nil
}

type TestPacketsLegacyCmd struct {
	File string `arg:"" help:"TSV file containing packets"`
}

func (c *TestPacketsLegacyCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	commands.TestPackets(c.File)
	return nil
}
