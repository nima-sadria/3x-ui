package mieru

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

const (
	commandTimeout  = 30 * time.Second
	generatedConfig = "generated-mita-config.json"
	backupConfig    = "generated-mita-config.json.bak"
)

// configPath returns the full path of the generated mita config file.
func configPath() string {
	return filepath.Join(config.GetMieruConfigDir(), generatedConfig)
}

func backupPath() string {
	return filepath.Join(config.GetMieruConfigDir(), backupConfig)
}

// MitaNotFoundError is returned when the mita binary cannot be found.
type MitaNotFoundError struct{}

func (MitaNotFoundError) Error() string {
	return "mita executable not found in PATH — install Mieru server to use this feature"
}

// IsMitaAvailable reports whether the mita binary exists on PATH.
func IsMitaAvailable() bool {
	_, err := exec.LookPath("mita")
	return err == nil
}

// runMita executes a mita subcommand, returning stdout/stderr on success or an
// error with the combined output on failure. Never panics on mita not found.
func runMita(args ...string) (string, error) {
	cmd := exec.Command("mita", args...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if isNotFound(err) {
			return "", MitaNotFoundError{}
		}
		if text != "" {
			return "", fmt.Errorf("mita %s: %s", strings.Join(args, " "), text)
		}
		return "", fmt.Errorf("mita %s: %w", strings.Join(args, " "), err)
	}
	return text, nil
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	var exitErr *exec.ExitError
	if ok := false; !ok {
		_ = exitErr
	}
	return strings.Contains(err.Error(), "executable file not found") ||
		strings.Contains(err.Error(), "no such file")
}

// ApplyConfig writes cfg to a temp file, backs up the existing config, applies
// the new one via `mita apply config <path>`, then reloads mita. On any
// failure after backup it restores the previous config.
func ApplyConfig(cfg *MitaConfig) error {
	if !IsMitaAvailable() {
		return MitaNotFoundError{}
	}
	dir := config.GetMieruConfigDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("mieru: create config dir: %w", err)
	}

	data, err := cfg.ToJSON()
	if err != nil {
		return fmt.Errorf("mieru: marshal config: %w", err)
	}

	// Atomic write via temp file + rename.
	tmpFile, err := os.CreateTemp(dir, "mita-*.json")
	if err != nil {
		return fmt.Errorf("mieru: create temp config: %w", err)
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("mieru: write temp config: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("mieru: close temp config: %w", err)
	}

	// Backup existing config before applying.
	cfgPath := configPath()
	backPath := backupPath()
	if _, statErr := os.Stat(cfgPath); statErr == nil {
		if err := copyFile(cfgPath, backPath); err != nil {
			logger.Warningf("mieru: backup config failed (continuing): %v", err)
		}
	}

	// Move temp into place.
	if err := os.Rename(tmpPath, cfgPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("mieru: rename config: %w", err)
	}

	// Apply via mita.
	if _, err := runMita("apply", "config", cfgPath); err != nil {
		logger.Errorf("mieru: apply config failed: %v — attempting rollback", err)
		rollbackConfig(cfgPath, backPath)
		return fmt.Errorf("mieru: apply config: %w", err)
	}

	logger.Info("mieru: config applied successfully")

	// Reload (non-fatal: a fresh apply already restarts mita internally on most
	// versions; log warning but don't block the caller).
	if out, reloadErr := runMita("reload"); reloadErr != nil {
		logger.Warningf("mieru: reload after apply failed (mita may not support reload yet): %v", reloadErr)
		// Try start in case it wasn't running.
		if _, startErr := runMita("start"); startErr != nil {
			logger.Warningf("mieru: start after apply also failed: %v", startErr)
		}
	} else if out != "" {
		logger.Debugf("mieru: reload: %s", out)
	}
	return nil
}

// Start starts the mita server.
func Start() error {
	if !IsMitaAvailable() {
		return MitaNotFoundError{}
	}
	out, err := runMita("start")
	if err != nil {
		return fmt.Errorf("mieru: start: %w", err)
	}
	if out != "" {
		logger.Infof("mieru: start: %s", out)
	}
	return nil
}

// Stop stops the mita server.
func Stop() error {
	if !IsMitaAvailable() {
		return MitaNotFoundError{}
	}
	out, err := runMita("stop")
	if err != nil {
		return fmt.Errorf("mieru: stop: %w", err)
	}
	if out != "" {
		logger.Infof("mieru: stop: %s", out)
	}
	return nil
}

