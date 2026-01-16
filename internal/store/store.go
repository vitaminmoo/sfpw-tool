package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Store manages a content-addressable collection of module EEPROM profiles.
type Store struct {
	baseDir      string
	profilesDir  string
	metadataDir  string
	indexPath    string
}

// Index contains quick lookup information for all profiles.
type Index struct {
	Profiles map[string]IndexEntry `json:"profiles"` // hash -> entry
	UpdatedAt time.Time            `json:"updated_at"`
}

// IndexEntry contains summary info for quick listing.
type IndexEntry struct {
	VendorName   string    `json:"vendor_name"`
	PartNumber   string    `json:"part_number"`
	SerialNumber string    `json:"serial_number"`
	ModuleType   string    `json:"module_type"`
	WavelengthNM int       `json:"wavelength_nm,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// DefaultPath returns the default store path (~/.sfpw/store).
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sfpw", "store"), nil
}

// Open opens or creates a store at the given path.
func Open(path string) (*Store, error) {
	s := &Store{
		baseDir:     path,
		profilesDir: filepath.Join(path, "profiles"),
		metadataDir: filepath.Join(path, "metadata"),
		indexPath:   filepath.Join(path, "index.json"),
	}

	// Create directories
	if err := os.MkdirAll(s.profilesDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create profiles dir: %w", err)
	}
	if err := os.MkdirAll(s.metadataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create metadata dir: %w", err)
	}

	return s, nil
}

// OpenDefault opens the store at the default path.
func OpenDefault() (*Store, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return Open(path)
}

// Import adds a profile to the store.
// If the profile already exists (same hash), it updates sources.
// Returns the hash and whether it was a new profile.
func (s *Store) Import(data []byte, source Source) (string, bool, error) {
	hash, err := ContentHash(data)
	if err != nil {
		return "", false, err
	}

	profilePath := filepath.Join(s.profilesDir, hashToFilename(hash)+".bin")
	metaPath := filepath.Join(s.metadataDir, hashToFilename(hash)+".json")

	// Check if profile already exists
	isNew := false
	var meta *Metadata

	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		// New profile
		isNew = true
		meta = ExtractMetadata(data, hash)
		if meta == nil {
			meta = &Metadata{
				ContentHash: hash,
				ModuleType:  "Unknown",
				Size:        len(data),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}
		}
		meta.Sources = []Source{source}

		// Write profile data
		if err := os.WriteFile(profilePath, data, 0644); err != nil {
			return "", false, fmt.Errorf("failed to write profile: %w", err)
		}
	} else {
		// Existing profile - load and update sources
		metaData, err := os.ReadFile(metaPath)
		if err != nil {
			return "", false, fmt.Errorf("failed to read metadata: %w", err)
		}
		meta = &Metadata{}
		if err := json.Unmarshal(metaData, meta); err != nil {
			return "", false, fmt.Errorf("failed to parse metadata: %w", err)
		}
		meta.Sources = append(meta.Sources, source)
		meta.UpdatedAt = time.Now()
	}

	// Write metadata
	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return "", false, fmt.Errorf("failed to marshal metadata: %w", err)
	}
	if err := os.WriteFile(metaPath, metaJSON, 0644); err != nil {
		return "", false, fmt.Errorf("failed to write metadata: %w", err)
	}

	// Update index
	if err := s.updateIndex(hash, meta); err != nil {
		return "", false, fmt.Errorf("failed to update index: %w", err)
	}

	return hash, isNew, nil
}

// Get retrieves profile data by hash.
func (s *Store) Get(hash string) ([]byte, error) {
	profilePath := filepath.Join(s.profilesDir, hashToFilename(hash)+".bin")
	return os.ReadFile(profilePath)
}

// GetMetadata retrieves profile metadata by hash.
func (s *Store) GetMetadata(hash string) (*Metadata, error) {
	metaPath := filepath.Join(s.metadataDir, hashToFilename(hash)+".json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}
	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// List returns all profiles in the store.
func (s *Store) List() ([]IndexEntry, error) {
	index, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	entries := make([]IndexEntry, 0, len(index.Profiles))
	for hash, entry := range index.Profiles {
		entry := entry // copy
		// Store hash in a field we can access (we'll use the map key)
		entries = append(entries, entry)
		_ = hash // hash is available via map iteration if needed
	}

	// Sort by creation date (newest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CreatedAt.After(entries[j].CreatedAt)
	})

	return entries, nil
}

// ListWithHashes returns all profiles with their hashes.
func (s *Store) ListWithHashes() (map[string]IndexEntry, error) {
	index, err := s.loadIndex()
	if err != nil {
		return nil, err
	}
	return index.Profiles, nil
}

// Export writes a profile to a file.
func (s *Store) Export(hash, destPath string) error {
	data, err := s.Get(hash)
	if err != nil {
		return err
	}
	return os.WriteFile(destPath, data, 0644)
}

// Count returns the number of profiles in the store.
func (s *Store) Count() (int, error) {
	index, err := s.loadIndex()
	if err != nil {
		return 0, err
	}
	return len(index.Profiles), nil
}

func (s *Store) loadIndex() (*Index, error) {
	data, err := os.ReadFile(s.indexPath)
	if os.IsNotExist(err) {
		return &Index{Profiles: make(map[string]IndexEntry)}, nil
	}
	if err != nil {
		return nil, err
	}

	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	if index.Profiles == nil {
		index.Profiles = make(map[string]IndexEntry)
	}
	return &index, nil
}

func (s *Store) updateIndex(hash string, meta *Metadata) error {
	index, err := s.loadIndex()
	if err != nil {
		return err
	}

	index.Profiles[hash] = IndexEntry{
		VendorName:   meta.Identity.VendorName,
		PartNumber:   meta.Identity.PartNumber,
		SerialNumber: meta.Identity.SerialNumber,
		ModuleType:   meta.ModuleType,
		WavelengthNM: meta.Specs.WavelengthNM,
		CreatedAt:    meta.CreatedAt,
	}
	index.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.indexPath, data, 0644)
}

// hashToFilename converts a full hash to a safe filename.
func hashToFilename(hash string) string {
	// Remove "sha256:" prefix
	if len(hash) > 7 && hash[:7] == "sha256:" {
		return hash[7:]
	}
	return hash
}
