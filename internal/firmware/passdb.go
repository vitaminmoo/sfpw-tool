package firmware

import (
	"fmt"
)

// PasswordEntry represents an entry in the SFP password database.
// Entry size varies by firmware version:
//   - 1.0.5: 16 bytes (no cable_length)
//   - 1.0.10, 1.1.0: 20 bytes (with cable_length at offset 0x10)
//   - 1.1.1+: 16 bytes (no cable_length)
type PasswordEntry struct {
	ReadOnly    bool
	PartNumber  string
	Locked      bool
	Password    [4]byte
	Flags       [3]byte
	CableLength int32 // Only present in 20-byte entries (1.0.10, 1.1.0)
}

// PasswordDatabase represents the extracted password database.
type PasswordDatabase struct {
	Entries      []PasswordEntry
	DefaultEntry *PasswordEntry // Entry with NULL part_number (fallback)
	EntrySize    int            // 16 or 20 bytes
	Version      string
}

// FirstEntryMarker is the part number of the first entry in the database.
// This is used as a stable anchor point to locate the database.
const FirstEntryMarker = "AOC-SFP10-5M"

// ExtractPasswordDatabase extracts the password database from an ESP32 firmware image.
func ExtractPasswordDatabase(img *ESP32Image) (*PasswordDatabase, error) {
	drom := img.GetDROMSegment()
	if drom == nil {
		return nil, fmt.Errorf("DROM segment not found")
	}

	// Step 1: Find the marker string "AOC-SFP10-5M\0"
	markerBytes := append([]byte(FirstEntryMarker), 0)
	markerOffsets := drom.FindBytes(markerBytes)
	if len(markerOffsets) == 0 {
		return nil, fmt.Errorf("marker string %q not found in DROM", FirstEntryMarker)
	}

	// Use the first occurrence (there may be duplicates in different contexts)
	markerOffset := markerOffsets[0]
	markerVAddr := drom.LoadAddr + uint32(markerOffset)

	// Step 2: Search for a pointer to this string (as little-endian uint32)
	ptrBytes := []byte{
		byte(markerVAddr),
		byte(markerVAddr >> 8),
		byte(markerVAddr >> 16),
		byte(markerVAddr >> 24),
	}
	ptrOffsets := drom.FindBytes(ptrBytes)
	if len(ptrOffsets) == 0 {
		return nil, fmt.Errorf("pointer to marker string not found")
	}

	// The first pointer occurrence is likely in the database
	// The pointer is at offset +4 in the entry (after read_only field)
	dbStartOffset := ptrOffsets[0] - 4
	if dbStartOffset < 0 {
		return nil, fmt.Errorf("invalid database offset")
	}

	// Step 3: Determine entry size (16 or 20 bytes)
	// Check if there's a valid pointer at offset 16 or 20 from the first entry
	entrySize := detectEntrySize(drom, dbStartOffset)
	if entrySize == 0 {
		return nil, fmt.Errorf("could not determine entry size")
	}

	// Step 4: Parse all entries
	db := &PasswordDatabase{
		EntrySize: entrySize,
	}

	offset := dbStartOffset
	for {
		entry, err := parseEntry(drom, offset, entrySize)
		if err != nil {
			break
		}
		if entry.PartNumber == "" {
			// Null part_number indicates end of database - this is the default entry
			if !entry.ReadOnly {
				db.DefaultEntry = entry
			}
			break
		}
		db.Entries = append(db.Entries, *entry)
		offset += int64(entrySize)
	}

	if len(db.Entries) == 0 {
		return nil, fmt.Errorf("no entries found in database")
	}

	// Determine version based on entry size
	// Entry size pattern: 1.0.5=16, 1.0.10=20, 1.1.0=20, 1.1.1+=16
	if entrySize == 20 {
		db.Version = "1.0.10-1.1.0 (20-byte entries with cable_length)"
	} else {
		db.Version = "1.0.5 or 1.1.1+ (16-byte entries)"
	}

	return db, nil
}

