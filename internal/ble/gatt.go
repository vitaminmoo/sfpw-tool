package ble

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/vitaminmoo/sfpw-tool/internal/config"

	"tinygo.org/x/bluetooth"
)

// GATTContext holds the BLE characteristics for Service 3 text commands.
// Service 3 uses simple text-based commands (getVer, powerOff, chargeCtrl).
type GATTContext struct {
	CommandChar *bluetooth.DeviceCharacteristic // Write commands (9280f26c)
	NotifyChar  *bluetooth.DeviceCharacteristic // Receive responses (d587c47f)
	InfoChar    *bluetooth.DeviceCharacteristic // Device info read (dc272a22)

	responseMu      sync.Mutex
	responseBuf     []byte
	responseChan    chan []byte
	notifyEnabled   bool
}

// SetupGATT discovers Service 3 characteristics for text-based GATT commands.
// Service 3 (8e60f02e) provides direct device control via plain text commands.
func SetupGATT(device bluetooth.Device) *GATTContext {
	config.Debugf("Discovering services for GATT commands...")

	allServices, err := device.DiscoverServices(nil)
	if err != nil {
		log.Fatal("Failed to discover services:", err)
	}

	// Find Service 3 (Device Info & Control)
	var service3 *bluetooth.DeviceService
	for i := range allServices {
		uuidStr := allServices[i].UUID().String()
		if strings.EqualFold(uuidStr, SFPServiceUUID) {
			service3 = &allServices[i]
			config.Debugf("Found Service 3: %s", uuidStr)
			break
		}
	}

	if service3 == nil {
		log.Fatal("Service 3 not found")
	}

	// Discover characteristics
	chars, err := service3.DiscoverCharacteristics(nil)
	if err != nil {
		log.Fatal("Failed to discover characteristics:", err)
	}

	ctx := &GATTContext{
		responseChan: make(chan []byte, 1),
	}

	// Find characteristics
	for i := range chars {
		uuidStr := chars[i].UUID().String()
		config.Debugf("Found characteristic: %s", uuidStr)
		if strings.EqualFold(uuidStr, SFPWriteCharUUID) {
			ctx.CommandChar = &chars[i]
		}
		if strings.EqualFold(uuidStr, SFPSecondaryNotifyUUID) {
			ctx.NotifyChar = &chars[i]
		}
		if strings.EqualFold(uuidStr, SFPNotifyCharUUID) {
			ctx.InfoChar = &chars[i]
		}
	}

	if ctx.CommandChar == nil {
		log.Fatal("Command characteristic not found")
	}
	if ctx.InfoChar == nil {
		log.Fatal("Info characteristic (dc272a22) not found")
	}

	return ctx
}

// enableNotifications sets up the notification handler for command responses.
// Responses come via gatt_send_notification on the same characteristic (InfoChar/dc272a22).
func (ctx *GATTContext) enableNotifications() error {
	if ctx.notifyEnabled {
		return nil
	}

	err := ctx.InfoChar.EnableNotifications(func(buf []byte) {
		ctx.responseMu.Lock()
		defer ctx.responseMu.Unlock()

		config.Debugf("GATT notification received: %d bytes", len(buf))
		config.Debugf("Response: %s", string(buf))

		// Store response and signal completion
		ctx.responseBuf = make([]byte, len(buf))
		copy(ctx.responseBuf, buf)

		select {
		case ctx.responseChan <- ctx.responseBuf:
		default:
		}
	})
	if err != nil {
		return err
	}

	ctx.notifyEnabled = true
	time.Sleep(100 * time.Millisecond)
	return nil
}

// SendCommand sends a text command and waits for a response.
// Returns the response data, or nil if no response expected.
// Note: Despite API.md saying dc272a22 is read-only, firmware shows it accepts
// write commands (getVer, powerOff, chargeCtrl) via ui_gatt_service_factory_cb.
func (ctx *GATTContext) SendCommand(command string, timeout time.Duration) ([]byte, error) {
	if err := ctx.enableNotifications(); err != nil {
		return nil, fmt.Errorf("failed to enable notifications: %w", err)
	}

	// Drain any pending responses
	select {
	case <-ctx.responseChan:
	default:
	}

	config.Debugf("Sending GATT command: %s", command)
	config.Debugf("Command bytes: %X", []byte(command))

	// Write to InfoChar (dc272a22) - firmware ui_gatt_service_factory_cb handles
	// both READ (device info) and WRITE (commands) on this characteristic
	n, err := ctx.InfoChar.WriteWithoutResponse([]byte(command))
	if err != nil {
		return nil, fmt.Errorf("failed to write command: %w", err)
	}
	config.Debugf("Wrote %d bytes to InfoChar", n)

	// Wait for response
	select {
	case resp := <-ctx.responseChan:
		return resp, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for response")
	}
}

// SendCommandNoResponse sends a text command that doesn't expect a response.
// Used for commands like powerOff where the device shuts down immediately.
func (ctx *GATTContext) SendCommandNoResponse(command string) error {
	config.Debugf("Sending GATT command (no response expected): %s", command)
	config.Debugf("Command bytes: %X", []byte(command))

	// Write to InfoChar (dc272a22) - firmware ui_gatt_service_factory_cb handles commands
	n, err := ctx.InfoChar.WriteWithoutResponse([]byte(command))
	if err != nil {
		return fmt.Errorf("failed to write command: %w", err)
	}
	config.Debugf("Wrote %d bytes to InfoChar", n)

	// Give device time to process
	time.Sleep(100 * time.Millisecond)

	return nil
}
