package firmware

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FirmwareStore manages downloaded firmware files.
type FirmwareStore struct {
	baseDir string
}

// FirmwareEntry represents a downloaded firmware file.
type FirmwareEntry struct {
	Path       string
	Version    string
	FileSize   int64
	Downloaded time.Time
}

// DefaultFirmwareStorePath returns the default firmware storage directory.
func DefaultFirmwareStorePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "sfpw-tools", "firmware"), nil
}

// NewFirmwareStore creates a firmware store at the default location.
func NewFirmwareStore() (*FirmwareStore, error) {
	path, err := DefaultFirmwareStorePath()
	if err != nil {
		return nil, err
	}
	return NewFirmwareStoreAt(path)
}

// NewFirmwareStoreAt creates a firmware store at the specified path.
func NewFirmwareStoreAt(path string) (*FirmwareStore, error) {
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create firmware directory: %w", err)
	}
	return &FirmwareStore{baseDir: path}, nil
}

// Path returns the firmware storage directory path.
func (s *FirmwareStore) Path() string {
	return s.baseDir
}

// GetPath returns the storage path for a firmware version.
func (s *FirmwareStore) GetPath(version string) string {
	// Sanitize version string for filename
	safeVersion := strings.ReplaceAll(version, "/", "_")
	return filepath.Join(s.baseDir, fmt.Sprintf("sfpw_%s.bin", safeVersion))
}

// Has checks if a firmware version is downloaded and valid.
func (s *FirmwareStore) Has(version, expectedSHA256 string) bool {
	path := s.GetPath(version)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}

	// Verify checksum if provided
	if expectedSHA256 != "" {
		actualSHA256, err := s.computeSHA256(path)
		if err != nil || actualSHA256 != expectedSHA256 {
			// Invalid entry, remove it
			os.Remove(path)
			return false
		}
	}

	return true
}

// Get returns the path to a downloaded firmware file.
// Returns empty string if not present.
func (s *FirmwareStore) Get(version, expectedSHA256 string) string {
	if s.Has(version, expectedSHA256) {
		return s.GetPath(version)
	}
	return ""
}

// Download fetches firmware and stores it with verification.
func (s *FirmwareStore) Download(v FirmwareVersion, progress ProgressCallback) (string, error) {
	// Check if already downloaded
	if path := s.Get(v.Version, v.SHA256); path != "" {
		if progress != nil {
			progress(v.FileSize, v.FileSize, "Already downloaded")
		}
		return path, nil
	}

	if v.DownloadURL == "" {
		return "", fmt.Errorf("no download URL available for version %s", v.Version)
	}

	destPath := s.GetPath(v.Version)
	tmpPath := destPath + ".tmp"

	// Download to temp file
	resp, err := http.Get(v.DownloadURL)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned %d", resp.StatusCode)
	}

	f, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}

	// Download with progress tracking and hash computation
	hasher := sha256.New()
	writer := io.MultiWriter(f, hasher)

	var downloaded int64
	totalSize := v.FileSize
	if totalSize == 0 && resp.ContentLength > 0 {
		totalSize = resp.ContentLength
	}

	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := writer.Write(buf[:n]); werr != nil {
				f.Close()
				os.Remove(tmpPath)
				return "", fmt.Errorf("write failed: %w", werr)
			}
			downloaded += int64(n)
			if progress != nil {
				progress(downloaded, totalSize, "Downloading firmware")
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			f.Close()
			os.Remove(tmpPath)
			return "", fmt.Errorf("download interrupted: %w", err)
		}
	}
	f.Close()

	// Verify checksum
	actualSHA256 := hex.EncodeToString(hasher.Sum(nil))
	if v.SHA256 != "" && actualSHA256 != v.SHA256 {
		os.Remove(tmpPath)
		return "", fmt.Errorf("checksum mismatch: expected %s, got %s", v.SHA256, actualSHA256)
	}

	// Move to final location
	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to finalize download: %w", err)
	}

	return destPath, nil
}

func (s *FirmwareStore) computeSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// List returns all downloaded firmware entries.
func (s *FirmwareStore) List() ([]FirmwareEntry, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var result []FirmwareEntry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".bin") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		// Parse version from filename: sfpw_vX.Y.Z.bin
		name := e.Name()
		version := strings.TrimPrefix(strings.TrimSuffix(name, ".bin"), "sfpw_")

		result = append(result, FirmwareEntry{
			Path:       filepath.Join(s.baseDir, name),
			Version:    version,
			FileSize:   info.Size(),
			Downloaded: info.ModTime(),
		})
	}

	return result, nil
}

// Remove removes a specific downloaded version.
func (s *FirmwareStore) Remove(version string) error {
	path := s.GetPath(version)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // Already gone
	}
	return os.Remove(path)
}

// ImportFile copies a local file into the firmware store.
// Returns the storage path and computed SHA256 checksum.
func (s *FirmwareStore) ImportFile(srcPath string) (storagePath string, sha256sum string, size int64, err error) {
	f, err := os.Open(srcPath)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to stat file: %w", err)
	}
	size = info.Size()

	// Generate a version name based on filename
	baseName := filepath.Base(srcPath)
	version := strings.TrimSuffix(baseName, filepath.Ext(baseName))

	destPath := s.GetPath(version)
	tmpPath := destPath + ".tmp"

	dest, err := os.Create(tmpPath)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to create storage file: %w", err)
	}

	hasher := sha256.New()
	writer := io.MultiWriter(dest, hasher)

	if _, err := io.Copy(writer, f); err != nil {
		dest.Close()
		os.Remove(tmpPath)
		return "", "", 0, fmt.Errorf("failed to copy file: %w", err)
	}
	dest.Close()

	sha256sum = hex.EncodeToString(hasher.Sum(nil))

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return "", "", 0, fmt.Errorf("failed to finalize import: %w", err)
	}

	return destPath, sha256sum, size, nil
}
