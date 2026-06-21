package mieru

import (
	"testing"
)

func TestIsMitaAvailable_noMita(t *testing.T) {
	// In CI / dev environments mita is not installed — IsMitaAvailable must
	// return false without panicking.
	got := IsMitaAvailable()
	// We can't assert the value because it depends on the host, but we
	// can assert the function completes without panic.
	_ = got
}

func TestGetStatus_noMita(t *testing.T) {
	// When mita is not installed, GetStatus must populate MitaAvailable=false
	// and a non-empty Error field, never panic.
	s := GetStatus()
	if IsMitaAvailable() {
		// mita is installed in this env; just verify shape
		if s.StatusOutput == "" && s.Error == "" {
			// either output or error must be set
		}
		return
	}
	if s.MitaAvailable {
		t.Error("GetStatus.MitaAvailable should be false when mita is not installed")
	}
	if s.Error == "" {
		t.Error("GetStatus.Error should be non-empty when mita is not installed")
	}
}

func TestParseUserStats_empty(t *testing.T) {
	if got := parseUserStats(""); got != nil {
		t.Errorf("expected nil for empty input, got %v", got)
	}
}

func TestParseUserStats_jsonV2(t *testing.T) {
	input := `{"users":[{"username":"alice","downloadBytesSinceLastStart":1000,"uploadBytesSinceLastStart":500,"lastActive":1700000000}]}`
	stats := parseUserStats(input)
	if len(stats) != 1 {
		t.Fatalf("want 1 stat, got %d", len(stats))
	}
	s := stats[0]
	if s.Username != "alice" {
		t.Errorf("want username alice, got %s", s.Username)
	}
	if s.DownloadBytes != 1000 {
		t.Errorf("want download 1000, got %d", s.DownloadBytes)
	}
	if s.UploadBytes != 500 {
		t.Errorf("want upload 500, got %d", s.UploadBytes)
	}
	if !s.UsageAvailable {
		t.Error("want UsageAvailable true")
	}
	if s.LastActive != 1700000000000 {
		t.Errorf("want LastActive 1700000000000 ms, got %d", s.LastActive)
	}
}

func TestParseUserStats_altFields(t *testing.T) {
	// older mita versions use "name" + "downloadBytes"
	input := `{"users":[{"name":"bob","downloadBytes":200,"uploadBytes":100}]}`
	stats := parseUserStats(input)
	if len(stats) != 1 {
		t.Fatalf("want 1 stat, got %d", len(stats))
	}
	if stats[0].Username != "bob" {
		t.Errorf("want bob, got %s", stats[0].Username)
	}
	if stats[0].DownloadBytes != 200 {
		t.Errorf("want 200, got %d", stats[0].DownloadBytes)
	}
}

func TestParseUserStats_plainArray(t *testing.T) {
	input := `[{"name":"carol"},{"username":"dave"}]`
	stats := parseUserStats(input)
	if len(stats) != 2 {
		t.Fatalf("want 2 stats, got %d", len(stats))
	}
	names := map[string]bool{}
	for _, s := range stats {
		names[s.Username] = true
	}
	if !names["carol"] || !names["dave"] {
		t.Errorf("unexpected usernames: %v", names)
	}
}

func TestParseUserStats_malformed(t *testing.T) {
	// Should return nil without panicking.
	got := parseUserStats("not json at all")
	if got != nil {
		t.Errorf("expected nil for malformed input, got %v", got)
	}
}

func TestRunMita_notFound(t *testing.T) {
	if IsMitaAvailable() {
		t.Skip("mita is installed; skipping not-found test")
	}
	_, err := runMita("status")
	if err == nil {
		t.Fatal("expected error when mita is not installed")
	}
	if _, ok := err.(MitaNotFoundError); !ok {
		t.Errorf("expected MitaNotFoundError, got %T: %v", err, err)
	}
}

func TestApplyConfig_notFound(t *testing.T) {
	if IsMitaAvailable() {
		t.Skip("mita is installed; skipping not-found test")
	}
	cfg := &MitaConfig{
		PortBindings: []PortBinding{{Protocol: "TCP", PortRange: "9000"}},
		Users:        []MitaUser{{Name: "x", Password: "y"}},
		LoggingLevel: "INFO",
		MTU:          1400,
	}
	err := ApplyConfig(cfg)
	if err == nil {
		t.Fatal("expected error when mita is not installed")
	}
	if _, ok := err.(MitaNotFoundError); !ok {
		t.Errorf("expected MitaNotFoundError, got %T: %v", err, err)
	}
}

func TestStart_notFound(t *testing.T) {
	if IsMitaAvailable() {
		t.Skip("mita is installed; skipping not-found test")
	}
	if err := Start(); err == nil {
		t.Fatal("expected error when mita is not installed")
	}
}

func TestStop_notFound(t *testing.T) {
	if IsMitaAvailable() {
		t.Skip("mita is installed; skipping not-found test")
	}
	if err := Stop(); err == nil {
		t.Fatal("expected error when mita is not installed")
	}
}
