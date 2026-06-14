package gbfs

import (
	"testing"
)

// These tests are offline: they exercise the URI driver's pure string functions.
// The client's HTTP behaviour is covered in gbfs_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "gbfs" {
		t.Errorf("Scheme = %q, want gbfs", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "gbfs" {
		t.Errorf("Identity.Binary = %q, want gbfs", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		in      string
		wantTyp string
		wantID  string
		wantErr bool
	}{
		{"bkn", "system", "bkn", false},
		{"divvy", "system", "divvy", false},
		{"bay", "system", "bay", false},
		{"capital", "system", "capital", false},
		{"station-12345", "station", "12345", false},
		{"unknown-thing", "", "", true},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("Classify(%q): expected error, got nil (type=%q id=%q)", tc.in, typ, id)
			}
			continue
		}
		if err != nil {
			t.Errorf("Classify(%q): unexpected error: %v", tc.in, err)
			continue
		}
		if typ != tc.wantTyp || id != tc.wantID {
			t.Errorf("Classify(%q) = (%q, %q), want (%q, %q)",
				tc.in, typ, id, tc.wantTyp, tc.wantID)
		}
	}
}

func TestLocate(t *testing.T) {
	cases := []struct {
		typ     string
		id      string
		wantURL string
		wantErr bool
	}{
		{"system", "bkn", "https://www.citibikenyc.com", false},
		{"system", "divvy", "https://www.divvybikes.com", false},
		{"system", "bay", "https://www.baywheels.com", false},
		{"system", "capital", "https://www.capitalbikeshare.com", false},
		{"station", "12345", "https://" + Host + "/station/12345", false},
		{"unknown", "x", "", true},
	}
	for _, tc := range cases {
		got, err := Domain{}.Locate(tc.typ, tc.id)
		if tc.wantErr {
			if err == nil {
				t.Errorf("Locate(%q, %q): expected error, got %q", tc.typ, tc.id, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Locate(%q, %q): unexpected error: %v", tc.typ, tc.id, err)
			continue
		}
		if got != tc.wantURL {
			t.Errorf("Locate(%q, %q) = %q, want %q", tc.typ, tc.id, got, tc.wantURL)
		}
	}
}

func TestLocateUnknown(t *testing.T) {
	_, err := Domain{}.Locate("system", "nonexistent")
	if err == nil {
		t.Error("Locate system/nonexistent: expected error, got nil")
	}
}
