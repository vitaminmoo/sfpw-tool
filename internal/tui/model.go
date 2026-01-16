package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"tinygo.org/x/bluetooth"

	"sfpw-tool/internal/api"
	"sfpw-tool/internal/firmware"
	"sfpw-tool/internal/store"
)

// View represents different screens in the TUI.
type View int

const (
	ViewMain View = iota
	ViewDevice
	ViewModule
	ViewStore
	ViewStoreDetail
	ViewFirmware
	ViewFirmwareSelect // Select a firmware version to install
	ViewHelp
)

// MenuItem represents a menu option.
type MenuItem struct {
	Title       string
	Description string
	View        View
	Action      func() tea.Cmd
}

// Model is the main Bubbletea model for the TUI.
type Model struct {
	// State
	view          View
	prevView      View
	cursor        int
	cursorHistory map[View]int // Remember cursor position per view
	menuItems     []MenuItem
	width         int
	height        int

	// Data
	connected     bool
	searching     bool
	connecting    bool
	deviceMAC     string
	storeProfiles map[string]store.IndexEntry
	selectedHash  string
	errorMsg      string
	statusMsg     string

	// Module data
	moduleData        []byte
	moduleLoading     bool
	moduleError       string
	moduleDetails     *api.ModuleDetails // Info about inserted module
	snapshotInfo      *api.SnapshotInfo  // Info about snapshot buffer
	moduleInfoLoading bool               // Loading module/snapshot info (initial load only)
	moduleInfoRefresh bool               // True during periodic refresh (no spinner)

	// Device data
	client               *api.Client
	stats                *api.Stats
	deviceInfo           *api.DeviceInfo
	settings             *api.Settings
	bluetooth            *api.BluetoothParams
	firmware             *api.FirmwareStatus
	loading              bool // True when fetching data
	connectionCheckFails int  // Consecutive connection check failures

	// Firmware update state
	availableFirmware   []firmware.FirmwareVersion
	availableFwLoading  bool
	availableFwError    string
	lastFirmwareRefresh time.Time // When we last refreshed the firmware list
	cachedFirmware      []firmware.FirmwareEntry

	// Firmware sync progress (downloading all versions)
	fwSyncing          bool
	fwSyncPhase        string  // "fetching", "downloading X of Y", "complete"
	fwSyncProgress     float64 // 0.0 to 1.0
	fwSyncCurrentVer   string  // Version currently being downloaded

	// Selected firmware for flashing
	selectedFwVersion string // e.g. "v1.1.3"
	selectedFwPath    string // path to cached .bin file
	selectedFwSize    int64
	selectedFwSHA256  string

	// Firmware flash progress
	fwFlashing      bool
	fwFlashPhase    string // "uploading", "installing", "complete", "error"
	fwFlashProgress float64
	fwFlashError    string

	// File picker state
	filepicker       filepicker.Model
	filePickerActive bool
	selectedFilePath string

	// Components
	keys    KeyMap
	help    help.Model
	spinner spinner.Model
	styles  Styles
}

// --- Custom messages for async operations ---

// scanResultMsg signals a device was found during scanning.
type scanResultMsg struct {
	device *bluetooth.Device
	err    error
}

// connectMsg signals connection attempt result.
type connectMsg struct {
	client *api.Client
	mac    string
	err    error
}

// statsMsg delivers device stats from async fetch.
type statsMsg struct {
	stats *api.Stats
	err   error
}

// deviceInfoMsg delivers device info from async fetch.
type deviceInfoMsg struct {
	info *api.DeviceInfo
	err  error
}

// settingsMsg delivers device settings from async fetch.
type settingsMsg struct {
	settings *api.Settings
	err      error
}

// bluetoothMsg delivers bluetooth params from async fetch.
type bluetoothMsg struct {
	bluetooth *api.BluetoothParams
	err       error
}

// firmwareMsg delivers firmware status from async fetch.
type firmwareMsg struct {
	firmware *api.FirmwareStatus
	err      error
}

// statusTickMsg triggers a status refresh.
type statusTickMsg time.Time

// moduleReadMsg delivers module EEPROM data from async read.
type moduleReadMsg struct {
	data []byte
	hash string // Store hash after save
	err  error
}

// snapshotReadMsg delivers snapshot EEPROM data from async read.
type snapshotReadMsg struct {
	data []byte
	hash string
	err  error
}

// moduleDetailsMsg delivers module details from async fetch.
type moduleDetailsMsg struct {
	details *api.ModuleDetails
	err     error
}

// snapshotInfoMsg delivers snapshot info from async fetch.
type snapshotInfoMsg struct {
	info *api.SnapshotInfo
	err  error
}

// moduleInfoTickMsg triggers periodic module/snapshot info refresh.
type moduleInfoTickMsg time.Time

// connectionCheckMsg triggers a periodic connection health check.
type connectionCheckMsg time.Time

// availableFirmwareMsg delivers available firmware versions from cloud.
type availableFirmwareMsg struct {
	versions []firmware.FirmwareVersion
	err      error
}

// firmwareSyncProgressMsg reports progress during firmware sync.
type firmwareSyncProgressMsg struct {
	phase      string  // "fetching", "downloading"
	progress   float64 // 0.0 to 1.0
	currentVer string  // Version currently being downloaded
	current    int     // Current item number
	total      int     // Total items
}

// firmwareSyncCompleteMsg signals firmware sync completed.
type firmwareSyncCompleteMsg struct {
	versions []firmware.FirmwareVersion
	cached   []firmware.FirmwareEntry
	err      error
}

// cachedFirmwareMsg delivers cached firmware list.
type cachedFirmwareMsg struct {
	cached []firmware.FirmwareEntry
}

// firmwareImportedMsg signals a file was imported to cache.
type firmwareImportedMsg struct {
	version string
	path    string
	size    int64
	sha256  string
	err     error
}

// firmwareDownloadedMsg signals a cloud firmware was downloaded.
type firmwareDownloadedMsg struct {
	version string
	path    string
	size    int64
	sha256  string
	err     error
}

// firmwareFlashProgressMsg reports firmware flash progress.
type firmwareFlashProgressMsg struct {
	phase    string  // "uploading", "installing"
	progress float64 // 0.0 to 1.0
}

// firmwareFlashCompleteMsg signals firmware flash completed.
type firmwareFlashCompleteMsg struct {
	success bool
	message string
	err     error
}

// NewModel creates a new TUI model.
func NewModel() Model {
	h := help.New()
	h.ShowAll = false // Use ShortHelp for horizontal layout

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4"))

	m := Model{
		view:          ViewMain,
		searching:     true, // Start searching on launch
		cursorHistory: make(map[View]int),
		keys:          DefaultKeyMap(),
		help:          h,
		spinner:       s,
		styles:        DefaultStyles(),
	}

	m.menuItems = []MenuItem{
		{
			Title:       "Device",
			Description: "View device info, stats, and settings",
			View:        ViewDevice,
		},
		{
			Title:       "Module",
			Description: "Read/write SFP module EEPROM",
			View:        ViewModule,
		},
		{
			Title:       "Store",
			Description: "Browse saved module profiles",
			View:        ViewStore,
		},
		{
			Title:       "Firmware",
			Description: "Update device firmware",
			View:        ViewFirmware,
		},
	}

	// Initialize file picker for firmware selection
	fp := filepicker.New()
	fp.AllowedTypes = []string{".bin"}
	fp.DirAllowed = true    // Allow navigating into directories
	fp.FileAllowed = true   // Allow selecting files
	fp.ShowHidden = false
	fp.ShowSize = true
	fp.ShowPermissions = false
	fp.SetHeight(15)        // Show 15 files at a time
	// Start in current working directory
	if cwd, err := os.Getwd(); err == nil {
		fp.CurrentDirectory = cwd
	} else {
		fp.CurrentDirectory = "."
	}
	m.filepicker = fp

	// Load store profiles
	m.loadStoreProfiles()

	return m
}

