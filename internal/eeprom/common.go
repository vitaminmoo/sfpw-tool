package eeprom

import (
	"fmt"
	"math"
)

// Log10 returns log base 10, handling zero
func Log10(x float64) float64 {
	if x <= 0 {
		return -40.0 // Return a very low dBm for zero power
	}
	return math.Log10(x)
}

// GetConnectorType returns a string description for connector type code
func GetConnectorType(code byte) string {
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

// GetEncodingType returns a string description for encoding type code
func GetEncodingType(code byte) string {
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

// PrintTransceiverCodes prints transceiver compliance codes
func PrintTransceiverCodes(codes []byte) {
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
