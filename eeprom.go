package main

import (
	"fmt"
	"math"
	"strings"
)

// parseSFPEEPROMDetailed parses SFP EEPROM data per SFF-8472
func parseSFPEEPROMDetailed(data []byte) {
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
	connStr := getConnectorType(data[2])
	fmt.Printf("Connector:        0x%02X (%s)\n", data[2], connStr)

	// Bytes 3-10: Transceiver compliance codes
	fmt.Println("\n--- Transceiver Compliance ---")
	parseTransceiverCodes(data[3:11])

	// Byte 11: Encoding
	encStr := getEncodingType(data[11])
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
			txPowerDbm := 10 * (log10(txPowerMw))
			fmt.Printf("TX Power:         %.2f mW (%.1f dBm)\n", txPowerMw, txPowerDbm)

			// RX Power: bytes 104-105 (unsigned, 0.1 uW)
			rxPowerRaw := uint16(a2[104])<<8 | uint16(a2[105])
			rxPowerMw := float64(rxPowerRaw) / 10000.0
			rxPowerDbm := 10 * (log10(rxPowerMw))
			fmt.Printf("RX Power:         %.2f mW (%.1f dBm)\n", rxPowerMw, rxPowerDbm)
		}
	}
}

// log10 returns log base 10, handling zero
func log10(x float64) float64 {
	if x <= 0 {
		return -40.0 // Return a very low dBm for zero power
	}
	return math.Log10(x)
}

// parseQSFPEEPROMDetailed parses QSFP EEPROM data per SFF-8636
func parseQSFPEEPROMDetailed(data []byte) {
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
	connStr := getConnectorType(data[130])
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

// getConnectorType returns a string description for connector type code
func getConnectorType(code byte) string {
	switch code {
	case 0x00:
		return "Unknown"
	case 0x01:
		return "SC"
	case 0x02:
		return "FC Style 1"
	case 0x03:
		return "FC Style 2"
	case 0x04:
		return "BNC/TNC"
	case 0x05:
		return "FC coax"
	case 0x06:
		return "Fiber Jack"
	case 0x07:
		return "LC"
	case 0x08:
		return "MT-RJ"
	case 0x09:
		return "MU"
	case 0x0A:
		return "SG"
	case 0x0B:
		return "Optical Pigtail"
	case 0x0C:
		return "MPO 1x12"
	case 0x0D:
		return "MPO 2x16"
	case 0x20:
		return "HSSDC II"
	case 0x21:
		return "Copper Pigtail"
	case 0x22:
		return "RJ45"
	case 0x23:
		return "No separable connector"
	case 0x24:
		return "MXC 2x16"
	default:
		return "Vendor specific"
	}
}

// getEncodingType returns a string description for encoding type code
func getEncodingType(code byte) string {
	switch code {
	case 0x00:
		return "Unspecified"
	case 0x01:
		return "8B/10B"
	case 0x02:
		return "4B/5B"
	case 0x03:
		return "NRZ"
	case 0x04:
		return "Manchester"
	case 0x05:
		return "SONET Scrambled"
	case 0x06:
		return "64B/66B"
	default:
		return "Unknown"
	}
}

// parseTransceiverCodes prints transceiver compliance codes
func parseTransceiverCodes(codes []byte) {
	// Byte 3: 10G Ethernet / Infiniband
	if codes[0]&0x80 != 0 {
		fmt.Println("  - 10G Base-ER")
	}
	if codes[0]&0x40 != 0 {
		fmt.Println("  - 10G Base-LRM")
	}
	if codes[0]&0x20 != 0 {
		fmt.Println("  - 10G Base-LR")
	}
	if codes[0]&0x10 != 0 {
		fmt.Println("  - 10G Base-SR")
	}

	// Byte 6: Gigabit Ethernet
	if codes[3]&0x08 != 0 {
		fmt.Println("  - 1000BASE-T")
	}
	if codes[3]&0x04 != 0 {
		fmt.Println("  - 1000BASE-CX")
	}
	if codes[3]&0x02 != 0 {
		fmt.Println("  - 1000BASE-LX")
	}
	if codes[3]&0x01 != 0 {
		fmt.Println("  - 1000BASE-SX")
	}

	// Byte 8: SFP+ Cable Technology
	if codes[5]&0x08 != 0 {
		fmt.Println("  - Active Cable")
	}
	if codes[5]&0x04 != 0 {
		fmt.Println("  - Passive Cable")
	}

	// Byte 9: Fibre Channel transmission media
	if codes[6]&0x80 != 0 {
		fmt.Println("  - Twin Axial Pair (TW)")
	}
	if codes[6]&0x40 != 0 {
		fmt.Println("  - Twisted Pair (TP)")
	}
	if codes[6]&0x20 != 0 {
		fmt.Println("  - Miniature Coax (MI)")
	}
	if codes[6]&0x10 != 0 {
		fmt.Println("  - Video Coax (TV)")
	}
	if codes[6]&0x08 != 0 {
		fmt.Println("  - Multi-mode 62.5um (M6)")
	}
	if codes[6]&0x04 != 0 {
		fmt.Println("  - Multi-mode 50um (M5)")
	}
	if codes[6]&0x02 != 0 {
		fmt.Println("  - Single Mode (SM)")
	}
}

// parseSFPInfo extracts and displays SFP module information from EEPROM data
func parseSFPInfo(data []byte) {
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