func (m *Model) loadStoreProfiles() {
	s, err := store.OpenDefault()
	if err != nil {
		return
	}
	profiles, err := s.ListWithHashes()
	if err != nil {
		return
	}
	m.storeProfiles = profiles
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	// Auto-start scanning for device and spinner
	return tea.Batch(scanForDeviceCmd, m.spinner.Tick)
}

// isTransientError checks if an error is a transient BLE error that shouldn't be displayed.
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "In progress") ||
		strings.Contains(msg, "in progress") ||
		strings.Contains(msg, "busy")
}

// Update handles messages and updates the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle file picker if active
	if m.filePickerActive {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			// Check for escape to cancel
			if key.Matches(msg, m.keys.Back) || key.Matches(msg, m.keys.Quit) {
				m.filePickerActive = false
				return m, nil
			}
		}

		var cmd tea.Cmd
		m.filepicker, cmd = m.filepicker.Update(msg)

		// Check if a file was selected
		if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
			m.filePickerActive = false
			m.selectedFilePath = path
			// Import to cache
			return m, importFirmwareFileCmd(path)
		}

		// Check if file picker was cancelled
		if didSelect, _ := m.filepicker.DidSelectDisabledFile(msg); didSelect {
			m.filePickerActive = false
			m.availableFwError = "Invalid file type selected (must be .bin)"
			return m, nil
		}

		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case scanResultMsg:
		m.searching = false
		if msg.err != nil {
			m.errorMsg = fmt.Sprintf("Scan failed: %v", msg.err)
			return m, nil
		}
		if msg.device == nil {
			m.errorMsg = "Device not found"
			return m, nil
		}
		// Device found, now connect
		m.connecting = true
		m.statusMsg = "Found device, connecting..."
		return m, connectToDeviceCmd(msg.device)

	case connectMsg:
		m.connecting = false
		if msg.err != nil {
			m.errorMsg = fmt.Sprintf("Connection failed: %v", msg.err)
			return m, nil
		}
		m.connected = true
		m.client = msg.client
		m.deviceMAC = msg.mac
		m.statusMsg = "Connected"
		m.errorMsg = ""
		m.loading = true
		m.connectionCheckFails = 0
		// Fetch device info first, stats will be fetched after
		// Also start connection health check
		return m, tea.Batch(
			fetchDeviceInfoCmd(m.client),
			connectionCheckCmd(),
			m.spinner.Tick,
		)

	case statsMsg:
		m.loading = false
		if msg.err != nil {
			// Ignore transient errors
			if !isTransientError(msg.err) {
				m.errorMsg = fmt.Sprintf("Stats error: %v", msg.err)
			}
			return m, nil
		}
		m.stats = msg.stats
		// Clear error only on success
		if m.errorMsg != "" && strings.HasPrefix(m.errorMsg, "Stats error") {
			m.errorMsg = ""
		}
		return m, nil

	case deviceInfoMsg:
		if msg.err != nil {
			m.loading = false
			// Ignore transient errors
			if !isTransientError(msg.err) {
				m.errorMsg = fmt.Sprintf("Info error: %v", msg.err)
			}
			return m, nil
		}
		m.deviceInfo = msg.info
		// Clear error only on success
		if m.errorMsg != "" && strings.HasPrefix(m.errorMsg, "Info error") {
			m.errorMsg = ""
		}
		// After getting device info, fetch stats and start periodic updates
		if m.client != nil && m.stats == nil {
			return m, tea.Batch(
				fetchStatsCmd(m.client),
				statusTickCmd(),
			)
		}
		m.loading = false
		return m, nil

	case settingsMsg:
		if msg.err != nil {
			// Ignore transient errors silently, continue chain
			if !isTransientError(msg.err) {
				m.errorMsg = fmt.Sprintf("Settings error: %v", msg.err)
			}
		} else {
			m.settings = msg.settings
		}
		// Chain to bluetooth fetch
		if m.client != nil {
			return m, fetchBluetoothCmd(m.client)
		}
		return m, nil

	case bluetoothMsg:
		if msg.err != nil {
			// Ignore transient errors silently, continue chain
			if !isTransientError(msg.err) {
				m.errorMsg = fmt.Sprintf("Bluetooth error: %v", msg.err)
			}
		} else {
			m.bluetooth = msg.bluetooth
		}
		// Chain to firmware fetch
		if m.client != nil {
			return m, fetchFirmwareCmd(m.client)
		}
		return m, nil

	case firmwareMsg:
		m.loading = false
		if msg.err != nil {
			// Ignore transient errors silently
			if !isTransientError(msg.err) {
				m.errorMsg = fmt.Sprintf("Firmware error: %v", msg.err)
			}
			return m, nil
		}
		m.firmware = msg.firmware
		return m, nil

	case statusTickMsg:
		// Refresh stats periodically when connected (but not if already loading)
		if m.connected && m.client != nil && !m.loading {
			return m, tea.Batch(
				fetchStatsCmd(m.client),
				statusTickCmd(),
			)
		}
		// Reschedule even if we skipped this tick
		if m.connected {
			return m, statusTickCmd()
		}
		return m, nil

	case moduleReadMsg:
		m.moduleLoading = false
		if msg.err != nil {
			m.moduleError = msg.err.Error()
			return m, nil
		}
		m.moduleData = msg.data
		m.moduleError = ""
		// Refresh store profiles to show newly added profile
		m.loadStoreProfiles()
		m.statusMsg = fmt.Sprintf("Module saved to store: %s", store.ShortHash(msg.hash))
		return m, nil

	case snapshotReadMsg:
		m.moduleLoading = false
		if msg.err != nil {
			m.moduleError = msg.err.Error()
			return m, nil
		}
		m.moduleData = msg.data
		m.moduleError = ""
		// Refresh store profiles to show newly added profile
		m.loadStoreProfiles()
		m.statusMsg = fmt.Sprintf("Snapshot saved to store: %s", store.ShortHash(msg.hash))
		return m, nil

	case moduleDetailsMsg:
		// Always update moduleDetails - use empty struct on error
		if msg.err == nil && msg.details != nil {
			m.moduleDetails = msg.details
		} else {
			m.moduleDetails = &api.ModuleDetails{} // Empty struct = no module
		}
		// Chain to snapshot info fetch
		if m.client != nil {
			return m, fetchSnapshotInfoCmd(m.client)
		}
		m.moduleInfoLoading = false
		m.moduleInfoRefresh = false
		return m, nil

	case snapshotInfoMsg:
		m.moduleInfoLoading = false
		m.moduleInfoRefresh = false
		// Always update snapshotInfo - use empty struct on error
		if msg.err == nil && msg.info != nil {
			m.snapshotInfo = msg.info
		} else {
			m.snapshotInfo = &api.SnapshotInfo{} // Empty struct = no data
		}
		// Schedule next refresh if we're still on Module view
		if m.view == ViewModule && m.connected {
			return m, moduleInfoTickCmd()
		}
		return m, nil

	case moduleInfoTickMsg:
		// Periodic refresh of module/snapshot info when on Module view
		// Only refresh if not already loading and not doing a user-initiated read
		if m.view == ViewModule && m.connected && m.client != nil && !m.moduleInfoLoading && !m.moduleInfoRefresh && !m.moduleLoading {
			m.moduleInfoRefresh = true // Use refresh flag, not loading (no spinner)
			return m, fetchModuleDetailsCmd(m.client)
		}
		// Reschedule even if we skipped this tick
		if m.view == ViewModule && m.connected {
			return m, moduleInfoTickCmd()
		}
		return m, nil

	case availableFirmwareMsg:
		m.availableFwLoading = false
		if msg.err != nil {
			m.availableFwError = msg.err.Error()
			return m, nil
		}
		m.availableFirmware = msg.versions
		m.availableFwError = ""
		return m, nil

	case firmwareSyncProgressMsg:
		m.fwSyncPhase = msg.phase
		m.fwSyncProgress = msg.progress
		m.fwSyncCurrentVer = msg.currentVer
		return m, nil

	case firmwareSyncCompleteMsg:
		m.fwSyncing = false
		m.fwSyncPhase = ""
		m.lastFirmwareRefresh = time.Now()
		if msg.err != nil {
			m.availableFwError = msg.err.Error()
			return m, nil
		}
		m.availableFirmware = msg.versions
		m.cachedFirmware = msg.cached
		m.availableFwError = ""
		return m, nil

	case cachedFirmwareMsg:
		m.cachedFirmware = msg.cached
		return m, nil

	case connectionCheckMsg:
		// Periodic connection health check
		if m.connected && m.client != nil {
			if !m.client.IsConnected() {
				m.connectionCheckFails++
				if m.connectionCheckFails >= 2 {
					// Declare disconnected after 2 consecutive failures
					return m.handleDisconnect()
				}
			} else {
				m.connectionCheckFails = 0
			}
			return m, connectionCheckCmd()
		}
		return m, nil

	case firmwareImportedMsg:
		if msg.err != nil {
			m.availableFwError = fmt.Sprintf("Failed to import file: %v", msg.err)
			return m, nil
		}
		m.selectedFwVersion = msg.version
		m.selectedFwPath = msg.path
		m.selectedFwSize = msg.size
		m.selectedFwSHA256 = msg.sha256
		m.availableFwError = ""
		m.statusMsg = fmt.Sprintf("Imported %s to cache", msg.version)
		return m, nil

	case firmwareDownloadedMsg:
		if msg.err != nil {
			m.availableFwError = fmt.Sprintf("Failed to download: %v", msg.err)
			return m, nil
		}
		m.selectedFwVersion = msg.version
		m.selectedFwPath = msg.path
		m.selectedFwSize = msg.size
		m.selectedFwSHA256 = msg.sha256
		m.availableFwError = ""
		m.statusMsg = fmt.Sprintf("Downloaded %s", msg.version)
		return m, nil

	case firmwareFlashProgressMsg:
		m.fwFlashPhase = msg.phase
		m.fwFlashProgress = msg.progress
		return m, nil

	case firmwareFlashCompleteMsg:
		m.fwFlashing = false
		if msg.err != nil {
			m.fwFlashPhase = "error"
			m.fwFlashError = msg.err.Error()
			return m, nil
		}
		m.fwFlashPhase = "complete"
		m.statusMsg = msg.message
		// Clear selection after successful flash
		m.selectedFwVersion = ""
		m.selectedFwPath = ""
		return m, nil
	}
	return m, nil
}

