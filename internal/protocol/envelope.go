package protocol

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
)

// Standard binme section type constants (from upstream binme library)
const (
	TypeHeader = 0x01 // Header section type (standard binme)
	TypeBody   = 0x02 // Body section type (standard binme)
)

// Device-specific section type constants
// The SFP Wizard device uses a modified binme format with different type values
const (
	DeviceTypeHeader = 0x03 // Header section type (device uses 0x03, not standard 0x01)
	DeviceTypeBody   = 0x02 // Body section type (matches standard binme)
)

// Binme format constants (from upstream binme library)
const (
	FormatJSON   = 0x01 // JSON data
	FormatString = 0x02 // UTF-8 string
	FormatBinary = 0x03 // Raw binary data
)

// zlibCompress compresses data using zlib
func zlibCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, err := w.Write(data)
	if err != nil {
		return nil, err
	}
	err = w.Close()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// zlibDecompress decompresses zlib data
func zlibDecompress(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// BinmeEncode wraps JSON data in the device's modified binme binary envelope format.
//
// Note: The SFP Wizard device uses a modified binme format that differs from
// the standard binme library. Key differences:
//   - Header section uses type 0x03 instead of standard 0x01
//   - Header section is 9 bytes (vs standard 8), with single-byte length at byte 8
//   - Body section matches standard format (8 bytes, type 0x02)
//
// Device message format:
//
//	[Device Transport Header - 4 bytes]
//	  bytes 0-1: total message length (big-endian, includes this header)
//	  bytes 2-3: sequence number (big-endian, matches request ID)
//
//	[Header Section - 9 bytes + data] (device-specific format)
//	  byte 0: type (0x03 = DeviceTypeHeader)
//	  byte 1: format (0x01 = FormatJSON)
//	  byte 2: isCompressed (0x01 = zlib compressed)
//	  byte 3: flags (0x01 for requests)
//	  bytes 4-7: reserved (0x00 0x00 0x00 0x00)
//	  byte 8: length (single byte)
//	  bytes 9+: compressed header data
//
//	[Body Section - 8 bytes + data] (standard binme format)
//	  byte 0: type (0x02 = TypeBody)
//	  byte 1: format (0x01 = FormatJSON)
//	  byte 2: isCompressed (0x01 = zlib compressed)
//	  byte 3: reserved (0x00)
//	  bytes 4-7: length (big-endian uint32)
//	  bytes 8+: compressed body data
func BinmeEncode(jsonData []byte, bodyData []byte, seqNum uint16) ([]byte, error) {
	// Compress header JSON
	compressedHeader, err := zlibCompress(jsonData)
	if err != nil {
		return nil, fmt.Errorf("failed to compress header: %w", err)
	}

	// Compress body
	compressedBody, err := zlibCompress(bodyData)
	if err != nil {
		return nil, fmt.Errorf("failed to compress body: %w", err)
	}

	// Build the message
	var buf bytes.Buffer

	// Header section: 9-byte device header + compressed data
	headerSection := make([]byte, 9+len(compressedHeader))
	headerSection[0] = DeviceTypeHeader // type: header section (device uses 0x03)
	headerSection[1] = FormatJSON       // format: JSON (0x01)
	headerSection[2] = 0x01             // isCompressed: true
	headerSection[3] = 0x01             // flags (0x01 for requests)
	headerSection[4] = 0x00             // reserved
	headerSection[5] = 0x00             // reserved
	headerSection[6] = 0x00             // reserved
	headerSection[7] = 0x00             // reserved
	headerSection[8] = byte(len(compressedHeader)) // length (single byte)
	copy(headerSection[9:], compressedHeader)

	// Body section: 8-byte standard binme header + compressed data
	bodySection := make([]byte, 8+len(compressedBody))
	bodySection[0] = DeviceTypeBody // type: body section (0x02)
	bodySection[1] = FormatJSON     // format: JSON (0x01)
	bodySection[2] = 0x01           // isCompressed: true
	bodySection[3] = 0x00           // reserved
	binary.BigEndian.PutUint32(bodySection[4:8], uint32(len(compressedBody)))
	copy(bodySection[8:], compressedBody)

	// Total message length (excluding device transport header)
	totalLen := len(headerSection) + len(bodySection)

	// Write device transport header
	transportHeader := make([]byte, 4)
	binary.BigEndian.PutUint16(transportHeader[0:2], uint16(totalLen+4)) // total length including this header
	binary.BigEndian.PutUint16(transportHeader[2:4], seqNum)             // sequence number matches request ID

	buf.Write(transportHeader)
	buf.Write(headerSection)
	buf.Write(bodySection)

	return buf.Bytes(), nil
}

// BinmeDecode extracts JSON data from a device binme binary envelope with zlib decompression.
// Returns the header JSON and body data.
//
// Expected format (device-specific modified binme):
//
//	[Device Transport Header - 4 bytes]
//	  bytes 0-1: total message length (big-endian)
//	  bytes 2-3: sequence number (big-endian)
//
//	[Header Section - 9 bytes + data] (device-specific format)
//	  byte 0: type (0x03 = DeviceTypeHeader)
//	  byte 1: format
//	  byte 2: isCompressed
//	  byte 3: flags (0x00 for responses)
//	  bytes 4-7: reserved (0x00 0x00 0x00 0x00)
//	  byte 8: length (single byte)
//	  bytes 9+: header data
//
//	[Body Section - 8 bytes + data] (standard binme format)
//	  byte 0: type (0x02 = TypeBody)
//	  byte 1: format
//	  byte 2: isCompressed
//	  byte 3: reserved
//	  bytes 4-7: length (big-endian uint32)
//	  bytes 8+: body data
func BinmeDecode(data []byte) (headerJSON []byte, bodyData []byte, err error) {
	if len(data) < 4 {
		return nil, nil, fmt.Errorf("binme data too short: %d bytes", len(data))
	}

	// Skip device transport header (4 bytes)
	// totalLen := binary.BigEndian.Uint16(data[0:2])
	// seqNum := binary.BigEndian.Uint16(data[2:4])
	pos := 4

	if len(data) < pos+9 {
		return nil, nil, fmt.Errorf("binme data too short for header section")
	}

	// Parse device header section (9-byte format)
	headerType := data[pos]
	if headerType != DeviceTypeHeader {
		return nil, nil, fmt.Errorf("expected header type 0x%02x, got 0x%02x", DeviceTypeHeader, headerType)
	}
	// headerFormat := data[pos+1]
	headerIsCompressed := data[pos+2]
	// headerFlags := data[pos+3]
	// reserved := data[pos+4:pos+8]
	headerLen := int(data[pos+8]) // single-byte length

	pos += 9
	if len(data) < pos+headerLen {
		return nil, nil, fmt.Errorf("binme header data truncated")
	}

	headerData := data[pos : pos+headerLen]
	pos += headerLen

	// Decompress header if needed - check for zlib magic byte (0x78)
	// Response may have isCompressed=1 but actually send raw JSON
	// Zlib headers: 78 01 (none), 78 5e (fast), 78 9c (default), 78 da (best)
	if headerIsCompressed == 0x01 && len(headerData) >= 2 && headerData[0] == 0x78 {
		headerJSON, err = zlibDecompress(headerData)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decompress header: %w", err)
		}
	} else {
		// Raw data (not actually compressed despite flag)
		headerJSON = headerData
	}

	// Parse body section (standard binme 8-byte format)
	if len(data) < pos+8 {
		// No body section
		return headerJSON, nil, nil
	}

	bodyType := data[pos]
	if bodyType != DeviceTypeBody {
		return nil, nil, fmt.Errorf("expected body type 0x%02x, got 0x%02x", DeviceTypeBody, bodyType)
	}
	// bodyFormat := data[pos+1]
	bodyIsCompressed := data[pos+2]
	// bodyReserved := data[pos+3]
	bodyLen := int(binary.BigEndian.Uint32(data[pos+4 : pos+8]))

	pos += 8
	if len(data) < pos+bodyLen {
		return nil, nil, fmt.Errorf("binme body data truncated")
	}

	rawBodyData := data[pos : pos+bodyLen]

	// Decompress body if needed - check for zlib magic byte (0x78)
	// Zlib headers: 78 01 (none), 78 5e (fast), 78 9c (default), 78 da (best)
	if bodyIsCompressed == 0x01 && bodyLen >= 2 && rawBodyData[0] == 0x78 {
		bodyData, err = zlibDecompress(rawBodyData)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decompress body: %w", err)
		}
	} else {
		bodyData = rawBodyData
	}

	return headerJSON, bodyData, nil
}

