package protocol

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
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

// BinmeEncode wraps JSON data in the binme binary envelope format with zlib compression.
// Format:
//
//	[Outer Header - 4 bytes]
//	  bytes 0-1: total message length (big-endian)
//	  bytes 2-3: flags (00 03 for requests)
//	[Header Section - 9 bytes + zlib data]
//	  byte 0: marker (0x03 = header section)
//	  byte 1: format (0x01 = JSON)
//	  byte 2: compression (0x01 = zlib)
//	  byte 3: flags (0x01)
//	  bytes 4-7: decompressed length (big-endian)
//	  byte 8: compressed length (for short messages)
//	  bytes 9+: zlib compressed JSON
//	[Body Section - 8 bytes + zlib data]
//	  byte 0: marker (0x02 = body section)
//	  byte 1: format (0x01 = JSON)
//	  byte 2: compression (0x01 = zlib)
//	  byte 3: reserved (0x00)
//	  bytes 4-7: compressed length (big-endian)
//	  bytes 8+: zlib compressed body
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

	// Header section: 9 bytes header + compressed data
	headerSection := make([]byte, 9+len(compressedHeader))
	headerSection[0] = 0x03 // marker: header section
	headerSection[1] = 0x01 // format: JSON
	headerSection[2] = 0x01 // compression: zlib
	headerSection[3] = 0x01 // flags
	// bytes 4-7: always 00 00 00 00 in captured traffic
	headerSection[4] = 0x00
	headerSection[5] = 0x00
	headerSection[6] = 0x00
	headerSection[7] = 0x00
	// Compressed length (single byte)
	headerSection[8] = byte(len(compressedHeader))
	copy(headerSection[9:], compressedHeader)

	// Body section: 8 bytes header + compressed data
	bodySection := make([]byte, 8+len(compressedBody))
	bodySection[0] = 0x02 // marker: body section
	bodySection[1] = 0x01 // format: JSON
	bodySection[2] = 0x01 // compression: zlib
	bodySection[3] = 0x00 // reserved
	// Compressed length (big-endian)
	binary.BigEndian.PutUint32(bodySection[4:8], uint32(len(compressedBody)))
	copy(bodySection[8:], compressedBody)

	// Total message length (excluding outer header)
	totalLen := len(headerSection) + len(bodySection)

	// Write outer header
	outerHeader := make([]byte, 4)
	binary.BigEndian.PutUint16(outerHeader[0:2], uint16(totalLen+4)) // total including header
	binary.BigEndian.PutUint16(outerHeader[2:4], seqNum)             // sequence number matches request ID

	buf.Write(outerHeader)
	buf.Write(headerSection)
	buf.Write(bodySection)

	return buf.Bytes(), nil
}