// handleDisconnect handles device disconnection.
func (m Model) handleDisconnect() (tea.Model, tea.Cmd) {
	m.connected = false
	m.connecting = false
	m.searching = false
	m.client = nil
	m.stats = nil
	m.deviceInfo = nil
	m.settings = nil
	m.bluetooth = nil
	m.firmware = nil
	m.moduleDetails = nil
	m.snapshotInfo = nil
	m.connectionCheckFails = 0
	m.loading = false
	m.moduleLoading = false
	m.moduleInfoLoading = false
	// Stop any in-progress firmware flash
	if m.fwFlashing {
		m.fwFlashing = false
		m.fwFlashError = "Device disconnected during flash"
	}
	m.errorMsg = "Device disconnected"
	m.statusMsg = "Press 'c' to reconnect"
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		if m.view == ViewMain {
			return m, tea.Quit
		}
		// Go back to main
		m.view = ViewMain
		m.cursor = 0
		return m, nil

	case key.Matches(msg, m.keys.Back), key.Matches(msg, m.keys.Left):
		return m.goBack()

	case key.Matches(msg, m.keys.Up):
		m.cursor--
		if m.cursor < 0 {
			m.cursor = m.maxCursor()
		}
		return m, nil

	case key.Matches(msg, m.keys.Down):
		m.cursor++
		if m.cursor > m.maxCursor() {
			m.cursor = 0
		}
		return m, nil

	case key.Matches(msg, m.keys.Select), key.Matches(msg, m.keys.Right):
		return m.handleSelect()

	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		return m, nil

	case key.Matches(msg, m.keys.Refresh):
		m.loadStoreProfiles()
		m.statusMsg = "Refreshed"
		if m.connected && m.client != nil {
			return m, fetchStatsCmd(m.client)
		}
		return m, nil

	case key.Matches(msg, m.keys.Connect):
		if !m.connected && !m.connecting && !m.searching {
			m.searching = true
			m.statusMsg = "Searching..."
			m.errorMsg = ""
			return m, scanForDeviceCmd
		}
		return m, nil
	}

	return m, nil
}

func (m Model) goBack() (tea.Model, tea.Cmd) {
	// Save current cursor position before leaving
	m.cursorHistory[m.view] = m.cursor

	switch m.view {
	case ViewMain:
		return m, tea.Quit
	case ViewStoreDetail:
		m.view = ViewStore
		m.selectedHash = ""
	case ViewFirmwareSelect:
		m.view = ViewFirmware
	default:
		m.view = ViewMain
	}

	// Restore cursor position for the target view
	m.cursor = m.cursorHistory[m.view]
	return m, nil
}

