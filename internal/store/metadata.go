package store

import (
	"strings"
	"time"
)

// Metadata contains parsed information about a module profile.
type Metadata struct {
	ContentHash string     `json:"content_hash"`
	ModuleType  string     `json:"module_type"` // "SFP", "QSFP", "QSFP+", "QSFP28"
	Size        int        `json:"size"`
	Identity    Identity   `json:"identity"`
	Specs       Specs      `json:"specs,omitempty"`
	Compliance  []string   `json:"compliance,omitempty"`
	Checksums   Checksums  `json:"checksums,omitempty"`
	Sources     []Source   `json:"sources"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Identity contains vendor and serial information.
type Identity struct {
	VendorName   string `json:"vendor_name"`
	VendorOUI    string `json:"vendor_oui,omitempty"`
	PartNumber   string `json:"part_number"`
	Revision     string `json:"revision,omitempty"`
	SerialNumber string `json:"serial_number"`
	DateCode     string `json:"date_code,omitempty"`
}

// Specs contains module specifications.
type Specs struct {
	ConnectorType string  `json:"connector_type,omitempty"`
	WavelengthNM  int     `json:"wavelength_nm,omitempty"`
	BitrateMbps   int     `json:"bitrate_mbps,omitempty"`
	Encoding      string  `json:"encoding,omitempty"`
	LinkLengthM   int     `json:"link_length_m,omitempty"`
}

// Checksums contains checksum validation results.
type Checksums struct {
	CCBase string `json:"cc_base,omitempty"`
	CCExt  string `json:"cc_ext,omitempty"`
	Valid  bool   `json:"valid"`
}

// Source records where a profile was obtained from.
type Source struct {
	DeviceMAC string    `json:"device_mac,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Method    string    `json:"method"` // "module_read", "snapshot_read", "support_dump", "import"
	Filename  string    `json:"filename,omitempty"`
}

// ExtractMetadata parses EEPROM data and extracts metadata.
func ExtractMetadata(data []byte, hash string) *Metadata {
	if len(data) < 96 {
		return nil
	}

	identifier := data[0]
	moduleType := "Unknown"
	switch identifier {
	case 0x03:
		moduleType = "SFP"
	case 0x0c:
		moduleType = "QSFP"
	case 0x0d:
		moduleType = "QSFP+"
	case 0x11:
		moduleType = "QSFP28"
	}

	meta := &Metadata{
		ContentHash: hash,
		ModuleType:  moduleType,
		Size:        len(data),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Extract identity fields (SFP layout - QSFP is different)
	if identifier == 0x03 && len(data) >= 96 {
		meta.Identity = Identity{
			VendorName:   strings.TrimSpace(string(data[20:36])),
			VendorOUI:    formatOUI(data[37:40]),
			PartNumber:   strings.TrimSpace(string(data[40:56])),
			Revision:     strings.TrimSpace(string(data[56:60])),
			SerialNumber: strings.TrimSpace(string(data[68:84])),
			DateCode:     strings.TrimSpace(string(data[84:92])),
		}

		// Extract specs
		meta.Specs = Specs{
			ConnectorType: connectorType(data[2]),
			BitrateMbps:   int(data[12]) * 100,
			Encoding:      encodingType(data[11]),
		}

		// Wavelength (bytes 60-61, units of nm)
		if data[60] != 0 || data[61] != 0 {
			meta.Specs.WavelengthNM = int(data[60])<<8 | int(data[61])
		}

		// Calculate checksums
		if len(data) >= 64 {
			ccBase := calculateChecksum(data[0:63])
			meta.Checksums.CCBase = formatHex(ccBase)
			meta.Checksums.Valid = (ccBase == data[63])
		}
	} else if (identifier == 0x0c || identifier == 0x0d || identifier == 0x11) && len(data) >= 192 {
		// QSFP layout - identity in upper memory (bytes 128+)
		meta.Identity = Identity{
			VendorName:   strings.TrimSpace(string(data[148:164])),
			PartNumber:   strings.TrimSpace(string(data[168:184])),
			Revision:     strings.TrimSpace(string(data[184:186])),
			SerialNumber: strings.TrimSpace(string(data[196:212])),
			DateCode:     strings.TrimSpace(string(data[212:220])),
		}
	}

	return meta
}

func formatOUI(data []byte) string {
	if len(data) < 3 {
		return ""
	}
	return formatHex(data[0]) + ":" + formatHex(data[1]) + ":" + formatHex(data[2])
}

func formatHex(b byte) string {
	const hex = "0123456789ABCDEF"
	return string([]byte{hex[b>>4], hex[b&0x0F]})
}

func calculateChecksum(data []byte) byte {
	var sum byte
	for _, b := range data {
		sum += b
	}
	return sum
}

func connectorType(b byte) string {
	switch b {
	case 0x01:
		return "SC"
	case 0x07:
		return "LC"
	case 0x0B:
		return "Optical Pigtail"
	case 0x21:
		return "Copper Pigtail"
	case 0x22:
		return "RJ45"
	case 0x23:
		return "No Separable Connector"
	default:
		return ""
	}
}

func encodingType(b byte) string {
	switch b {
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
		return ""
	}
}
