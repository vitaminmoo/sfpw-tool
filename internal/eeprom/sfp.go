package eeprom

import (
	"fmt"
	"strings"
)

// ParseSFPDetailed parses SFP EEPROM data per SFF-8472
func ParseSFPDetailed(data []byte) {
	// === Basic Info (A0h page) ===
	fmt.Println("--- Basic Info ---")

	// Byte 0: Identifier
	identStr := "Unknown"
	switch data[0] {
	case 0x01:
		identStr = "GBIC"
	case 0x02:
		identStr = "Module soldered to motherboard"
	case 0x03:
		identStr = "SFP/SFP+"
	case 0x04:
		identStr = "300 pin XBI"
	case 0x05:
		identStr = "XENPAK"
	case 0x06:
		identStr = "XFP"
	case 0x07:
		identStr = "XFF"
	case 0x08:
		identStr = "XFP-E"
	case 0x09:
		identStr = "XPAK"
	case 0x0A:
		identStr = "X2"
	}
	fmt.Printf("Identifier:       0x%02X (%s)\n", data[0], identStr)

	// Byte 1: Extended Identifier
	fmt.Printf("Ext Identifier:   0x%02X\n", data[1])

	// Byte 2: Connector Type
	connStr := GetConnectorType(data[2])
	fmt.Printf("Connector:        0x%02X (%s)\n", data[2], connStr)

	// Bytes 3-10: Transceiver compliance codes
	fmt.Println("\n--- Transceiver Compliance ---")
	PrintTransceiverCodes(data[3:11])

	// Byte 11: Encoding
	encStr := GetEncodingType(data[11])
	fmt.Printf("Encoding:         0x%02X (%s)\n", data[11], encStr)

	// Byte 12: Nominal Bit Rate (units of 100 MBd)
	bitrate := int(data[12]) * 100
	fmt.Printf("Nominal Bitrate:  %d MBd\n", bitrate)

	// Byte 13: Rate Identifier
	fmt.Printf("Rate Identifier:  0x%02X\n", data[13])

	// Bytes 14-19: Link length
	fmt.Println("\n--- Link Length ---")
	if data[14] > 0 {
		fmt.Printf("Single Mode (km): %d km\n", int(data[14]))
	}
	if data[15] > 0 {
		fmt.Printf("Single Mode (m):  %d00 m\n", int(data[15]))
	}
	if data[16] > 0 {
		fmt.Printf("50um OM2:         %d0 m\n", int(data[16]))
	}
	if data[17] > 0 {
		fmt.Printf("62.5um OM1:       %d0 m\n", int(data[17]))
	}
	if data[18] > 0 {
		fmt.Printf("Copper/OM4:       %d m\n", int(data[18]))
	}
	if data[19] > 0 {
		fmt.Printf("OM3:              %d0 m\n", int(data[19]))
	}

	// Vendor info
	fmt.Println("\n--- Vendor Info ---")
	vendorName := strings.TrimSpace(string(data[20:36]))
	fmt.Printf("Vendor Name:      %s\n", vendorName)

	// Bytes 37-39: Vendor OUI
	oui := fmt.Sprintf("%02X:%02X:%02X", data[37], data[38], data[39])
	fmt.Printf("Vendor OUI:       %s\n", oui)

	vendorPN := strings.TrimSpace(string(data[40:56]))
	fmt.Printf("Part Number:      %s\n", vendorPN)

	vendorRev := strings.TrimSpace(string(data[56:60]))
	fmt.Printf("Revision:         %s\n", vendorRev)

	// Bytes 60-61: Wavelength (in nm) or copper cable attenuation
	wavelength := int(data[60])<<8 | int(data[61])
	if wavelength > 0 && wavelength < 2000 {
		fmt.Printf("Wavelength:       %d nm\n", wavelength)
	} else if wavelength > 0 {
		// Might be copper cable attenuation
		fmt.Printf("Cable Atten:      %d (raw value)\n", wavelength)
	}

	// Byte 62: Unallocated
	// Byte 63: CC_BASE (checksum)

	vendorSN := strings.TrimSpace(string(data[68:84]))
	fmt.Printf("Serial Number:    %s\n", vendorSN)

	// Bytes 84-91: Date code (YYMMDDLL)
	dateCode := string(data[84:92])
	if len(dateCode) >= 6 {
		year := dateCode[0:2]
		month := dateCode[2:4]
		day := dateCode[4:6]
		lot := ""
		if len(dateCode) >= 8 {
			lot = dateCode[6:8]
		}
		fmt.Printf("Date Code:        20%s-%s-%s (Lot: %s)\n", year, month, day, lot)
	}

	// Diagnostic Monitoring Type (byte 92)
	fmt.Println("\n--- Diagnostic Monitoring ---")
	dmType := data[92]
	fmt.Printf("Diag Type:        0x%02X\n", dmType)
	if dmType&0x40 != 0 {
		fmt.Println("  - Digital diagnostics implemented")
	}
	if dmType&0x20 != 0 {
		fmt.Println("  - Internally calibrated")
	}
	if dmType&0x10 != 0 {
		fmt.Println("  - Externally calibrated")
	}
	if dmType&0x08 != 0 {
		fmt.Println("  - Received power measurement: average")
	} else {
		fmt.Println("  - Received power measurement: OMA")
	}
	if dmType&0x04 != 0 {
		fmt.Println("  - Address change required")
	}

	// Enhanced Options (byte 93)
	enhOpts := data[93]
	fmt.Printf("Enhanced Opts:    0x%02X\n", enhOpts)

	// SFF-8472 Compliance (byte 94)
	compliance := data[94]
	compStr := "Unknown"
	switch compliance {
	case 0:
		compStr = "Not specified"
	case 1:
		compStr = "SFF-8472 Rev 9.3"
	case 2:
		compStr = "SFF-8472 Rev 9.5"
	case 3:
		compStr = "SFF-8472 Rev 10.2"
	case 4:
		compStr = "SFF-8472 Rev 10.4"
	case 5:
		compStr = "SFF-8472 Rev 11.0"
	case 6:
		compStr = "SFF-8472 Rev 11.3"
	case 7:
		compStr = "SFF-8472 Rev 11.4"
	case 8:
		compStr = "SFF-8472 Rev 12.0"
	}
	fmt.Printf("SFF-8472 Rev:     0x%02X (%s)\n", compliance, compStr)

	// Checksum validation
	fmt.Println("\n--- Checksums ---")
	// CC_BASE covers bytes 0-62
	var ccBase byte
	for i := 0; i < 63; i++ {
		ccBase += data[i]
	}
	storedCCBase := data[63]
	if ccBase == storedCCBase {
		fmt.Printf("CC_BASE:          0x%02X (VALID)\n", storedCCBase)
	} else {
		fmt.Printf("CC_BASE:          0x%02X (INVALID - calculated 0x%02X)\n", storedCCBase, ccBase)
	}

	// CC_EXT covers bytes 64-94
	var ccExt byte
	for i := 64; i < 95; i++ {
		ccExt += data[i]
	}
	storedCCExt := data[95]
	if ccExt == storedCCExt {
		fmt.Printf("CC_EXT:           0x%02X (VALID)\n", storedCCExt)
	} else {
		fmt.Printf("CC_EXT:           0x%02X (INVALID - calculated 0x%02X)\n", storedCCExt, ccExt)
	}

	// A2h page (if present - bytes 256-511)
	if len(data) >= 512 {
		fmt.Println("\n--- A2h Page (Diagnostic Data) ---")
		a2 := data[256:]

		// Alarm and warning thresholds (bytes 0-55)
		// Real-time diagnostics (bytes 96-105)
		if len(a2) >= 106 {
			// Temperature: bytes 96-97 (signed, 1/256 degree C)
			tempRaw := int16(a2[96])<<8 | int16(a2[97])
			temp := float64(tempRaw) / 256.0
			fmt.Printf("Temperature:      %.1f C\n", temp)

			// Vcc: bytes 98-99 (unsigned, 100 uV)
			vccRaw := uint16(a2[98])<<8 | uint16(a2[99])
			vcc := float64(vccRaw) / 10000.0
			fmt.Printf("Supply Voltage:   %.2f V\n", vcc)

			// TX Bias: bytes 100-101 (unsigned, 2 uA)
			biasRaw := uint16(a2[100])<<8 | uint16(a2[101])
			bias := float64(biasRaw) * 2 / 1000.0
			fmt.Printf("TX Bias Current:  %.1f mA\n", bias)

			// TX Power: bytes 102-103 (unsigned, 0.1 uW)
			txPowerRaw := uint16(a2[102])<<8 | uint16(a2[103])
			txPowerMw := float64(txPowerRaw) / 10000.0
			txPowerDbm := 10 * (Log10(txPowerMw))
			fmt.Printf("TX Power:         %.2f mW (%.1f dBm)\n", txPowerMw, txPowerDbm)

			// RX Power: bytes 104-105 (unsigned, 0.1 uW)
			rxPowerRaw := uint16(a2[104])<<8 | uint16(a2[105])
			rxPowerMw := float64(rxPowerRaw) / 10000.0
			rxPowerDbm := 10 * (Log10(rxPowerMw))
			fmt.Printf("RX Power:         %.2f mW (%.1f dBm)\n", rxPowerMw, rxPowerDbm)
		}
	}
}