func (m Model) handleSelect() (tea.Model, tea.Cmd) {
	switch m.view {
	case ViewMain:
		if m.cursor < len(m.menuItems) {
			// Save cursor position before leaving
			m.cursorHistory[m.view] = m.cursor

			targetView := m.menuItems[m.cursor].View
			m.view = targetView
			m.cursor = m.cursorHistory[targetView] // Restore saved position or 0

			// Fetch all device data when entering Device view
			if targetView == ViewDevice && m.connected && m.client != nil && !m.loading {
				m.loading = true
				// Start the chain: settings -> bluetooth -> firmware
				// Stats continue to update via periodic tick
				return m, tea.Batch(
					fetchSettingsCmd(m.client),
					m.spinner.Tick,
				)
			}

			// Fetch module/snapshot info when entering Module view
			if targetView == ViewModule && m.connected && m.client != nil && !m.moduleInfoLoading {
				m.moduleInfoLoading = true
				m.moduleDetails = nil
				m.snapshotInfo = nil
				return m, tea.Batch(
					fetchModuleDetailsCmd(m.client),
					m.spinner.Tick,
				)
			}

			// Firmware view: check if we need to sync firmware cache
			if targetView == ViewFirmware {
				var cmds []tea.Cmd
				cmds = append(cmds, m.spinner.Tick)

				// Fetch device firmware status if connected
				if m.connected && m.client != nil && m.firmware == nil {
					cmds = append(cmds, fetchFirmwareCmd(m.client))
				}

				// Check if we need to sync (hasn't been done in 10 minutes)
				needsSync := time.Since(m.lastFirmwareRefresh) > 10*time.Minute
				if needsSync && !m.fwSyncing && !m.availableFwLoading {
					m.fwSyncing = true
					m.fwSyncPhase = "fetching"
					m.fwSyncProgress = 0
					m.availableFwError = ""
					cmds = append(cmds, syncFirmwareCacheCmd())
				} else if !needsSync && len(m.cachedFirmware) == 0 {
					// Just refresh cached list if we have recent data but no cache loaded
					cmds = append(cmds, refreshCachedFirmwareCmd())
				}

				return m, tea.Batch(cmds...)
			}
		}
	case ViewStore:
		// Save cursor position before leaving
		m.cursorHistory[m.view] = m.cursor

		// Select a profile to view details
		hashes := m.getSortedHashes()
		if m.cursor < len(hashes) {
			m.selectedHash = hashes[m.cursor]
			m.view = ViewStoreDetail
			m.cursor = m.cursorHistory[ViewStoreDetail]
		}

	case ViewModule:
		// Handle module menu selection
		if !m.connected || m.client == nil || m.moduleLoading {
			return m, nil
		}
		m.moduleLoading = true
		m.moduleError = ""
		m.statusMsg = ""
		switch m.cursor {
		case 0: // Read Module
			return m, tea.Batch(
				readModuleCmd(m.client, m.deviceMAC),
				m.spinner.Tick,
			)
		case 1: // Read Snapshot
			return m, tea.Batch(
				readSnapshotCmd(m.client, m.deviceMAC),
				m.spinner.Tick,
			)
		}

	case ViewFirmware:
		// Handle firmware menu selection based on dynamic menu
		menuItems := m.getFirmwareMenuItems()
		if m.cursor >= len(menuItems) || m.fwSyncing {
			return m, nil
		}

		selectedItem := menuItems[m.cursor].title
		switch selectedItem {
		case "Select from Cache":
			if len(m.cachedFirmware) == 0 {
				return m, nil
			}
			// Go to version selection view
			m.cursorHistory[m.view] = m.cursor
			m.view = ViewFirmwareSelect
			m.cursor = 0
			return m, nil
		case "Select from File":
			// Activate file picker
			m.filePickerActive = true
			// Reset to current working directory
			if cwd, err := os.Getwd(); err == nil {
				m.filepicker.CurrentDirectory = cwd
			}
			return m, m.filepicker.Init()
		case "Refresh Cache":
			if !m.fwSyncing {
				m.fwSyncing = true
				m.fwSyncPhase = "fetching"
				m.fwSyncProgress = 0
				m.availableFwError = ""
				return m, tea.Batch(
					syncFirmwareCacheCmd(),
					m.spinner.Tick,
				)
			}
		case "Flash Firmware":
			if m.selectedFwPath == "" || m.fwFlashing {
				return m, nil
			}
			if !m.connected || m.client == nil {
				m.availableFwError = "Not connected to device"
				return m, nil
			}
			m.fwFlashing = true
			m.fwFlashPhase = "uploading"
			m.fwFlashError = ""
			m.statusMsg = ""
			return m, tea.Batch(
				flashFirmwareCmd(m.client, m.selectedFwPath),
				m.spinner.Tick,
			)
		case "Clear Selection":
			m.selectedFwVersion = ""
			m.selectedFwPath = ""
			m.selectedFwSize = 0
			m.selectedFwSHA256 = ""
			m.fwFlashError = ""
			m.statusMsg = ""
			// Reset cursor if it's beyond the new menu length
			newMax := len(m.getFirmwareMenuItems()) - 1
			if newMax < 0 {
				newMax = 0
			}
			if m.cursor > newMax {
				m.cursor = newMax
			}
			return m, nil
		}

	case ViewFirmwareSelect:
		// Select a cached firmware version
		if m.cursor < len(m.cachedFirmware) {
			selected := m.cachedFirmware[m.cursor]
			m.selectedFwVersion = selected.Version
			m.selectedFwPath = selected.Path
			m.selectedFwSize = selected.FileSize
			m.selectedFwSHA256 = "" // We don't have this from cache entry
			m.view = ViewFirmware
			m.statusMsg = fmt.Sprintf("Selected %s", selected.Version)
			return m, nil
		}
	}
	return m, nil
}

func (m Model) maxCursor() int {
	switch m.view {
	case ViewMain:
		return len(m.menuItems) - 1
	case ViewStore:
		return len(m.storeProfiles) - 1
	case ViewModule:
		return 1 // 2 menu items: Read Module, Read Snapshot
	case ViewFirmware:
		return len(m.getFirmwareMenuItems()) - 1
	case ViewFirmwareSelect:
		if len(m.cachedFirmware) == 0 {
			return 0
		}
		return len(m.cachedFirmware) - 1
	default:
		return 0
	}
}

func (m Model) getSortedHashes() []string {
	hashes := make([]string, 0, len(m.storeProfiles))
	for h := range m.storeProfiles {
		hashes = append(hashes, h)
	}
	// Sort by creation time (newest first), then by hash for stability
	sort.Slice(hashes, func(i, j int) bool {
		ei := m.storeProfiles[hashes[i]]
		ej := m.storeProfiles[hashes[j]]
		if ei.CreatedAt.Equal(ej.CreatedAt) {
			return hashes[i] < hashes[j]
		}
		return ei.CreatedAt.After(ej.CreatedAt)
	})
	return hashes
}

// View renders the model.
func (m Model) View() string {
	// File picker overlay
	if m.filePickerActive {
		content := m.styles.Title.Render("Select Firmware File") + "\n" +
			m.styles.Muted.Render("Directory: "+m.filepicker.CurrentDirectory) + "\n\n" +
			m.filepicker.View() + "\n\n" +
			m.styles.Muted.Render("↑/↓ navigate • Enter/→ select • ←/h parent dir • ESC cancel")
		return m.styles.App.Render(content)
	}

	var content string

	switch m.view {
	case ViewMain:
		content = m.viewMain()
	case ViewDevice:
		content = m.viewDevice()
	case ViewModule:
		content = m.viewModule()
	case ViewStore:
		content = m.viewStore()
	case ViewStoreDetail:
		content = m.viewStoreDetail()
	case ViewFirmware:
		content = m.viewFirmware()
	case ViewFirmwareSelect:
		content = m.viewFirmwareSelect()
	default:
		content = "Unknown view"
	}

	// Help
	helpView := m.styles.Help.Render(m.help.View(m.keys))

	return m.styles.App.Render(
		content + "\n" + helpView,
	)
}

func (m Model) viewMain() string {
	var b strings.Builder

	// Title bar with connection status
	b.WriteString(m.renderTitleBar("SFP Wizard"))
	b.WriteString("\n")

	// Error message if any (only when not connected)
	if !m.connected && m.errorMsg != "" {
		b.WriteString(m.styles.Error.Render(m.errorMsg))
		b.WriteString("  ")
		connectKey := m.keys.Connect.Help().Key
		b.WriteString(m.styles.Muted.Render(fmt.Sprintf("['%s' to retry]", connectKey)))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Menu with dynamic descriptions
	for i, item := range m.menuItems {
		title := item.Title
		desc := item.Description

		// Add profile count to Store description
		if item.View == ViewStore {
			desc = fmt.Sprintf("%s (%d profiles)", item.Description, len(m.storeProfiles))
		}

		if i == m.cursor {
			b.WriteString(m.styles.MenuItemSelected.Render("> " + title))
		} else {
			b.WriteString(m.styles.MenuItem.Render("  " + title))
		}
		b.WriteString("\n")
		b.WriteString(m.styles.MenuItemDim.Render(desc))
		b.WriteString("\n\n")
	}

	return b.String()
}

// renderTitleBar renders a consistent title bar with connection status.
func (m Model) renderTitleBar(title string) string {
	var parts []string

	// Title
	parts = append(parts, m.styles.Title.Render(title))

	// Connection status
	if m.searching {
		parts = append(parts, m.spinner.View()+" "+m.styles.Warning.Render("Searching..."))
	} else if m.connecting {
		parts = append(parts, m.spinner.View()+" "+m.styles.Warning.Render("Connecting..."))
	} else if m.connected {
		parts = append(parts, m.styles.Success.Render("●"))
		parts = append(parts, m.styles.Muted.Render(formatMAC(m.deviceMAC)))
		if m.deviceInfo != nil {
			parts = append(parts, m.styles.Muted.Render("FW "+m.deviceInfo.FWVersion))
		}
		if m.stats != nil {
			batteryIcon := "🔋"
			if m.stats.IsLowBattery {
				batteryIcon = "🪫"
			}
			parts = append(parts, fmt.Sprintf("%s %d%%", batteryIcon, m.stats.Battery))
		}
	} else {
		parts = append(parts, m.styles.StatusOffline.Render("○ Offline"))
	}

	return strings.Join(parts, "  ")
}

// formatMAC formats a MAC address with colons (aa:bb:cc:dd:ee:ff).
func formatMAC(mac string) string {
	// Remove any existing separators and lowercase
	clean := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(mac, ":", ""), "-", ""))
	if len(clean) != 12 {
		return mac // Return as-is if not valid
	}
	// Insert colons
	return fmt.Sprintf("%s:%s:%s:%s:%s:%s",
		clean[0:2], clean[2:4], clean[4:6],
		clean[6:8], clean[8:10], clean[10:12])
}