// detectEntrySize determines whether entries are 16 or 20 bytes.
func detectEntrySize(seg *ESP32Segment, firstEntryOffset int64) int {
	// Count valid entries with each size - the correct size will produce more entries
	entries16 := countValidEntries(seg, firstEntryOffset, 16)
	entries20 := countValidEntries(seg, firstEntryOffset, 20)

	// The correct entry size should produce significantly more valid entries
	// For databases with ~50+ entries, the wrong size will quickly hit invalid pointers
	if entries20 > entries16 {
		return 20
	}
	if entries16 > entries20 {
		return 16
	}

	// If counts are equal (unlikely), prefer checking individual entries
	if isValidNonNullEntryAt(seg, firstEntryOffset+16) {
		return 16
	}
	if isValidNonNullEntryAt(seg, firstEntryOffset+20) {
		return 20
	}

	// Default to 16 if we can't determine
	if entries16 > 0 {
		return 16
	}
	return 0
}

// isValidNonNullEntryAt checks if there's a valid entry with non-NULL part_number at the given offset.
func isValidNonNullEntryAt(seg *ESP32Segment, offset int64) bool {
	// Read part_number pointer at offset +4
	ptr, ok := seg.ReadUint32At(offset + 4)
	if !ok || ptr == 0 {
		return false
	}
	// Valid DROM pointer should be within segment range
	if ptr >= seg.LoadAddr && ptr < seg.LoadAddr+seg.DataLen {
		// Verify it points to readable string
		strOffset, ok := seg.VAddrToDataOffset(ptr)
		if ok {
			str := seg.ReadStringAt(strOffset)
			return len(str) > 0 && len(str) < 64 // Reasonable part number length
		}
	}
	return false
}

// countValidEntries counts how many valid entries exist with given entry size.
func countValidEntries(seg *ESP32Segment, startOffset int64, entrySize int) int {
	count := 0
	offset := startOffset
	for range 200 { // Safety limit
		ptr, ok := seg.ReadUint32At(offset + 4)
		if !ok {
			break
		}
		if ptr == 0 {
			break // End marker
		}
		if ptr < seg.LoadAddr || ptr >= seg.LoadAddr+seg.DataLen {
			break // Invalid pointer
		}
		count++
		offset += int64(entrySize)
	}
	return count
}

// parseEntry parses a single password database entry.
func parseEntry(seg *ESP32Segment, offset int64, entrySize int) (*PasswordEntry, error) {
	entry := &PasswordEntry{}

	// Read read_only field (4 bytes)
	readOnly, ok := seg.ReadUint32At(offset)
	if !ok {
		return nil, fmt.Errorf("failed to read read_only at offset %d", offset)
	}
	entry.ReadOnly = readOnly != 0

	// Read part_number pointer (4 bytes)
	partNumPtr, ok := seg.ReadUint32At(offset + 4)
	if !ok {
		return nil, fmt.Errorf("failed to read part_number ptr at offset %d", offset)
	}
	if partNumPtr == 0 {
		// End of database
		entry.PartNumber = ""
		return entry, nil
	}

	// Resolve part_number string
	strOffset, ok := seg.VAddrToDataOffset(partNumPtr)
	if !ok {
		return nil, fmt.Errorf("part_number pointer 0x%08x out of range", partNumPtr)
	}
	entry.PartNumber = seg.ReadStringAt(strOffset)

	// Read locked (1 byte at offset +8)
	locked, ok := seg.ReadByteAt(offset + 8)
	if !ok {
		return nil, fmt.Errorf("failed to read locked at offset %d", offset)
	}
	entry.Locked = locked != 0

	// Read password (4 bytes at offset +9)
	for i := range 4 {
		b, ok := seg.ReadByteAt(offset + 9 + int64(i))
		if !ok {
			return nil, fmt.Errorf("failed to read password byte %d", i)
		}
		entry.Password[i] = b
	}

	// Read flags (3 bytes at offset +13)
	for i := range 3 {
		b, ok := seg.ReadByteAt(offset + 13 + int64(i))
		if !ok {
			return nil, fmt.Errorf("failed to read flags byte %d", i)
		}
		entry.Flags[i] = b
	}

	// Read cable_length if 20-byte entry (4 bytes at offset +16)
	if entrySize == 20 {
		cableLen, ok := seg.ReadUint32At(offset + 16)
		if !ok {
			return nil, fmt.Errorf("failed to read cable_length at offset %d", offset)
		}
		entry.CableLength = int32(cableLen)
	}

	return entry, nil
}

