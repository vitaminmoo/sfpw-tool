package eeprom

import (
	"fmt"
	"strings"
)

// ParseInfo extracts and displays SFP module information from EEPROM data
// This is a compact single-line output format
func ParseInfo(data []byte) {
	// A0h page (first 256 bytes of EEPROM)
	// Based on SFF-8472 specification

	if len(data) < 96 {
		fmt.Println("           (insufficient data)")
		return
	}

	// Byte 0: Identifier (03 = SFP)
	identifier := data[0]
	idStr := "Unknown"
	switch identifier {
	case 0x03:
		idStr = "SFP/SFP+"
	case 0x0d:
		idStr = "QSFP+"
	case 0x11:
		idStr = "QSFP28"
	}

	// Bytes 20-35: Vendor Name (16 bytes ASCII)
	vendorName := strings.TrimSpace(string(data[20:36]))

	// Bytes 40-55: Vendor Part Number (16 bytes ASCII)
	vendorPN := strings.TrimSpace(string(data[40:56]))

	// Bytes 68-83: Vendor Serial Number (16 bytes ASCII)
	vendorSN := strings.TrimSpace(string(data[68:84]))

	// Byte 12: Nominal Bit Rate (units of 100 MBd)
	bitrate := int(data[12]) * 100

	// Bytes 60-61: Wavelength (in nm)
	wavelength := 0
	if len(data) >= 62 {
		wavelength = int(data[60])<<8 | int(data[61])
	}

	// Compact single-line output
	fmt.Printf("           %s: %s %s (S/N: %s) %dMBd %dnm\n",
		idStr, vendorName, vendorPN, vendorSN, bitrate, wavelength)
}
