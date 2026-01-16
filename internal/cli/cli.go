package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"sfpw-tool/internal/ble"
	"sfpw-tool/internal/commands"
	"sfpw-tool/internal/config"
	"sfpw-tool/internal/store"
	"sfpw-tool/internal/tui"
)

// CLI is the root command structure for sfpw.
type CLI struct {
	Verbose bool `short:"v" help:"Enable verbose debug output"`

	// Default command - TUI
	Tui TuiCmd `cmd:"" default:"withargs" help:"Launch interactive TUI (default)"`

	Device   DeviceCmd   `cmd:"" help:"Device info and control"`
	Module   ModuleCmd   `cmd:"" help:"SFP module operations"`
	Snapshot SnapshotCmd `cmd:"" help:"Snapshot buffer operations"`
	Fw       FwCmd       `cmd:"" help:"Firmware operations"`
	Support  SupportCmd  `cmd:"" help:"Support and diagnostics"`
	Store    StoreCmd    `cmd:"" help:"Module profile store"`
	Debug    DebugCmd    `cmd:"" help:"Debug and development tools"`

	// Legacy commands for backwards compatibility (hidden)
	Version      VersionCmd      `cmd:"" hidden:""`
	APIVersion   APIVersionCmd   `cmd:"" name:"api-version" hidden:""`
	Stats        StatsCmd        `cmd:"" hidden:""`
	Info         InfoCmd         `cmd:"" hidden:""`
	Settings     SettingsCmd     `cmd:"" hidden:""`
	Bt           BtCmd           `cmd:"" hidden:""`
	Logs         LogsCmd         `cmd:"" hidden:""`
	Reboot       RebootCmd       `cmd:"" hidden:""`
	Explore      ExploreCmd      `cmd:"" hidden:""`
	ModuleInfo   ModuleInfoLegacyCmd   `cmd:"" name:"module-info" hidden:""`
	ModuleRead   ModuleReadLegacyCmd   `cmd:"" name:"module-read" hidden:""`
	SnapshotInfo SnapshotInfoLegacyCmd `cmd:"" name:"snapshot-info" hidden:""`
	SnapshotRead SnapshotReadLegacyCmd `cmd:"" name:"snapshot-read" hidden:""`
	SnapshotWrite SnapshotWriteLegacyCmd `cmd:"" name:"snapshot-write" hidden:""`
	FwUpdate     FwUpdateLegacyCmd     `cmd:"" name:"fw-update" hidden:""`
	FwAbort      FwAbortLegacyCmd      `cmd:"" name:"fw-abort" hidden:""`
	FwStatusLegacy FwStatusLegacyCmd   `cmd:"" name:"fw-status" hidden:""`
	SupportDump  SupportDumpLegacyCmd  `cmd:"" name:"support-dump" hidden:""`
	ParseEeprom  ParseEepromLegacyCmd  `cmd:"" name:"parse-eeprom" hidden:""`
	TestEncode   TestEncodeLegacyCmd   `cmd:"" name:"test-encode" hidden:""`
	TestPackets  TestPacketsLegacyCmd  `cmd:"" name:"test-packets" hidden:""`
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
	Info  ModuleInfoCmd  `cmd:"" help:"Get details about the inserted SFP module"`
	Read  ModuleReadCmd  `cmd:"" help:"Read EEPROM from physical module to file"`
	Write ModuleWriteCmd `cmd:"" help:"Write EEPROM file to snapshot buffer (alias for snapshot write)"`
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

type ModuleWriteCmd struct {
	Input string `arg:"" help:"Input EEPROM file to write to snapshot"`
}

func (c *ModuleWriteCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.SnapshotWrite(device, c.Input)
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
	Input string `arg:"" help:"Input EEPROM file to write to snapshot"`
}

func (c *SnapshotWriteCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.SnapshotWrite(device, c.Input)
	return nil
}

// --- Firmware Commands ---

type FwCmd struct {
	Status FwStatusCmd `cmd:"" help:"Get detailed firmware status"`
	Update FwUpdateCmd `cmd:"" help:"Upload and install firmware from file"`
	Abort  FwAbortCmd  `cmd:"" help:"Abort an in-progress firmware update"`
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
	File string `arg:"" help:"Firmware file to upload"`
}

func (c *FwUpdateCmd) Run(globals *CLI) error {
	config.Verbose = globals.Verbose
	device := ble.Connect()
	defer device.Disconnect()
	commands.FirmwareUpdate(device, c.File)
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

	fmt.Printf("Found %d profile(s):\n\n", len(profiles))
	for hash, entry := range profiles {
		shortHash := store.ShortHash(hash)
		fmt.Printf("  %s  %-16s  %-20s  %s\n",
			shortHash,
			entry.VendorName,
			entry.PartNumber,
			entry.SerialNumber)
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

	meta, err := s.GetMetadata(fullHash)
	if err != nil {
		return fmt.Errorf("failed to get metadata: %w", err)
	}

	// Pretty print metadata
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))

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
