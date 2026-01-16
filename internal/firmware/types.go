package firmware

import "time"

// FirmwareVersion represents an available firmware version from the manifest API.
type FirmwareVersion struct {
	ID          string    `json:"id"`
	Version     string    `json:"version"`
	Created     time.Time `json:"created"`
	Updated     time.Time `json:"updated"`
	FileSize    int64     `json:"file_size"`
	MD5         string    `json:"md5"`
	SHA256      string    `json:"sha256_checksum"`
	DownloadURL string    `json:"-"` // Extracted from _links
	Channel     string    `json:"channel"`
	Product     string    `json:"product"`
	Platform    string    `json:"platform"`
}

// ProgressCallback is called during long operations to report progress.
// current and total are byte counts, description is a human-readable phase name.
type ProgressCallback func(current, total int64, description string)

// TransferProgress tracks a file transfer operation.
type TransferProgress struct {
	BytesSent   int64
	TotalBytes  int64
	ChunksSent  int
	TotalChunks int
	Phase       string // "downloading", "uploading", "installing"
}

// Percent returns the progress as a percentage (0.0 to 1.0).
func (p TransferProgress) Percent() float64 {
	if p.TotalBytes == 0 {
		return 0
	}
	return float64(p.BytesSent) / float64(p.TotalBytes)
}

// ManifestFilter defines filters for firmware queries.
type ManifestFilter struct {
	Product  string
	Platform string
	Channel  string
}

// DefaultSFPWizardFilter returns the filter for SFP Wizard firmware on ESP32.
func DefaultSFPWizardFilter() ManifestFilter {
	return ManifestFilter{
		Product:  "SFP-Wizard",
		Platform: "ESP32",
		Channel:  "release",
	}
}
