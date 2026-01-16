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

// Cache manages downloaded firmware files.
type Cache struct {
	baseDir string
}

// CacheEntry represents a cached firmware file.
type CacheEntry struct {
	Path       string
	Version    string
	FileSize   int64
	Downloaded time.Time
}

// DefaultCachePath returns the default cache directory.
func DefaultCachePath() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		// Fallback to home directory
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		cacheDir = filepath.Join(home, ".cache")
	}
	return filepath.Join(cacheDir, "sfpw", "firmware"), nil
}

// NewCache creates a cache at the default location.
func NewCache() (*Cache, error) {
	path, err := DefaultCachePath()
	if err != nil {
		return nil, err
	}
	return NewCacheAt(path)
}

// NewCacheAt creates a cache at the specified path.
func NewCacheAt(path string) (*Cache, error) {
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}
	return &Cache{baseDir: path}, nil
}

// Path returns the cache directory path.
func (c *Cache) Path() string {
	return c.baseDir
}

// GetPath returns the cache path for a firmware version.
func (c *Cache) GetPath(version string) string {
	// Sanitize version string for filename
	safeVersion := strings.ReplaceAll(version, "/", "_")
	return filepath.Join(c.baseDir, fmt.Sprintf("sfpw_%s.bin", safeVersion))
}

// Has checks if a firmware version is cached and valid.
func (c *Cache) Has(version, expectedSHA256 string) bool {
	path := c.GetPath(version)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}

	// Verify checksum if provided
	if expectedSHA256 != "" {
		actualSHA256, err := c.computeSHA256(path)
		if err != nil || actualSHA256 != expectedSHA256 {
			// Invalid cache entry, remove it
			os.Remove(path)
			return false
		}
	}

	return true
}

// Get returns the path to a cached firmware file.
// Returns empty string if not cached.
func (c *Cache) Get(version, expectedSHA256 string) string {
	if c.Has(version, expectedSHA256) {
		return c.GetPath(version)
	}
	return ""
}

// Download fetches firmware and stores in cache with verification.
func (c *Cache) Download(v FirmwareVersion, progress ProgressCallback) (string, error) {
	// Check if already cached
	if path := c.Get(v.Version, v.SHA256); path != "" {
		if progress != nil {
			progress(v.FileSize, v.FileSize, "Using cached firmware")
		}
		return path, nil
	}

	if v.DownloadURL == "" {
		return "", fmt.Errorf("no download URL available for version %s", v.Version)
	}

	destPath := c.GetPath(v.Version)
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

func (c *Cache) computeSHA256(path string) (string, error) {
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

// List returns all cached firmware entries.
func (c *Cache) List() ([]CacheEntry, error) {
	entries, err := os.ReadDir(c.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var result []CacheEntry
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

		result = append(result, CacheEntry{
			Path:       filepath.Join(c.baseDir, name),
			Version:    version,
			FileSize:   info.Size(),
			Downloaded: info.ModTime(),
		})
	}

	return result, nil
}

// Clear removes all cached firmware files.
func (c *Cache) Clear() error {
	entries, err := c.List()
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.Remove(e.Path); err != nil {
			return err
		}
	}
	return nil
}

// Remove removes a specific cached version.
func (c *Cache) Remove(version string) error {
	path := c.GetPath(version)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // Already gone
	}
	return os.Remove(path)
}

// ImportFile copies a local file into the cache.
// Returns the cache path and computed SHA256 checksum.
func (c *Cache) ImportFile(srcPath string) (cachePath string, sha256sum string, size int64, err error) {
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

	destPath := c.GetPath(version)
	tmpPath := destPath + ".tmp"

	dest, err := os.Create(tmpPath)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to create cache file: %w", err)
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