// FormatPassword returns the password as a hex string.
func (e *PasswordEntry) FormatPassword() string {
	return fmt.Sprintf("%02x %02x %02x %02x", e.Password[0], e.Password[1], e.Password[2], e.Password[3])
}

// FormatPasswordASCII returns the password as ASCII if printable, otherwise empty.
func (e *PasswordEntry) FormatPasswordASCII() string {
	allPrintable := true
	for _, b := range e.Password {
		if b < 0x20 || b > 0x7e {
			allPrintable = false
			break
		}
	}
	if allPrintable {
		return string(e.Password[:])
	}
	return ""
}

// IsDefault returns true if this appears to be a default/fallback entry.
func (e *PasswordEntry) IsDefault() bool {
	return e.PartNumber == "" && !e.ReadOnly
}

// UniquePasswords returns a deduplicated list of passwords from the database.
func (db *PasswordDatabase) UniquePasswords() [][4]byte {
	seen := make(map[[4]byte]bool)
	var result [][4]byte

	for _, entry := range db.Entries {
		// Skip read-only and all-FF passwords
		if entry.ReadOnly {
			continue
		}
		if entry.Password == [4]byte{0xff, 0xff, 0xff, 0xff} {
			continue
		}
		if !seen[entry.Password] {
			seen[entry.Password] = true
			result = append(result, entry.Password)
		}
	}
	return result
}

// FindByPartNumber searches for entries matching the given part number using exact match (strcmp),
// matching the firmware's lookup behavior.
func (db *PasswordDatabase) FindByPartNumber(partNum string) []PasswordEntry {
	var results []PasswordEntry
	for _, entry := range db.Entries {
		if entry.PartNumber == partNum {
			results = append(results, entry)
		}
	}
	return results
}

// GetPasswordsToTry emulates the firmware's password lookup algorithm.
// It collects all entries matching the part number, then deduplicates by password value,
// returning the passwords in the order they would be tried.
func (db *PasswordDatabase) GetPasswordsToTry(partNum string) []PasswordEntry {
	matches := db.FindByPartNumber(partNum)
	if len(matches) == 0 {
		return nil
	}

	// Deduplicate by password value (like firmware's sfp_collect_unique_passwords)
	seen := make(map[[4]byte]bool)
	var result []PasswordEntry
	for _, entry := range matches {
		if entry.ReadOnly {
			continue
		}
		if !seen[entry.Password] {
			seen[entry.Password] = true
			result = append(result, entry)
		}
	}
	return result
}

// FormatFlags returns a human-readable representation of the flags.
func (e *PasswordEntry) FormatFlags() string {
	// Check if all zeros
	if e.Flags[0] == 0 && e.Flags[1] == 0 && e.Flags[2] == 0 {
		return ""
	}
	return fmt.Sprintf("%02x%02x%02x", e.Flags[0], e.Flags[1], e.Flags[2])
}

// InterpretFlags returns a human-readable interpretation of flags[0] page bits.
// flags[0] is a bitmask indicating which EEPROM pages can be written after unlock:
//   - Bit 0: Lower page / A0h (basic identity)
//   - Bit 1: A2h (SFP diagnostic) / Upper page 1 (QSFP)
//   - Bit 2: Upper page 2 (QSFP)
//   - Bit 3: Upper page 3 - thresholds (QSFP)
func (e *PasswordEntry) InterpretFlags() string {
	if e.Flags[0] == 0 {
		return "none"
	}

	var pages []string
	if e.Flags[0]&0x01 != 0 {
		pages = append(pages, "A0h/lower")
	}
	if e.Flags[0]&0x02 != 0 {
		pages = append(pages, "A2h/upper1")
	}
	if e.Flags[0]&0x04 != 0 {
		pages = append(pages, "upper2")
	}
	if e.Flags[0]&0x08 != 0 {
		pages = append(pages, "upper3/thresholds")
	}

	if len(pages) == 0 {
		return fmt.Sprintf("0x%02x", e.Flags[0])
	}

	result := ""
	for i, p := range pages {
		if i > 0 {
			result += "+"
		}
		result += p
	}
	return result
}