func (m Model) viewDevice() string {
	var b strings.Builder

	// Title bar
	b.WriteString(m.renderTitleBar("Device Info"))
	b.WriteString("\n\n")

	if !m.connected {
		if m.searching {
			b.WriteString(m.spinner.View())
			b.WriteString(" ")
			b.WriteString(m.styles.Warning.Render("Searching for device..."))
		} else if m.connecting {
			b.WriteString(m.spinner.View())
			b.WriteString(" ")
			b.WriteString(m.styles.Warning.Render("Connecting..."))
		} else {
			connectKey := m.keys.Connect.Help().Key
			b.WriteString(m.styles.Muted.Render(fmt.Sprintf("Press '%s' to connect", connectKey)))
		}
		return b.String()
	}

	// Device Info section (always shown if connected since we have deviceInfo from connect)
	b.WriteString(m.styles.Highlight.Render("Device"))
	b.WriteString("\n")
	if m.deviceInfo != nil {
		if m.deviceInfo.Name != "" {
			b.WriteString(m.renderField("Name", m.deviceInfo.Name))
		}
		if m.deviceInfo.Type != "" {
			b.WriteString(m.renderField("Type", m.deviceInfo.Type))
		}
		b.WriteString(m.renderField("Firmware", m.deviceInfo.FWVersion))
		if m.deviceInfo.APIVersion != "" {
			b.WriteString(m.renderField("API Version", m.deviceInfo.APIVersion))
		}
		if m.deviceInfo.State != "" {
			b.WriteString(m.renderField("State", m.deviceInfo.State))
		}
	}

	// Stats section
	b.WriteString("\n")
	b.WriteString(m.styles.Highlight.Render("Stats"))
	b.WriteString("\n")
	if m.stats != nil {
		batteryIcon := "🔋"
		if m.stats.IsLowBattery {
			batteryIcon = "🪫"
		}
		batteryStr := fmt.Sprintf("%s %d%% (%.2fV)", batteryIcon, m.stats.Battery, m.stats.BatteryV)
		b.WriteString(m.renderField("Battery", batteryStr))
		b.WriteString(m.renderField("Signal", fmt.Sprintf("%d dBm", m.stats.SignalDbm)))
		b.WriteString(m.renderField("Uptime", formatUptime(m.stats.Uptime)))
	} else {
		b.WriteString(m.renderField("Battery", m.spinner.View()))
		b.WriteString(m.renderField("Signal", ""))
		b.WriteString(m.renderField("Uptime", ""))
	}

	// Settings section
	b.WriteString("\n")
	b.WriteString(m.styles.Highlight.Render("Settings"))
	b.WriteString("\n")
	if m.settings != nil {
		b.WriteString(m.renderField("Device Name", m.settings.Name))
		b.WriteString(m.renderField("Channel", m.settings.Channel))
		b.WriteString(m.renderField("Region", m.settings.UWSType))
		ledStatus := "Off"
		if m.settings.IsLedEnabled {
			ledStatus = "On"
		}
		b.WriteString(m.renderField("LED", ledStatus))
		hwReset := "Allowed"
		if m.settings.IsHwResetBlocked {
			hwReset = "Blocked"
		}
		b.WriteString(m.renderField("HW Reset", hwReset))
	} else {
		b.WriteString(m.renderField("Device Name", m.spinner.View()))
		b.WriteString(m.renderField("Channel", ""))
		b.WriteString(m.renderField("Region", ""))
		b.WriteString(m.renderField("LED", ""))
		b.WriteString(m.renderField("HW Reset", ""))
	}

	// Bluetooth section
	b.WriteString("\n")
	b.WriteString(m.styles.Highlight.Render("Bluetooth"))
	b.WriteString("\n")
	if m.bluetooth != nil {
		b.WriteString(m.renderField("Mode", m.bluetooth.Mode))
	} else {
		b.WriteString(m.renderField("Mode", m.spinner.View()))
	}

	// Firmware section
	b.WriteString("\n")
	b.WriteString(m.styles.Highlight.Render("Firmware"))
	b.WriteString("\n")
	if m.firmware != nil {
		b.WriteString(m.renderField("Version", m.firmware.FWVersion))
		b.WriteString(m.renderField("Hardware", fmt.Sprintf("v%d", m.firmware.HWVersion)))
		fwStatus := m.firmware.Status
		if m.firmware.IsUpdating && m.firmware.ProgressPercent > 0 {
			fwStatus = fmt.Sprintf("%s (%d%%)", m.firmware.Status, m.firmware.ProgressPercent)
		}
		b.WriteString(m.renderField("Status", fwStatus))
	} else {
		b.WriteString(m.renderField("Version", m.spinner.View()))
		b.WriteString(m.renderField("Hardware", ""))
		b.WriteString(m.renderField("Status", ""))
	}

	// Only show persistent errors (not transient ones)
	if m.errorMsg != "" && !m.loading {
		b.WriteString("\n")
		b.WriteString(m.styles.Error.Render(m.errorMsg))
	}

	return b.String()
}

// formatUptime converts milliseconds to a human-readable format.
func formatUptime(ms int) string {
	seconds := ms / 1000
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm %ds", seconds/60, seconds%60)
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	if hours < 24 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	days := hours / 24
	hours = hours % 24
	return fmt.Sprintf("%dd %dh", days, hours)
}

func (m Model) viewModule() string {
	var b strings.Builder

	// Title bar
	b.WriteString(m.renderTitleBar("Module"))
	b.WriteString("\n\n")

	if !m.connected {
		if m.searching {
			b.WriteString(m.spinner.View())
			b.WriteString(" ")
			b.WriteString(m.styles.Warning.Render("Searching for device..."))
		} else if m.connecting {
			b.WriteString(m.spinner.View())
			b.WriteString(" ")
			b.WriteString(m.styles.Warning.Render("Connecting..."))
		} else {
			connectKey := m.keys.Connect.Help().Key
			b.WriteString(m.styles.Muted.Render(fmt.Sprintf("Press '%s' to connect", connectKey)))
		}
		return b.String()
	}

	// Build module info column
	moduleCol := m.renderModuleInfoColumn("Module Slot", m.moduleDetails, m.moduleInfoLoading && m.moduleDetails == nil)

	// Build snapshot info column
	snapshotCol := m.renderSnapshotInfoColumn("Snapshot Buffer", m.snapshotInfo, m.moduleInfoLoading && m.snapshotInfo == nil)

	// Join columns side by side with gap
	columns := lipgloss.JoinHorizontal(lipgloss.Top, moduleCol, "    ", snapshotCol)
	b.WriteString(columns)
	b.WriteString("\n\n")

	// Menu items
	menuItems := []struct {
		title string
		desc  string
	}{
		{"Read Module", "Read EEPROM from physical SFP module"},
		{"Read Snapshot", "Read from device buffer (last read via device screen)"},
	}

	for i, item := range menuItems {
		if i == m.cursor {
			b.WriteString(m.styles.MenuItemSelected.Render("> " + item.title))
		} else {
			b.WriteString(m.styles.MenuItem.Render("  " + item.title))
		}
		b.WriteString("\n")
		b.WriteString(m.styles.MenuItemDim.Render("    " + item.desc))
		b.WriteString("\n\n")
	}

	// Show loading state for read operations
	if m.moduleLoading {
		b.WriteString(m.spinner.View())
		b.WriteString(" ")
		b.WriteString(m.styles.Warning.Render("Reading..."))
		b.WriteString("\n\n")
	}

	// Show error if any
	if m.moduleError != "" {
		b.WriteString(m.styles.Error.Render(m.moduleError))
		b.WriteString("\n\n")
	}

	// Show success message
	if m.statusMsg != "" && strings.Contains(m.statusMsg, "saved to store") {
		b.WriteString(m.styles.Success.Render(m.statusMsg))
		b.WriteString("\n\n")
	}

	return b.String()
}

