package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// ContentHash computes a content-addressable hash for module EEPROM data.
// The hash only covers the identity bytes, excluding volatile diagnostic data.
//
// For SFP (SFF-8472): bytes 0-95 of page A0h (base ID fields)
// For QSFP (SFF-8636): bytes 128-219 of upper memory (ID fields)
//
// This ensures modules with identical identity but different real-time
// measurements (temperature, power, etc.) are recognized as the same profile.
func ContentHash(data []byte) (string, error) {
	if len(data) < 96 {
		return "", fmt.Errorf("data too short: need at least 96 bytes, got %d", len(data))
	}

	// Determine module type from identifier byte
	identifier := data[0]

	var hashData []byte
	switch identifier {
	case 0x03: // SFP/SFP+
		// Hash bytes 0-95 (A0h page identity fields)
		hashData = data[0:96]
	case 0x0c, 0x0d, 0x11: // QSFP, QSFP+, QSFP28
		// For QSFP, the identity data starts at byte 128
		if len(data) < 220 {
			// Fall back to first 96 bytes if we don't have full QSFP data
			hashData = data[0:96]
		} else {
			// Hash bytes 128-219 (upper memory identity fields)
			hashData = data[128:220]
		}
	default:
		// Unknown type, hash first 96 bytes
		hashData = data[0:96]
	}

	hash := sha256.Sum256(hashData)
	return "sha256:" + hex.EncodeToString(hash[:]), nil
}

// ShortHash returns a shortened version of the hash for display purposes.
func ShortHash(fullHash string) string {
	// Remove "sha256:" prefix and take first 12 chars
	if len(fullHash) > 19 {
		return fullHash[7:19] // Skip "sha256:" prefix
	}
	return fullHash
}
