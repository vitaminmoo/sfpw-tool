package firmware

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"time"
)

const ManifestBaseURL = "https://fw-update.ubnt.com/api/firmware"

// ManifestClient fetches firmware metadata from the Ubiquiti API.
type ManifestClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewManifestClient creates a new manifest API client.
func NewManifestClient() *ManifestClient {
	return &ManifestClient{
		baseURL: ManifestBaseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetAvailable fetches available firmware versions matching the filter.
// Results are sorted by Created date, newest first.
func (c *ManifestClient) GetAvailable(filter ManifestFilter) ([]FirmwareVersion, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}

	q := u.Query()
	if filter.Channel != "" {
		q.Add("filter", fmt.Sprintf("eq~~channel~~%s", filter.Channel))
	}
	if filter.Product != "" {
		q.Add("filter", fmt.Sprintf("eq~~product~~%s", filter.Product))
	}
	if filter.Platform != "" {
		q.Add("filter", fmt.Sprintf("eq~~platform~~%s", filter.Platform))
	}
	u.RawQuery = q.Encode()

	resp, err := c.httpClient.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch firmware manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("manifest API returned %d: %s", resp.StatusCode, string(body))
	}

	var result manifestResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	versions := make([]FirmwareVersion, 0, len(result.Embedded.Firmware))
	for _, fw := range result.Embedded.Firmware {
		v := FirmwareVersion{
			ID:       fw.ID,
			Version:  fw.Version,
			Created:  fw.Created,
			Updated:  fw.Updated,
			FileSize: fw.FileSize,
			MD5:      fw.MD5,
			SHA256:   fw.SHA256Checksum,
			Channel:  fw.Channel,
			Product:  fw.Product,
			Platform: fw.Platform,
		}
		if fw.Links.Data.Href != "" {
			v.DownloadURL = fw.Links.Data.Href
		}
		versions = append(versions, v)
	}

	// Sort by created date, newest first
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Created.After(versions[j].Created)
	})

	return versions, nil
}

// GetLatest returns the most recently created firmware version.
func (c *ManifestClient) GetLatest(filter ManifestFilter) (*FirmwareVersion, error) {
	versions, err := c.GetAvailable(filter)
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("no firmware versions found")
	}
	return &versions[0], nil
}

// FindVersion finds a specific version in the available firmware.
func (c *ManifestClient) FindVersion(filter ManifestFilter, version string) (*FirmwareVersion, error) {
	versions, err := c.GetAvailable(filter)
	if err != nil {
		return nil, err
	}
	for _, v := range versions {
		if v.Version == version {
			return &v, nil
		}
	}
	return nil, fmt.Errorf("version %s not found", version)
}

// manifestResponse matches the Ubiquiti API response structure.
type manifestResponse struct {
	Embedded struct {
		Firmware []manifestFirmware `json:"firmware"`
	} `json:"_embedded"`
}

type manifestFirmware struct {
	ID             string    `json:"id"`
	Version        string    `json:"version"`
	Created        time.Time `json:"created"`
	Updated        time.Time `json:"updated"`
	FileSize       int64     `json:"file_size"`
	MD5            string    `json:"md5"`
	SHA256Checksum string    `json:"sha256_checksum"`
	Channel        string    `json:"channel"`
	Product        string    `json:"product"`
	Platform       string    `json:"platform"`
	Links          struct {
		Self struct {
			Href string `json:"href"`
		} `json:"self"`
		Data struct {
			Href string `json:"href"`
		} `json:"data"`
	} `json:"_links"`
}