// renderModuleInfoColumn renders the module info as a vertical column.
func (m Model) renderModuleInfoColumn(title string, details *api.ModuleDetails, loading bool) string {
	const targetLines = 5 // Match max height for stable layout
	var lines []string

	if loading {
		lines = append(lines, m.styles.Highlight.Render(title)+" "+m.spinner.View())
	} else if details != nil && details.IsModulePresent() {
		lines = append(lines, m.styles.Success.Render("●")+" "+m.styles.Highlight.Render(title))
		lines = append(lines, m.styles.Label.Render("  Part:")+" "+m.styles.Value.Render(details.PartNumber))
		lines = append(lines, m.styles.Label.Render("  Vendor:")+" "+m.styles.Value.Render(details.Vendor))
		lines = append(lines, m.styles.Label.Render("  SN:")+" "+m.styles.Value.Render(details.SN))
		if details.Compliance != "" {
			lines = append(lines, m.styles.Label.Render("  Type:")+" "+m.styles.Value.Render(details.Compliance))
		}
	} else {
		lines = append(lines, m.styles.Error.Render("●")+" "+m.styles.Highlight.Render(title))
		lines = append(lines, m.styles.Muted.Render("  No module inserted"))
	}

	// Pad to fixed height to prevent layout shifts
	for len(lines) < targetLines {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// renderSnapshotInfoColumn renders the snapshot info as a vertical column.
func (m Model) renderSnapshotInfoColumn(title string, info *api.SnapshotInfo, loading bool) string {
	const targetLines = 5 // Match max height for stable layout
	var lines []string

	if loading {
		lines = append(lines, m.styles.Highlight.Render(title)+" "+m.spinner.View())
	} else if info != nil && info.HasData() {
		lines = append(lines, m.styles.Success.Render("●")+" "+m.styles.Highlight.Render(title))
		lines = append(lines, m.styles.Label.Render("  Part:")+" "+m.styles.Value.Render(info.PartNumber))
		lines = append(lines, m.styles.Label.Render("  Vendor:")+" "+m.styles.Value.Render(info.Vendor))
		lines = append(lines, m.styles.Label.Render("  SN:")+" "+m.styles.Value.Render(info.SN))
		lines = append(lines, m.styles.Label.Render("  Size:")+" "+m.styles.Value.Render(fmt.Sprintf("%d bytes", info.Size)))
	} else {
		lines = append(lines, m.styles.Error.Render("●")+" "+m.styles.Highlight.Render(title))
		lines = append(lines, m.styles.Muted.Render("  Module must be present"))
	}

	// Pad to fixed height to prevent layout shifts
	for len(lines) < targetLines {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

func (m Model) viewStore() string {
	var b strings.Builder

	// Title bar
	b.WriteString(m.renderTitleBar("Store"))
	b.WriteString("\n\n")

	if len(m.storeProfiles) == 0 {
		b.WriteString(m.styles.Muted.Render("No profiles in store."))
		b.WriteString("\n\n")
		b.WriteString(m.styles.Muted.Render("Import with: sfpw store import <file>"))
	} else {
		b.WriteString(fmt.Sprintf("%d profile(s)\n\n", len(m.storeProfiles)))

		hashes := m.getSortedHashes()
		for i, hash := range hashes {
			entry := m.storeProfiles[hash]
			shortHash := store.ShortHash(hash)

			// Format wavelength if available
			wavelength := ""
			if entry.WavelengthNM > 0 {
				wavelength = fmt.Sprintf("%dnm", entry.WavelengthNM)
			}

			line := fmt.Sprintf("%-12s  %-16s  %-16s  %s",
				shortHash,
				truncate(entry.VendorName, 16),
				truncate(entry.PartNumber, 16),
				wavelength)

			if i == m.cursor {
				b.WriteString(m.styles.MenuItemSelected.Render("> " + line))
			} else {
				b.WriteString(m.styles.MenuItem.Render("  " + line))
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (m Model) viewStoreDetail() string {
	var b strings.Builder

	// Title bar
	b.WriteString(m.renderTitleBar("Profile"))
	b.WriteString("\n\n")

	if m.selectedHash == "" {
		b.WriteString(m.styles.Error.Render("No profile selected"))
		return b.String()
	}

	entry, ok := m.storeProfiles[m.selectedHash]
	if !ok {
		b.WriteString(m.styles.Error.Render("Profile not found"))
		return b.String()
	}

	// Get full metadata
	s, _ := store.OpenDefault()
	meta, _ := s.GetMetadata(m.selectedHash)

	shortHash := store.ShortHash(m.selectedHash)
	b.WriteString(m.renderField("Hash", shortHash))
	b.WriteString(m.renderField("Type", entry.ModuleType))
	b.WriteString(m.renderField("Vendor", entry.VendorName))
	b.WriteString(m.renderField("Part Number", entry.PartNumber))
	b.WriteString(m.renderField("Serial", entry.SerialNumber))

	if meta != nil {
		if meta.Identity.DateCode != "" {
			b.WriteString(m.renderField("Date Code", meta.Identity.DateCode))
		}
		if meta.Specs.WavelengthNM > 0 {
			b.WriteString(m.renderField("Wavelength", fmt.Sprintf("%d nm", meta.Specs.WavelengthNM)))
		}
		if meta.Specs.ConnectorType != "" {
			b.WriteString(m.renderField("Connector", meta.Specs.ConnectorType))
		}
		b.WriteString(m.renderField("Sources", fmt.Sprintf("%d", len(meta.Sources))))
	}

	b.WriteString("\n")
	b.WriteString(m.styles.Muted.Render("Export: sfpw store export " + shortHash + " <file>"))

	return b.String()
}

func (m Model) viewFirmware() string {
	var b strings.Builder

	// Title bar
	b.WriteString(m.renderTitleBar("Firmware"))
	b.WriteString("\n\n")

	// Show sync progress if syncing
	if m.fwSyncing {
		b.WriteString(m.styles.Highlight.Render("Syncing Firmware Cache"))
		b.WriteString("\n")
		b.WriteString("  ")
		b.WriteString(m.spinner.View())
		if m.fwSyncCurrentVer != "" {
			b.WriteString(fmt.Sprintf(" Downloading %s...", m.fwSyncCurrentVer))
		} else {
			b.WriteString(" Fetching available versions...")
		}
		b.WriteString("\n\n")
	}

	// Current firmware info
	b.WriteString(m.styles.Highlight.Render("Current Version"))
	b.WriteString("\n")

	if m.connected && m.firmware != nil {
		b.WriteString(m.renderField("Version", "v"+m.firmware.FWVersion))
		b.WriteString(m.renderField("Hardware", fmt.Sprintf("v%d", m.firmware.HWVersion)))
		if m.firmware.IsUpdating {
			b.WriteString(m.renderField("Status", fmt.Sprintf("%s (%d%%)", m.firmware.Status, m.firmware.ProgressPercent)))
		}
	} else if m.connected {
		b.WriteString("  ")
		b.WriteString(m.spinner.View())
		b.WriteString(" Loading...")
		b.WriteString("\n")
	} else {
		b.WriteString(m.styles.Muted.Render("  Not connected"))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Selected firmware (for flashing)
	b.WriteString(m.styles.Highlight.Render("Selected Version"))
	b.WriteString("\n")

	if m.fwFlashing {
		b.WriteString("  ")
		b.WriteString(m.spinner.View())
		b.WriteString(fmt.Sprintf(" %s...", m.fwFlashPhase))
		b.WriteString("\n")
	} else if m.fwFlashError != "" {
		b.WriteString(m.styles.Error.Render("  Error: " + m.fwFlashError))
		b.WriteString("\n")
	} else if m.selectedFwVersion != "" {
		b.WriteString(m.renderField("Version", m.selectedFwVersion))
		b.WriteString(m.renderField("Size", humanizeBytesShort(m.selectedFwSize)))
	} else {
		b.WriteString(m.styles.Muted.Render("  None selected"))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Cached firmware versions
	b.WriteString(m.styles.Highlight.Render("Cached Firmware"))
	b.WriteString("\n")

	if len(m.cachedFirmware) > 0 {
		for _, fw := range m.cachedFirmware {
			marker := "  "
			if fw.Version == m.selectedFwVersion {
				marker = "● "
			}
			line := fmt.Sprintf("%s%s  (%s)", marker, fw.Version, humanizeBytesShort(fw.FileSize))
			b.WriteString(line)
			b.WriteString("\n")
		}
	} else if m.fwSyncing {
		b.WriteString(m.styles.Muted.Render("  Syncing..."))
		b.WriteString("\n")
	} else {
		b.WriteString(m.styles.Muted.Render("  No cached versions"))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Build menu items dynamically
	menuItems := m.getFirmwareMenuItems()

	for i, item := range menuItems {
		if i == m.cursor {
			b.WriteString(m.styles.MenuItemSelected.Render("> " + item.title))
		} else {
			b.WriteString(m.styles.MenuItem.Render("  " + item.title))
		}
		b.WriteString("\n")
		b.WriteString(m.styles.Muted.Render("    " + item.desc))
		b.WriteString("\n\n")
	}

	// Error display
	if m.availableFwError != "" && !m.fwSyncing {
		b.WriteString(m.styles.Error.Render(m.availableFwError))
		b.WriteString("\n")
	}

	// Status message
	if m.statusMsg != "" {
		b.WriteString(m.styles.Success.Render(m.statusMsg))
		b.WriteString("\n")
	}

	return b.String()
}

// getFirmwareMenuItems returns the dynamic menu items for firmware view.
func (m Model) getFirmwareMenuItems() []struct{ title, desc string } {
	var items []struct{ title, desc string }

	// If syncing, show minimal menu
	if m.fwSyncing {
		return items
	}

	// Cached version selection
	if len(m.cachedFirmware) > 0 {
		items = append(items, struct{ title, desc string }{
			"Select from Cache",
			fmt.Sprintf("Choose from %d cached versions", len(m.cachedFirmware)),
		})
	}

	// File selection
	items = append(items, struct{ title, desc string }{
		"Select from File",
		"Choose a local firmware file",
	})

	// Refresh
	items = append(items, struct{ title, desc string }{
		"Refresh Cache",
		"Re-sync firmware from cloud",
	})

	// Flash button if a version is selected and connected
	if m.selectedFwVersion != "" && !m.fwFlashing && m.connected {
		items = append(items, struct{ title, desc string }{
			"Flash Firmware",
			fmt.Sprintf("Install %s to device", m.selectedFwVersion),
		})
	}

	// Clear selection if selected
	if m.selectedFwVersion != "" {
		items = append(items, struct{ title, desc string }{
			"Clear Selection",
			"Deselect the current firmware",
		})
	}

	return items
}

func (m Model) viewFirmwareSelect() string {
	var b strings.Builder

	// Title bar
	b.WriteString(m.renderTitleBar("Select Cached Firmware"))
	b.WriteString("\n\n")

	if len(m.cachedFirmware) == 0 {
		b.WriteString(m.styles.Muted.Render("No cached firmware versions"))
		b.WriteString("\n")
		return b.String()
	}

	// Header
	header := fmt.Sprintf("  %-12s  %-10s  %s", "VERSION", "SIZE", "DOWNLOADED")
	b.WriteString(m.styles.Label.Render(header))
	b.WriteString("\n\n")

	// List all cached versions
	for i, fw := range m.cachedFirmware {
		line := fmt.Sprintf("%-12s  %-10s  %s",
			fw.Version,
			humanizeBytesShort(fw.FileSize),
			fw.Downloaded.Format("2006-01-02 15:04"))

		// Mark current version
		if m.firmware != nil && fw.Version == "v"+m.firmware.FWVersion {
			line += " (current)"
		}

		if i == m.cursor {
			b.WriteString(m.styles.MenuItemSelected.Render("> " + line))
		} else {
			b.WriteString(m.styles.MenuItem.Render("  " + line))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(m.styles.Muted.Render("Press Enter to select, Esc to go back"))
	b.WriteString("\n")

	// Show status message if any
	if m.statusMsg != "" {
		b.WriteString("\n")
		b.WriteString(m.styles.Success.Render(m.statusMsg))
		b.WriteString("\n")
	}

	return b.String()
}

func humanizeBytesShort(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func (m Model) renderField(label, value string) string {
	return m.styles.Label.Render(label+":") + " " + m.styles.Value.Render(value) + "\n"
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// --- Async commands for BLE operations ---

// scanForDeviceCmd scans for an SFP Wizard device.
func scanForDeviceCmd() tea.Msg {
	adapter := bluetooth.DefaultAdapter
	if err := adapter.Enable(); err != nil {
		return scanResultMsg{err: fmt.Errorf("bluetooth init failed: %w", err)}
	}

	var deviceResult bluetooth.ScanResult
	var found bool

	// Scan with timeout
	go func() {
		time.Sleep(15 * time.Second)
		adapter.StopScan()
	}()

	err := adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
		name := result.LocalName()
		nameLower := strings.ToLower(name)
		if nameLower == "sfp-wizard" || nameLower == "sfp wizard" || strings.Contains(nameLower, "sfp") {
			deviceResult = result
			found = true
			adapter.StopScan()
		}
	})

	if err != nil {
		return scanResultMsg{err: fmt.Errorf("scan failed: %w", err)}
	}
	if !found {
		return scanResultMsg{err: fmt.Errorf("device not found")}
	}

	// Connect to the found device
	device, err := adapter.Connect(deviceResult.Address, bluetooth.ConnectionParams{})
	if err != nil {
		return scanResultMsg{err: fmt.Errorf("connect failed: %w", err)}
	}

	return scanResultMsg{device: &device}
}

// connectToDeviceCmd sets up the API connection to an already-connected device.
func connectToDeviceCmd(device *bluetooth.Device) tea.Cmd {
	return func() tea.Msg {
		client := api.New(*device)
		if err := client.Connect(); err != nil {
			return connectMsg{err: err}
		}
		return connectMsg{
			client: client,
			mac:    client.MAC(),
		}
	}
}

// fetchStatsCmd fetches device stats.
func fetchStatsCmd(client *api.Client) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return statsMsg{err: fmt.Errorf("not connected")}
		}
		stats, err := client.GetStats()
		return statsMsg{stats: stats, err: err}
	}
}

// fetchDeviceInfoCmd fetches device info.
func fetchDeviceInfoCmd(client *api.Client) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return deviceInfoMsg{err: fmt.Errorf("not connected")}
		}
		info, err := client.GetDeviceInfo()
		return deviceInfoMsg{info: info, err: err}
	}
}

// fetchSettingsCmd fetches device settings.
func fetchSettingsCmd(client *api.Client) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return settingsMsg{err: fmt.Errorf("not connected")}
		}
		settings, err := client.GetSettings()
		return settingsMsg{settings: settings, err: err}
	}
}

// fetchBluetoothCmd fetches bluetooth parameters.
func fetchBluetoothCmd(client *api.Client) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return bluetoothMsg{err: fmt.Errorf("not connected")}
		}
		bt, err := client.GetBluetooth()
		return bluetoothMsg{bluetooth: bt, err: err}
	}
}

// fetchFirmwareCmd fetches firmware status.
func fetchFirmwareCmd(client *api.Client) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return firmwareMsg{err: fmt.Errorf("not connected")}
		}
		fw, err := client.GetFirmwareStatus()
		return firmwareMsg{firmware: fw, err: err}
	}
}