// BinmeDecode extracts JSON data from a binme binary envelope with zlib decompression.
// Returns the header JSON and body data.
func BinmeDecode(data []byte) (headerJSON []byte, bodyData []byte, err error) {
	if len(data) < 4 {
		return nil, nil, fmt.Errorf("binme data too short: %d bytes", len(data))
	}

	// Skip outer header (4 bytes)
	// totalLen := binary.BigEndian.Uint16(data[0:2])
	// flags := binary.BigEndian.Uint16(data[2:4])
	pos := 4

	if len(data) < pos+9 {
		return nil, nil, fmt.Errorf("binme data too short for header section")
	}

	// Parse header section
	headerMarker := data[pos]
	if headerMarker != 0x03 {
		return nil, nil, fmt.Errorf("expected header marker 0x03, got 0x%02x", headerMarker)
	}
	// headerFormat := data[pos+1]
	headerCompressed := data[pos+2]
	// headerFlags := data[pos+3]
	// decompressedLen := binary.BigEndian.Uint32(data[pos+4 : pos+8])
	compressedHeaderLen := int(data[pos+8])

	pos += 9
	if len(data) < pos+compressedHeaderLen {
		return nil, nil, fmt.Errorf("binme header data truncated")
	}

	compressedHeader := data[pos : pos+compressedHeaderLen]
	pos += compressedHeaderLen

	// Decompress header if needed - check for zlib magic byte (0x78)
	// Response may have compression=01 but actually send raw JSON
	// Zlib headers: 78 01 (none), 78 5e (fast), 78 9c (default), 78 da (best)
	if headerCompressed == 0x01 && len(compressedHeader) >= 2 && compressedHeader[0] == 0x78 {
		headerJSON, err = zlibDecompress(compressedHeader)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decompress header: %w", err)
		}
	} else {
		// Raw data (not actually compressed despite flag)
		headerJSON = compressedHeader
	}

	// Parse body section if present
	if len(data) < pos+8 {
		// No body section
		return headerJSON, nil, nil
	}

	bodyMarker := data[pos]
	if bodyMarker != 0x02 {
		return nil, nil, fmt.Errorf("expected body marker 0x02, got 0x%02x", bodyMarker)
	}
	// bodyFormat := data[pos+1]
	bodyCompressed := data[pos+2]
	// bodyReserved := data[pos+3]
	compressedBodyLen := int(binary.BigEndian.Uint32(data[pos+4 : pos+8]))

	pos += 8
	if len(data) < pos+compressedBodyLen {
		return nil, nil, fmt.Errorf("binme body data truncated")
	}

	compressedBody := data[pos : pos+compressedBodyLen]

	// Decompress body if needed - check for zlib magic byte (0x78)
	// Zlib headers: 78 01 (none), 78 5e (fast), 78 9c (default), 78 da (best)
	if bodyCompressed == 0x01 && compressedBodyLen >= 2 && compressedBody[0] == 0x78 {
		bodyData, err = zlibDecompress(compressedBody)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decompress body: %w", err)
		}
	} else {
		bodyData = compressedBody
	}

	return headerJSON, bodyData, nil
}

// BinmeEncodeRawBody wraps JSON header with a raw binary body (format=0x03).
// Used for XSFP write operations that send binary EEPROM data.
func BinmeEncodeRawBody(jsonData []byte, bodyData []byte, seqNum uint16) ([]byte, error) {
	// Compress header JSON
	compressedHeader, err := zlibCompress(jsonData)
	if err != nil {
		return nil, fmt.Errorf("failed to compress header: %w", err)
	}

	// Build the message
	var buf bytes.Buffer

	// Header section: 9 bytes header + compressed data
	headerSection := make([]byte, 9+len(compressedHeader))
	headerSection[0] = 0x03 // marker: header section
	headerSection[1] = 0x01 // format: JSON
	headerSection[2] = 0x01 // compression: zlib
	headerSection[3] = 0x01 // flags
	headerSection[4] = 0x00
	headerSection[5] = 0x00
	headerSection[6] = 0x00
	headerSection[7] = 0x00
	headerSection[8] = byte(len(compressedHeader))
	copy(headerSection[9:], compressedHeader)

	// Body section: 8 bytes header + raw binary data (NOT compressed)
	bodySection := make([]byte, 8+len(bodyData))
	bodySection[0] = 0x02 // marker: body section
	bodySection[1] = 0x03 // format: raw binary (0x03)
	bodySection[2] = 0x00 // compression: none
	bodySection[3] = 0x00 // reserved
	binary.BigEndian.PutUint32(bodySection[4:8], uint32(len(bodyData)))
	copy(bodySection[8:], bodyData)

	// Total message length (excluding outer header)
	totalLen := len(headerSection) + len(bodySection)

	// Write outer header
	outerHeader := make([]byte, 4)
	binary.BigEndian.PutUint16(outerHeader[0:2], uint16(totalLen+4))
	binary.BigEndian.PutUint16(outerHeader[2:4], seqNum)

	buf.Write(outerHeader)
	buf.Write(headerSection)
	buf.Write(bodySection)

	return buf.Bytes(), nil
}
