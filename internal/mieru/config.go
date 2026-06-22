// Package mieru manages the external mita (Mieru server) process for the
// 3x-ui panel. Mieru is NOT an Xray protocol — it is managed entirely
// through the `mita` command-line tool and never appears in the Xray config.
package mieru

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// PortBinding represents one TCP or UDP port binding for mita.
type PortBinding struct {
	Protocol  string `json:"protocol"`
	PortRange string `json:"portRange"`
}

// MitaUser is the credential object mita's config file accepts.
type MitaUser struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

// MitaConfig is the JSON structure expected by `mita apply config <file>`.
type MitaConfig struct {
	PortBindings []PortBinding `json:"portBindings"`
	Users        []MitaUser    `json:"users"`
	LoggingLevel string        `json:"loggingLevel"`
	MTU          int           `json:"mtu"`
}

// BuildConfig generates a MitaConfig from an inbound and its enabled users.
// Disabled users are excluded so disabling a user takes effect on next apply.
func BuildConfig(inbound *model.MieruInbound, users []model.MieruUser) (*MitaConfig, error) {
	if inbound == nil {
		return nil, fmt.Errorf("mieru: nil inbound")
	}
	bindings, err := buildPortBindings(inbound.TCPPortRange, inbound.UDPPortRange)
	if err != nil {
		return nil, err
	}
	if len(bindings) == 0 {
		return nil, fmt.Errorf("mieru: inbound %q has no port bindings (set tcpPortRange or udpPortRange)", inbound.Name)
	}

	loggingLevel := strings.ToUpper(strings.TrimSpace(inbound.LoggingLevel))
	if loggingLevel == "" {
		loggingLevel = "INFO"
	}
	mtu := inbound.MTU
	if mtu <= 0 {
		mtu = 1400
	}

	mitaUsers := make([]MitaUser, 0, len(users))
	for _, u := range users {
		if !u.Enable {
			continue
		}
		if strings.TrimSpace(u.Username) == "" || strings.TrimSpace(u.Password) == "" {
			continue
		}
		mitaUsers = append(mitaUsers, MitaUser{
			Name:     u.Username,
			Password: u.Password,
		})
	}

	return &MitaConfig{
		PortBindings: bindings,
		Users:        mitaUsers,
		LoggingLevel: loggingLevel,
		MTU:          mtu,
	}, nil
}

// ToJSON marshals the config to indented JSON, ready to write to disk.
func (c *MitaConfig) ToJSON() ([]byte, error) {
	return json.MarshalIndent(c, "", "  ")
}

// buildPortBindings parses tcpPortRange and udpPortRange strings into
// PortBinding slices. Either may be empty to omit that protocol.
func buildPortBindings(tcpRange, udpRange string) ([]PortBinding, error) {
	var out []PortBinding
	tcp := strings.TrimSpace(tcpRange)
	udp := strings.TrimSpace(udpRange)
	if tcp != "" {
		if err := validatePortRange(tcp); err != nil {
			return nil, fmt.Errorf("mieru: invalid tcpPortRange %q: %w", tcp, err)
		}
		out = append(out, PortBinding{Protocol: "TCP", PortRange: tcp})
	}
	if udp != "" {
		if err := ValidateUDPPortRange(udp); err != nil {
			return nil, fmt.Errorf("mieru: invalid udpPortRange: %w", err)
		}
		out = append(out, PortBinding{Protocol: "UDP", PortRange: udp})
	}
	return out, nil
}

// validatePortRange checks that a port range is either a single port ("34787")
// or a dash-separated range ("34787-34790") with sensible values.
func validatePortRange(s string) error {
	parts := strings.SplitN(s, "-", 2)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return fmt.Errorf("empty port segment in %q", s)
		}
		n := 0
		for _, ch := range p {
			if ch < '0' || ch > '9' {
				return fmt.Errorf("non-numeric character in port %q", p)
			}
			n = n*10 + int(ch-'0')
		}
		if n < 1 || n > 65535 {
			return fmt.Errorf("port %d out of range [1,65535]", n)
		}
	}
	return nil
}

// ValidateUDPPortRange is like validatePortRange but additionally rejects single
// ports. mita requires UDP bindings to be expressed as an explicit range even
// when only one port is intended — use "33177-33177" not "33177".
func ValidateUDPPortRange(s string) error {
	if err := validatePortRange(s); err != nil {
		return err
	}
	if !strings.Contains(s, "-") {
		return fmt.Errorf("UDP port range %q must be a dash-separated range (e.g. %s-%s); mita does not accept single ports for UDP", s, s, s)
	}
	return nil
}

// ClientExportJSON builds the official Mieru client profile JSON that can be
// imported directly into a Mieru-compatible client (e.g. the desktop app or
// Hiddify). serverAddr is the panel's public address.
func ClientExportJSON(serverAddr string, inbound *model.MieruInbound, user *model.MieruUser) ([]byte, error) {
	bindings, err := buildPortBindings(inbound.TCPPortRange, inbound.UDPPortRange)
	if err != nil {
		return nil, err
	}

	clientBindings := make([]map[string]string, 0, len(bindings))
	for _, b := range bindings {
		clientBindings = append(clientBindings, map[string]string{
			"protocol":  b.Protocol,
			"portRange": b.PortRange,
		})
	}

	profile := map[string]any{
		"profileName":   user.Username,
		"activeProfile": user.Username,
		"user": map[string]string{
			"name":     user.Username,
			"password": user.Password,
		},
		"servers": []map[string]any{
			{
				"ipAddress":    serverAddr,
				"domainName":   serverAddr,
				"portBindings": clientBindings,
			},
		},
		"advancedSettings": map[string]any{
			"mtu":                  inbound.MTU,
			"multiplexing":         map[string]any{"level": "MULTIPLEXING_HIGH"},
			"handshakeTimeoutSecs": 8,
		},
		"socks5Port":    1080,
		"httpProxyPort": 8080,
		"rpcPort":       8964,
		"loggingLevel":  "INFO",
	}
	return json.MarshalIndent(profile, "", "  ")
}

// ClientExportText returns a plain-text summary for manual configuration.
func ClientExportText(serverAddr string, inbound *model.MieruInbound, user *model.MieruUser) string {
	var sb strings.Builder
	sb.WriteString("Mieru Client Configuration\n")
	sb.WriteString("==========================\n")
	fmt.Fprintf(&sb, "Server:    %s\n", serverAddr)
	if inbound.TCPPortRange != "" {
		fmt.Fprintf(&sb, "TCP Ports: %s\n", inbound.TCPPortRange)
	}
	if inbound.UDPPortRange != "" {
		fmt.Fprintf(&sb, "UDP Ports: %s\n", inbound.UDPPortRange)
	}
	fmt.Fprintf(&sb, "Username:  %s\n", user.Username)
	fmt.Fprintf(&sb, "Password:  %s\n", user.Password)
	fmt.Fprintf(&sb, "MTU:       %d\n", inbound.MTU)
	return sb.String()
}