// BinmeEncodeRawBody wraps JSON header with a raw binary body (format=FormatBinary).
// Used for XSFP write operations that send binary EEPROM data.
func BinmeEncodeRawBody(jsonData []byte, bodyData []byte, seqNum uint16) ([]byte, error) {
	// Compress header JSON
	compressedHeader, err := zlibCompress(jsonData)
	if err != nil {
		return nil, fmt.Errorf("failed to compress header: %w", err)
	}

	// Build the message
	var buf bytes.Buffer

	// Header section: 9-byte device header + compressed data
	headerSection := make([]byte, 9+len(compressedHeader))
	headerSection[0] = DeviceTypeHeader // type: header section (device uses 0x03)
	headerSection[1] = FormatJSON       // format: JSON (0x01)
	headerSection[2] = 0x01             // isCompressed: true
	headerSection[3] = 0x01             // flags (0x01 for requests)
	headerSection[4] = 0x00             // reserved
	headerSection[5] = 0x00             // reserved
	headerSection[6] = 0x00             // reserved
	headerSection[7] = 0x00             // reserved
	headerSection[8] = byte(len(compressedHeader)) // length (single byte)
	copy(headerSection[9:], compressedHeader)

	// Body section: 8-byte standard binme header + raw binary data (NOT compressed)
	bodySection := make([]byte, 8+len(bodyData))
	bodySection[0] = DeviceTypeBody // type: body section (0x02)
	bodySection[1] = FormatBinary   // format: raw binary (0x03)
	bodySection[2] = 0x00           // isCompressed: false
	bodySection[3] = 0x00           // reserved
	binary.BigEndian.PutUint32(bodySection[4:8], uint32(len(bodyData)))
	copy(bodySection[8:], bodyData)

	// Total message length (excluding device transport header)
	totalLen := len(headerSection) + len(bodySection)

	// Write device transport header
	transportHeader := make([]byte, 4)
	binary.BigEndian.PutUint16(transportHeader[0:2], uint16(totalLen+4))
	binary.BigEndian.PutUint16(transportHeader[2:4], seqNum)

	buf.Write(transportHeader)
	buf.Write(headerSection)
	buf.Write(bodySection)

	return buf.Bytes(), nil
}
