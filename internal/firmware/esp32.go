package firmware

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// ESP32 image format constants
const (
	ESP32ImageMagic    = 0xE9
	ESP32HeaderSize    = 24 // Main image header size
	ESP32SegmentHdrSize = 8  // Segment header size (load_addr + data_len)
)

// ESP32ImageHeader represents the main header of an ESP32 app image.
type ESP32ImageHeader struct {
	Magic        uint8
	SegmentCount uint8
	SPIMode      uint8
	SPISpeed     uint8 // Combined with size in some versions
	EntryAddr    uint32
	WPPin        uint8
	SPIPinDrv    [3]uint8
	ChipID       uint16
	MinChipRev   uint8
	MinRevFull   uint16
	MaxRevFull   uint16
	Reserved     [4]uint8
	HashAppended uint8
}

// ESP32Segment represents a memory segment in the image.
type ESP32Segment struct {
	LoadAddr   uint32
	DataLen    uint32
	FileOffset int64 // Where segment data starts in the file
	Data       []byte
}

// ESP32Image represents a parsed ESP32 app image.
type ESP32Image struct {
	Header   ESP32ImageHeader
	Segments []ESP32Segment
}

// ParseESP32Image parses an ESP32 app image from a file.
func ParseESP32Image(path string) (*ESP32Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	return ParseESP32ImageReader(f)
}

// ParseESP32ImageReader parses an ESP32 app image from a reader.
func ParseESP32ImageReader(r io.ReadSeeker) (*ESP32Image, error) {
	img := &ESP32Image{}

	// Read main header
	if err := binary.Read(r, binary.LittleEndian, &img.Header); err != nil {
		return nil, fmt.Errorf("failed to read image header: %w", err)
	}

	if img.Header.Magic != ESP32ImageMagic {
		return nil, fmt.Errorf("invalid ESP32 image magic: 0x%02x (expected 0x%02x)",
			img.Header.Magic, ESP32ImageMagic)
	}

	// Read segments
	for i := 0; i < int(img.Header.SegmentCount); i++ {
		var seg ESP32Segment

		// Read segment header
		if err := binary.Read(r, binary.LittleEndian, &seg.LoadAddr); err != nil {
			return nil, fmt.Errorf("failed to read segment %d load addr: %w", i, err)
		}
		if err := binary.Read(r, binary.LittleEndian, &seg.DataLen); err != nil {
			return nil, fmt.Errorf("failed to read segment %d data len: %w", i, err)
		}

		// Record file offset where data starts
		pos, err := r.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, fmt.Errorf("failed to get position: %w", err)
		}
		seg.FileOffset = pos

		// Read segment data
		seg.Data = make([]byte, seg.DataLen)
		if _, err := io.ReadFull(r, seg.Data); err != nil {
			return nil, fmt.Errorf("failed to read segment %d data: %w", i, err)
		}

		img.Segments = append(img.Segments, seg)
	}

	return img, nil
}

// GetDROMSegment returns the DROM segment (typically segment 0).
// DROM segments have load addresses starting with 0x3c (ESP32-S3).
func (img *ESP32Image) GetDROMSegment() *ESP32Segment {
	for i := range img.Segments {
		// DROM is typically the first segment and loads at 0x3c0xxxxx
		if (img.Segments[i].LoadAddr & 0xFF000000) == 0x3C000000 {
			return &img.Segments[i]
		}
	}
	return nil
}

// GetIROMSegment returns the IROM segment (code segment).
// IROM segments have load addresses starting with 0x42 (ESP32-S3).
func (img *ESP32Image) GetIROMSegment() *ESP32Segment {
	for i := range img.Segments {
		if (img.Segments[i].LoadAddr & 0xFF000000) == 0x42000000 {
			return &img.Segments[i]
		}
	}
	return nil
}

// FileOffsetToVAddr converts a file offset within a segment to a virtual address.
func (seg *ESP32Segment) FileOffsetToVAddr(fileOffset int64) uint32 {
	relOffset := fileOffset - seg.FileOffset
	return seg.LoadAddr + uint32(relOffset)
}

// VAddrToDataOffset converts a virtual address to an offset within the segment data.
func (seg *ESP32Segment) VAddrToDataOffset(vaddr uint32) (int64, bool) {
	if vaddr < seg.LoadAddr || vaddr >= seg.LoadAddr+seg.DataLen {
		return 0, false
	}
	return int64(vaddr - seg.LoadAddr), true
}

// FindBytes searches for a byte pattern in the segment data and returns all offsets.
func (seg *ESP32Segment) FindBytes(pattern []byte) []int64 {
	var results []int64
	offset := 0
	for {
		idx := bytes.Index(seg.Data[offset:], pattern)
		if idx == -1 {
			break
		}
		results = append(results, int64(offset+idx))
		offset = offset + idx + 1
	}
	return results
}

// ReadStringAt reads a null-terminated string at the given data offset.
func (seg *ESP32Segment) ReadStringAt(offset int64) string {
	if offset < 0 || offset >= int64(len(seg.Data)) {
		return ""
	}
	end := bytes.IndexByte(seg.Data[offset:], 0)
	if end == -1 {
		return ""
	}
	return string(seg.Data[offset : offset+int64(end)])
}

// ReadUint32At reads a little-endian uint32 at the given data offset.
func (seg *ESP32Segment) ReadUint32At(offset int64) (uint32, bool) {
	if offset < 0 || offset+4 > int64(len(seg.Data)) {
		return 0, false
	}
	return binary.LittleEndian.Uint32(seg.Data[offset:]), true
}

// ReadByteAt reads a byte at the given data offset.
func (seg *ESP32Segment) ReadByteAt(offset int64) (byte, bool) {
	if offset < 0 || offset >= int64(len(seg.Data)) {
		return 0, false
	}
	return seg.Data[offset], true
}