// Status holds the current mita runtime state.
type Status struct {
	MitaAvailable bool   `json:"mitaAvailable"`
	Running       bool   `json:"running"`
	StatusOutput  string `json:"statusOutput"`
	ConfigOutput  string `json:"configOutput"`
	Error         string `json:"error,omitempty"`
}

// GetStatus queries mita for its current status. Never returns an error —
// failures are captured in the Status struct so the API always responds 200.
func GetStatus() Status {
	s := Status{MitaAvailable: IsMitaAvailable()}
	if !s.MitaAvailable {
		s.Error = "mita not installed"
		return s
	}

	statusOut, err := runMita("status")
	if err != nil {
		s.Error = err.Error()
		return s
	}
	s.StatusOutput = statusOut

	lower := strings.ToLower(statusOut)
	s.Running = strings.Contains(lower, "running") || strings.Contains(lower, "started")

	cfgOut, err := runMita("describe", "config")
	if err != nil {
		// Non-fatal — mita may be stopped but status still works.
		logger.Debugf("mieru: describe config: %v", err)
	} else {
		s.ConfigOutput = cfgOut
	}
	return s
}

// GetUserStats calls `mita get users` and parses the JSON output. Returns nil
// slice without error when mita is not available or the output is empty/unknown.
func GetUserStats() ([]UserStat, error) {
	if !IsMitaAvailable() {
		return nil, nil
	}
	out, err := runMita("get", "users")
	if err != nil {
		return nil, err
	}
	return parseUserStats(out), nil
}

// UserStat is the per-user traffic data returned by `mita get users`.
type UserStat struct {
	Username       string
	DownloadBytes  int64
	UploadBytes    int64
	LastActive     int64 // unix ms, 0 if unknown
	UsageAvailable bool
}

func parseUserStats(output string) []UserStat {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil
	}
	// Try JSON first (mita ≥ v2 returns JSON from `get users`).
	var parsed struct {
		Users []struct {
			Name                     string `json:"username"`
			NameAlt                  string `json:"name"`
			DownloadBytes            int64  `json:"downloadBytesSinceLastStart"`
			DownloadBytesAlt         int64  `json:"downloadBytes"`
			UploadBytes              int64  `json:"uploadBytesSinceLastStart"`
			UploadBytesAlt           int64  `json:"uploadBytes"`
			LastActiveSec            int64  `json:"lastActive"`
		} `json:"users"`
	}
	if err := json.Unmarshal([]byte(output), &parsed); err == nil && len(parsed.Users) > 0 {
		out := make([]UserStat, 0, len(parsed.Users))
		for _, u := range parsed.Users {
			name := u.Name
			if name == "" {
				name = u.NameAlt
			}
			down := u.DownloadBytes
			if down == 0 {
				down = u.DownloadBytesAlt
			}
			up := u.UploadBytes
			if up == 0 {
				up = u.UploadBytesAlt
			}
			var lastMs int64
			if u.LastActiveSec > 0 {
				lastMs = u.LastActiveSec * 1000
			}
			out = append(out, UserStat{
				Username:       name,
				DownloadBytes:  down,
				UploadBytes:    up,
				LastActive:     lastMs,
				UsageAvailable: true,
			})
		}
		return out
	}
	// Plain-JSON array fallback.
	var arr []struct {
		Name     string `json:"name"`
		Username string `json:"username"`
	}
	if err := json.Unmarshal([]byte(output), &arr); err == nil {
		out := make([]UserStat, 0, len(arr))
		for _, u := range arr {
			name := u.Username
			if name == "" {
				name = u.Name
			}
			out = append(out, UserStat{Username: name, UsageAvailable: false})
		}
		return out
	}
	// Could parse table output here (like proxy-manager does), but for now just
	// return nil so callers skip traffic accounting gracefully.
	return nil
}

func rollbackConfig(cfgPath, backPath string) {
	if _, err := os.Stat(backPath); err != nil {
		logger.Warning("mieru: no backup config to restore")
		return
	}
	if err := copyFile(backPath, cfgPath); err != nil {
		logger.Errorf("mieru: rollback copy failed: %v", err)
		return
	}
	if _, err := runMita("apply", "config", cfgPath); err != nil {
		logger.Errorf("mieru: rollback apply failed: %v", err)
		return
	}
	logger.Warning("mieru: rolled back to previous config after failed apply")
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o640)
}
