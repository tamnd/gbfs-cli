// Package gbfs is the library behind the gbfs command line:
// the HTTP client, request shaping, and the typed data models for GBFS
// (General Bikeshare Feed Specification) feeds.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public feed throws under load.
package gbfs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Host is the primary GBFS host (Citi Bike NYC / Lyft).
const Host = "gbfs.lyft.com"

// Config holds constructor parameters for Client.
type Config struct {
	// SystemURLs maps short system IDs to their GBFS base URL.
	SystemURLs map[string]string
	UserAgent  string
	Rate       time.Duration
	Retries    int
	Timeout    time.Duration
}

// DefaultConfig returns sensible defaults for GBFS feeds.
func DefaultConfig() Config {
	return Config{
		SystemURLs: map[string]string{
			"bkn":     "https://gbfs.lyft.com/gbfs/2.3/bkn/en",
			"divvy":   "https://gbfs.divvybikes.com/gbfs/2.3/en",
			"bay":     "https://gbfs.baywheels.com/gbfs/2.3/en",
			"capital": "https://gbfs.capitalbikeshare.com/gbfs/2.3/en",
		},
		UserAgent: "tamnd-gbfs-cli/0.1 (tamnd87@gmail.com)",
		Rate:      200 * time.Millisecond,
		Retries:   3,
		Timeout:   15 * time.Second,
	}
}

// Client is a rate-limited HTTP client for GBFS feeds.
type Client struct {
	cfg  Config
	http *http.Client
	last time.Time
}

// NewClient returns a Client configured with cfg.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// Station is one bike share station with availability data joined in.
type Station struct {
	ID              string  `json:"id" kit:"id"`
	Name            string  `json:"name"`
	Lat             float64 `json:"lat"`
	Lon             float64 `json:"lon"`
	BikesAvailable  int     `json:"bikes_available"`
	EBikesAvailable int     `json:"ebikes_available"`
	DocksAvailable  int     `json:"docks_available"`
	IsRenting       bool    `json:"is_renting"`
	LastReported    int64   `json:"last_reported"`
}

// SystemInfo describes a GBFS system.
type SystemInfo struct {
	ID       string `json:"id" kit:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Timezone string `json:"timezone"`
	Language string `json:"language"`
	Email    string `json:"email"`
}

// --- wire types ---

type wireStationInfoResponse struct {
	Data wireStationInfo `json:"data"`
}

type wireStationInfo struct {
	Stations []wireStation `json:"stations"`
}

type wireStation struct {
	ID   string  `json:"station_id"`
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

type wireStationStatusResponse struct {
	Data wireStationStatus `json:"data"`
}

type wireStationStatus struct {
	Stations []wireStationSt `json:"stations"`
}

type wireStationSt struct {
	ID              string `json:"station_id"`
	BikesAvailable  int    `json:"num_bikes_available"`
	EBikesAvailable int    `json:"num_ebikes_available"`
	DocksAvailable  int    `json:"num_docks_available"`
	IsRenting       int    `json:"is_renting"`
	LastReported    int64  `json:"last_reported"`
}

type wireSystemInfoResponse struct {
	Data wireSysInfoData `json:"data"`
}

type wireSysInfoData struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Timezone string `json:"timezone"`
	Language string `json:"language"`
	Email    string `json:"email"`
}

// --- public API ---

// ListStations fetches station_information and station_status for the given
// system, joins them by station_id, and filters to stations with at least
// minBikes bikes available. Pass minBikes=0 to return all stations.
func (c *Client) ListStations(ctx context.Context, system string, minBikes int) ([]Station, error) {
	baseURL := c.cfg.SystemURLs[system]
	if baseURL == "" {
		return nil, fmt.Errorf("unknown system %q; known: bkn, divvy, bay, capital", system)
	}

	var infoResp wireStationInfoResponse
	if err := c.getJSON(ctx, baseURL+"/station_information.json", &infoResp); err != nil {
		return nil, fmt.Errorf("station_information: %w", err)
	}

	var statusResp wireStationStatusResponse
	if err := c.getJSON(ctx, baseURL+"/station_status.json", &statusResp); err != nil {
		return nil, fmt.Errorf("station_status: %w", err)
	}

	// build info index
	infoByID := make(map[string]wireStation, len(infoResp.Data.Stations))
	for _, s := range infoResp.Data.Stations {
		infoByID[s.ID] = s
	}

	var out []Station
	for _, st := range statusResp.Data.Stations {
		if minBikes > 0 && st.BikesAvailable < minBikes {
			continue
		}
		info := infoByID[st.ID]
		out = append(out, Station{
			ID:              st.ID,
			Name:            info.Name,
			Lat:             info.Lat,
			Lon:             info.Lon,
			BikesAvailable:  st.BikesAvailable,
			EBikesAvailable: st.EBikesAvailable,
			DocksAvailable:  st.DocksAvailable,
			IsRenting:       st.IsRenting == 1,
			LastReported:    st.LastReported,
		})
	}
	return out, nil
}

// GetSystemInfo fetches the system_information feed for the given system.
func (c *Client) GetSystemInfo(ctx context.Context, system string) (*SystemInfo, error) {
	baseURL := c.cfg.SystemURLs[system]
	if baseURL == "" {
		return nil, fmt.Errorf("unknown system %q; known: bkn, divvy, bay, capital", system)
	}

	var resp wireSystemInfoResponse
	if err := c.getJSON(ctx, baseURL+"/system_information.json", &resp); err != nil {
		return nil, fmt.Errorf("system_information: %w", err)
	}

	return &SystemInfo{
		ID:       system,
		Name:     resp.Data.Name,
		URL:      resp.Data.URL,
		Timezone: resp.Data.Timezone,
		Language: resp.Data.Language,
		Email:    resp.Data.Email,
	}, nil
}

// --- HTTP helpers ---

func (c *Client) getJSON(ctx context.Context, rawURL string, v any) error {
	body, err := c.get(ctx, rawURL)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("decode %s: %w", rawURL, err)
	}
	return nil
}

func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}
