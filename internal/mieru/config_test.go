package mieru

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestBuildConfig_basic(t *testing.T) {
	ib := &model.MieruInbound{
		Id:           1,
		Name:         "test",
		Enable:       true,
		TCPPortRange: "34787-34790",
		UDPPortRange: "33177-33180",
		MTU:          1400,
		LoggingLevel: "INFO",
	}
	users := []model.MieruUser{
		{Id: 1, InboundId: 1, Username: "alice", Password: "secret1", Enable: true},
		{Id: 2, InboundId: 1, Username: "bob", Password: "secret2", Enable: false}, // disabled → excluded
	}

	cfg, err := BuildConfig(ib, users)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	if len(cfg.PortBindings) != 2 {
		t.Errorf("want 2 port bindings, got %d", len(cfg.PortBindings))
	}
	if len(cfg.Users) != 1 {
		t.Errorf("want 1 enabled user, got %d", len(cfg.Users))
	}
	if cfg.Users[0].Name != "alice" {
		t.Errorf("want user alice, got %s", cfg.Users[0].Name)
	}
	if cfg.MTU != 1400 {
		t.Errorf("want MTU 1400, got %d", cfg.MTU)
	}
	if cfg.LoggingLevel != "INFO" {
		t.Errorf("want LoggingLevel INFO, got %s", cfg.LoggingLevel)
	}
}

func TestBuildConfig_noPortBindings(t *testing.T) {
	ib := &model.MieruInbound{Id: 1, Name: "empty"}
	_, err := BuildConfig(ib, nil)
	if err == nil {
		t.Fatal("expected error for inbound with no port bindings")
	}
}

func TestBuildConfig_nilInbound(t *testing.T) {
	_, err := BuildConfig(nil, nil)
	if err == nil {
		t.Fatal("expected error for nil inbound")
	}
}

func TestBuildConfig_tcpOnly(t *testing.T) {
	ib := &model.MieruInbound{
		Id:           2,
		Name:         "tcp-only",
		TCPPortRange: "9000",
		MTU:          1400,
		LoggingLevel: "DEBUG",
	}
	cfg, err := BuildConfig(ib, nil)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	if len(cfg.PortBindings) != 1 {
		t.Errorf("want 1 binding, got %d", len(cfg.PortBindings))
	}
	if cfg.PortBindings[0].Protocol != "TCP" {
		t.Errorf("want TCP, got %s", cfg.PortBindings[0].Protocol)
	}
	if cfg.LoggingLevel != "DEBUG" {
		t.Errorf("want DEBUG, got %s", cfg.LoggingLevel)
	}
}

func TestBuildConfig_defaultsMTUAndLevel(t *testing.T) {
	ib := &model.MieruInbound{
		Id:           3,
		Name:         "defaults",
		UDPPortRange: "5000-5001",
		// MTU=0, LoggingLevel="" → should use defaults
	}
	cfg, err := BuildConfig(ib, nil)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	if cfg.MTU != 1400 {
		t.Errorf("want default MTU 1400, got %d", cfg.MTU)
	}
	if cfg.LoggingLevel != "INFO" {
		t.Errorf("want default LoggingLevel INFO, got %s", cfg.LoggingLevel)
	}
}

func TestBuildConfig_skipBlankCredentials(t *testing.T) {
	ib := &model.MieruInbound{
		Id:           4,
		Name:         "creds",
		TCPPortRange: "8080",
		MTU:          1400,
	}
	users := []model.MieruUser{
		{Id: 1, InboundId: 4, Username: "", Password: "pw", Enable: true},  // blank username
		{Id: 2, InboundId: 4, Username: "bob", Password: "", Enable: true}, // blank password
		{Id: 3, InboundId: 4, Username: "ok", Password: "pw", Enable: true},
	}
	cfg, err := BuildConfig(ib, users)
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	if len(cfg.Users) != 1 {
		t.Errorf("want 1 valid user, got %d", len(cfg.Users))
	}
}

func TestMitaConfigToJSON(t *testing.T) {
	cfg := &MitaConfig{
		PortBindings: []PortBinding{{Protocol: "TCP", PortRange: "1234"}},
		Users:        []MitaUser{{Name: "user1", Password: "pass1"}},
		LoggingLevel: "INFO",
		MTU:          1400,
	}
	data, err := cfg.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	var roundtrip MitaConfig
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatalf("unmarshal ToJSON output: %v", err)
	}
	if roundtrip.MTU != 1400 {
		t.Errorf("roundtrip MTU: want 1400, got %d", roundtrip.MTU)
	}
}

func TestValidatePortRange(t *testing.T) {
	cases := []struct {
		input   string
		wantErr bool
	}{
		{"34787", false},
		{"34787-34790", false},
		{"1-65535", false},
		{"0-100", true},    // port 0 invalid
		{"65536", true},    // over max
		{"abc", true},      // non-numeric
		{"100-abc", true},  // non-numeric after dash
		{"", true},         // empty (via SplitN)
		{"-100", true},     // leading dash → empty first segment
	}
	for _, tc := range cases {
		err := validatePortRange(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("validatePortRange(%q): wantErr=%v got err=%v", tc.input, tc.wantErr, err)
		}
	}
}

func TestClientExportJSON(t *testing.T) {
	ib := &model.MieruInbound{
		Id:           1,
		Name:         "test",
		TCPPortRange: "34787",
		MTU:          1400,
	}
	u := &model.MieruUser{
		Id:        1,
		InboundId: 1,
		Username:  "alice",
		Password:  "s3cr3t",
		Enable:    true,
	}
	data, err := ClientExportJSON("example.com", ib, u)
	if err != nil {
		t.Fatalf("ClientExportJSON: %v", err)
	}
	var profile map[string]any
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatalf("unmarshal client profile: %v", err)
	}
	if profile["profileName"] != "alice" {
		t.Errorf("profileName: want alice, got %v", profile["profileName"])
	}
	servers, ok := profile["servers"].([]any)
	if !ok || len(servers) == 0 {
		t.Fatal("servers field missing or empty")
	}
	srv := servers[0].(map[string]any)
	if srv["ipAddress"] != "example.com" {
		t.Errorf("ipAddress: want example.com, got %v", srv["ipAddress"])
	}
}

func TestClientExportText(t *testing.T) {
	ib := &model.MieruInbound{
		Id:           1,
		Name:         "test",
		TCPPortRange: "34787",
		UDPPortRange: "33177",
		MTU:          1400,
	}
	u := &model.MieruUser{
		Id:        1,
		InboundId: 1,
		Username:  "bob",
		Password:  "hunter2",
		Enable:    true,
	}
	text := ClientExportText("1.2.3.4", ib, u)
	for _, want := range []string{"bob", "hunter2", "1.2.3.4", "34787", "33177"} {
		if !strings.Contains(text, want) {
			t.Errorf("ClientExportText: missing %q in output", want)
		}
	}
}