// statusTickCmd returns a command that triggers periodic status updates.
func statusTickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return statusTickMsg(t)
	})
}

// connectionCheckCmd returns a command that triggers periodic connection health checks.
func connectionCheckCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return connectionCheckMsg(t)
	})
}

// fetchAvailableFirmwareCmd fetches available firmware versions from the cloud.
func fetchAvailableFirmwareCmd() tea.Cmd {
	return func() tea.Msg {
		client := firmware.NewManifestClient()
		versions, err := client.GetAvailable(firmware.DefaultSFPWizardFilter())
		return availableFirmwareMsg{versions: versions, err: err}
	}
}

// readModuleCmd reads module EEPROM and saves to store.
func readModuleCmd(client *api.Client, mac string) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return moduleReadMsg{err: fmt.Errorf("not connected")}
		}

		data, err := client.ReadModule()
		if err != nil {
			return moduleReadMsg{err: err}
		}

		// Save to store
		s, err := store.OpenDefault()
		if err != nil {
			return moduleReadMsg{data: data, err: fmt.Errorf("failed to open store: %w", err)}
		}

		source := store.Source{
			DeviceMAC: mac,
			Timestamp: time.Now(),
			Method:    "module_read",
		}

		hash, _, err := s.Import(data, source)
		if err != nil {
			return moduleReadMsg{data: data, err: fmt.Errorf("failed to save to store: %w", err)}
		}

		return moduleReadMsg{data: data, hash: hash}
	}
}

