package gbfs_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamnd/gbfs-cli/gbfs"
)

// newTestClient returns a client pointed at the given base URL map with no
// pacing, for fast httptest-based tests.
func newTestClient(systemURLs map[string]string) *gbfs.Client {
	cfg := gbfs.DefaultConfig()
	cfg.SystemURLs = systemURLs
	cfg.Rate = 0
	cfg.Timeout = 5 * time.Second
	return gbfs.NewClient(cfg)
}

func TestListStations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/station_information.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"data": {
					"stations": [
						{"station_id": "s1", "name": "Main St", "lat": 40.7, "lon": -74.0},
						{"station_id": "s2", "name": "Park Ave", "lat": 40.8, "lon": -73.9}
					]
				}
			}`))
		case "/station_status.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"data": {
					"stations": [
						{"station_id": "s1", "num_bikes_available": 5, "num_ebikes_available": 2,
						 "num_docks_available": 10, "is_renting": 1, "last_reported": 1700000000},
						{"station_id": "s2", "num_bikes_available": 0, "num_ebikes_available": 0,
						 "num_docks_available": 15, "is_renting": 0, "last_reported": 1700000001}
					]
				}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(map[string]string{"test": srv.URL})
	stations, err := c.ListStations(context.Background(), "test", 0)
	if err != nil {
		t.Fatalf("ListStations: %v", err)
	}
	if len(stations) != 2 {
		t.Fatalf("got %d stations, want 2", len(stations))
	}

	s1 := stations[0]
	if s1.ID != "s1" {
		t.Errorf("ID = %q, want s1", s1.ID)
	}
	if s1.Name != "Main St" {
		t.Errorf("Name = %q, want Main St", s1.Name)
	}
	if s1.BikesAvailable != 5 {
		t.Errorf("BikesAvailable = %d, want 5", s1.BikesAvailable)
	}
	if s1.EBikesAvailable != 2 {
		t.Errorf("EBikesAvailable = %d, want 2", s1.EBikesAvailable)
	}
	if s1.DocksAvailable != 10 {
		t.Errorf("DocksAvailable = %d, want 10", s1.DocksAvailable)
	}
	if !s1.IsRenting {
		t.Errorf("IsRenting = false, want true")
	}
	if s1.LastReported != 1700000000 {
		t.Errorf("LastReported = %d, want 1700000000", s1.LastReported)
	}
	if s1.Lat != 40.7 {
		t.Errorf("Lat = %f, want 40.7", s1.Lat)
	}
	if s1.Lon != -74.0 {
		t.Errorf("Lon = %f, want -74.0", s1.Lon)
	}

	s2 := stations[1]
	if s2.IsRenting {
		t.Errorf("s2 IsRenting = true, want false")
	}
}

func TestListStationsMinBikes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/station_information.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"data": {
					"stations": [
						{"station_id": "a", "name": "Station A", "lat": 1.0, "lon": 2.0},
						{"station_id": "b", "name": "Station B", "lat": 3.0, "lon": 4.0},
						{"station_id": "c", "name": "Station C", "lat": 5.0, "lon": 6.0}
					]
				}
			}`))
		case "/station_status.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"data": {
					"stations": [
						{"station_id": "a", "num_bikes_available": 1, "is_renting": 1},
						{"station_id": "b", "num_bikes_available": 3, "is_renting": 1},
						{"station_id": "c", "num_bikes_available": 5, "is_renting": 1}
					]
				}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(map[string]string{"test": srv.URL})

	// minBikes=3 should return b and c
	stations, err := c.ListStations(context.Background(), "test", 3)
	if err != nil {
		t.Fatalf("ListStations: %v", err)
	}
	if len(stations) != 2 {
		t.Fatalf("got %d stations with minBikes=3, want 2", len(stations))
	}
	for _, s := range stations {
		if s.BikesAvailable < 3 {
			t.Errorf("station %q has %d bikes, below min=3", s.ID, s.BikesAvailable)
		}
	}
}

func TestGetSystemInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/system_information.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"name": "Citi Bike",
				"url": "http://www.citibikenyc.com",
				"timezone": "America/New_York",
				"language": "en",
				"email": "citibike@example.com"
			}
		}`))
	}))
	defer srv.Close()

	c := newTestClient(map[string]string{"test": srv.URL})
	info, err := c.GetSystemInfo(context.Background(), "test")
	if err != nil {
		t.Fatalf("GetSystemInfo: %v", err)
	}
	if info.ID != "test" {
		t.Errorf("ID = %q, want test", info.ID)
	}
	if info.Name != "Citi Bike" {
		t.Errorf("Name = %q, want Citi Bike", info.Name)
	}
	if info.URL != "http://www.citibikenyc.com" {
		t.Errorf("URL = %q, want http://www.citibikenyc.com", info.URL)
	}
	if info.Timezone != "America/New_York" {
		t.Errorf("Timezone = %q, want America/New_York", info.Timezone)
	}
	if info.Language != "en" {
		t.Errorf("Language = %q, want en", info.Language)
	}
}

func TestRetry503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		switch r.URL.Path {
		case "/system_information.json":
			if hits < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"data": {
					"name": "Test System",
					"url": "http://example.com",
					"timezone": "UTC"
				}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := gbfs.DefaultConfig()
	cfg.SystemURLs = map[string]string{"test": srv.URL}
	cfg.Rate = 0
	cfg.Retries = 5
	cfg.Timeout = 10 * time.Second
	c := gbfs.NewClient(cfg)

	info, err := c.GetSystemInfo(context.Background(), "test")
	if err != nil {
		t.Fatalf("GetSystemInfo after retries: %v", err)
	}
	if info.Name != "Test System" {
		t.Errorf("Name = %q, want Test System", info.Name)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
}

func TestUnknownSystem(t *testing.T) {
	c := gbfs.NewClient(gbfs.DefaultConfig())
	_, err := c.ListStations(context.Background(), "unknown", 0)
	if err == nil {
		t.Error("expected error for unknown system, got nil")
	}
}
