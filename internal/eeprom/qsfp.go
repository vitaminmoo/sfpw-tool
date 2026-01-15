package eeprom

import (
	"fmt"
	"strings"
)

// ParseQSFPDetailed parses QSFP EEPROM data per SFF-8636
func ParseQSFPDetailed(data []byte) {
	// QSFP has different layout - Page 00h starts at byte 128
	if len(data) < 256 {
		fmt.Printf("ERROR: Insufficient data for QSFP parsing (need 256+ bytes)\n")
		return
	}

	fmt.Println("--- Basic Info ---")

	// Byte 128: Identifier
	identStr := "Unknown"
	switch data[128] {
	case 0x0c:
		identStr = "QSFP"
	case 0x0d:
		identStr = "QSFP+"
	case 0x11:
		identStr = "QSFP28"
	}
	fmt.Printf("Identifier:       0x%02X (%s)\n", data[128], identStr)

	// Connector type at byte 130
	connStr := GetConnectorType(data[130])
	fmt.Printf("Connector:        0x%02X (%s)\n", data[130], connStr)

	// Vendor info
	fmt.Println("\n--- Vendor Info ---")
	vendorName := strings.TrimSpace(string(data[148:164]))
	fmt.Printf("Vendor Name:      %s\n", vendorName)

	vendorPN := strings.TrimSpace(string(data[168:184]))
	fmt.Printf("Part Number:      %s\n", vendorPN)

	vendorRev := strings.TrimSpace(string(data[184:186]))
	fmt.Printf("Revision:         %s\n", vendorRev)

	vendorSN := strings.TrimSpace(string(data[196:212]))
	fmt.Printf("Serial Number:    %s\n", vendorSN)

	// Date code (bytes 212-219)
	dateCode := string(data[212:220])
	if len(dateCode) >= 6 {
		year := dateCode[0:2]
		month := dateCode[2:4]
		day := dateCode[4:6]
		fmt.Printf("Date Code:        20%s-%s-%s\n", year, month, day)
	}

	// Real-time monitoring data is in lower page (bytes 22-33 for temps, voltages, etc)
	fmt.Println("\n--- Real-time Diagnostics ---")
	// Temperature at bytes 22-23
	if len(data) >= 24 {
		tempRaw := int16(data[22])<<8 | int16(data[23])
		temp := float64(tempRaw) / 256.0
		fmt.Printf("Temperature:      %.1f C\n", temp)
	}

	// Vcc at bytes 26-27
	if len(data) >= 28 {
		vccRaw := uint16(data[26])<<8 | uint16(data[27])
		vcc := float64(vccRaw) / 10000.0
		fmt.Printf("Supply Voltage:   %.2f V\n", vcc)
	}
}