// readSnapshotCmd reads snapshot buffer and saves to store.
func readSnapshotCmd(client *api.Client, mac string) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return snapshotReadMsg{err: fmt.Errorf("not connected")}
		}

		data, err := client.ReadSnapshot()
		if err != nil {
			return snapshotReadMsg{err: err}
		}

		// Save to store
		s, err := store.OpenDefault()
		if err != nil {
			return snapshotReadMsg{data: data, err: fmt.Errorf("failed to open store: %w", err)}
		}

		source := store.Source{
			DeviceMAC: mac,
			Timestamp: time.Now(),
			Method:    "snapshot_read",
		}

		hash, _, err := s.Import(data, source)
		if err != nil {
			return snapshotReadMsg{data: data, err: fmt.Errorf("failed to save to store: %w", err)}
		}

		return snapshotReadMsg{data: data, hash: hash}
	}
}

// fetchModuleDetailsCmd fetches module details.
func fetchModuleDetailsCmd(client *api.Client) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return moduleDetailsMsg{err: fmt.Errorf("not connected")}
		}
		details, err := client.GetModuleDetails()
		return moduleDetailsMsg{details: details, err: err}
	}
}

// fetchSnapshotInfoCmd fetches snapshot buffer info.
func fetchSnapshotInfoCmd(client *api.Client) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return snapshotInfoMsg{err: fmt.Errorf("not connected")}
		}
		info, err := client.GetSnapshotInfo()
		return snapshotInfoMsg{info: info, err: err}
	}
}

// moduleInfoTickCmd returns a command that triggers periodic module info updates.
func moduleInfoTickCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return moduleInfoTickMsg(t)
	})
}

// importFirmwareFileCmd imports a local file into the firmware cache.
func importFirmwareFileCmd(path string) tea.Cmd {
	return func() tea.Msg {
		cache, err := firmware.NewFirmwareStore()
		if err != nil {
			return firmwareImportedMsg{err: err}
		}
		cachePath, sha256, size, err := cache.ImportFile(path)
		if err != nil {
			return firmwareImportedMsg{err: err}
		}
		// Extract version from path
		base := filepath.Base(path)
		version := strings.TrimSuffix(base, filepath.Ext(base))
		return firmwareImportedMsg{
			version: version,
			path:    cachePath,
			size:    size,
			sha256:  sha256,
		}
	}
}

// downloadFirmwareCmd downloads a firmware version to the cache.
func downloadFirmwareCmd(fw firmware.FirmwareVersion) tea.Cmd {
	return func() tea.Msg {
		cache, err := firmware.NewFirmwareStore()
		if err != nil {
			return firmwareDownloadedMsg{err: err}
		}
		path, err := cache.Download(fw, nil)
		if err != nil {
			return firmwareDownloadedMsg{err: err}
		}
		return firmwareDownloadedMsg{
			version: fw.Version,
			path:    path,
			size:    fw.FileSize,
			sha256:  fw.SHA256,
		}
	}
}

// flashFirmwareCmd flashes firmware to the device.
func flashFirmwareCmd(client *api.Client, path string) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return firmwareFlashCompleteMsg{err: fmt.Errorf("not connected")}
		}

		// Read firmware file
		data, err := os.ReadFile(path)
		if err != nil {
			return firmwareFlashCompleteMsg{err: fmt.Errorf("failed to read file: %w", err)}
		}

		// Use the client to update firmware (no progress callback for simplicity)
		err = client.UpdateFirmware(data, nil)
		if err != nil {
			return firmwareFlashCompleteMsg{err: err}
		}

		return firmwareFlashCompleteMsg{
			success: true,
			message: "Firmware update complete! Device may reboot.",
		}
	}
}

// syncFirmwareCacheCmd fetches available firmware and downloads all missing versions.
func syncFirmwareCacheCmd() tea.Cmd {
	return func() tea.Msg {
		// Create cache and manifest client
		cache, err := firmware.NewFirmwareStore()
		if err != nil {
			return firmwareSyncCompleteMsg{err: fmt.Errorf("cache error: %w", err)}
		}

		client := firmware.NewManifestClient()
		versions, err := client.GetAvailable(firmware.DefaultSFPWizardFilter())
		if err != nil {
			return firmwareSyncCompleteMsg{err: fmt.Errorf("fetch error: %w", err)}
		}

		// Download missing versions
		for i, v := range versions {
			// Check if already cached
			if cache.Has(v.Version, v.SHA256) {
				continue
			}

			// Download with simple progress (no callback to TUI for now - would need channels)
			_, err := cache.Download(v, nil)
			if err != nil {
				// Log but continue with other versions
				continue
			}

			// Small delay between downloads
			if i < len(versions)-1 {
				time.Sleep(100 * time.Millisecond)
			}
		}

		// Get final cached list
		cached, _ := cache.List()

		return firmwareSyncCompleteMsg{
			versions: versions,
			cached:   cached,
		}
	}
}

// refreshCachedFirmwareCmd just refreshes the cached firmware list without downloading.
func refreshCachedFirmwareCmd() tea.Cmd {
	return func() tea.Msg {
		cache, err := firmware.NewFirmwareStore()
		if err != nil {
			return cachedFirmwareMsg{}
		}
		cached, _ := cache.List()
		return cachedFirmwareMsg{cached: cached}
	}
}
