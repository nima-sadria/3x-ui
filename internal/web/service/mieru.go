package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/mieru"
	"github.com/mhsanaei/3x-ui/v3/internal/util/random"

	"gorm.io/gorm"
)

// MieruService provides CRUD, config generation, lifecycle management, and
// enforcement for Mieru inbounds and users. It never touches Xray.
type MieruService struct{}

// ── Inbound CRUD ────────────────────────────────────────────────────────────

func (s *MieruService) ListInbounds() ([]model.MieruInbound, error) {
	var rows []model.MieruInbound
	if err := database.GetDB().Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *MieruService) GetInbound(id int) (*model.MieruInbound, error) {
	var row model.MieruInbound
	if err := database.GetDB().First(&row, id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *MieruService) CreateInbound(ib *model.MieruInbound) error {
	if err := validateInbound(ib); err != nil {
		return err
	}
	return database.GetDB().Create(ib).Error
}

func (s *MieruService) UpdateInbound(ib *model.MieruInbound) error {
	if err := validateInbound(ib); err != nil {
		return err
	}
	return database.GetDB().Save(ib).Error
}

func (s *MieruService) DeleteInbound(id int) error {
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("inbound_id = ?", id).Delete(&model.MieruUser{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.MieruInbound{}, id).Error
	})
}

func (s *MieruService) SetInboundEnable(id int, enable bool) error {
	return database.GetDB().Model(&model.MieruInbound{}).Where("id = ?", id).Update("enable", enable).Error
}

// ── User CRUD ────────────────────────────────────────────────────────────────

func (s *MieruService) ListUsers(inboundID int) ([]model.MieruUser, error) {
	var rows []model.MieruUser
	if err := database.GetDB().Where("inbound_id = ?", inboundID).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *MieruService) GetUser(id int) (*model.MieruUser, error) {
	var row model.MieruUser
	if err := database.GetDB().First(&row, id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *MieruService) CreateUser(u *model.MieruUser) error {
	if strings.TrimSpace(u.Password) == "" {
		u.Password = random.Seq(32)
	}
	if err := validateUser(u); err != nil {
		return err
	}
	// Check duplicate username within same inbound.
	var count int64
	database.GetDB().Model(&model.MieruUser{}).
		Where("inbound_id = ? AND username = ?", u.InboundId, u.Username).Count(&count)
	if count > 0 {
		return fmt.Errorf("username %q already exists on this inbound", u.Username)
	}
	return database.GetDB().Create(u).Error
}

func (s *MieruService) UpdateUser(u *model.MieruUser) error {
	if err := validateUser(u); err != nil {
		return err
	}
	return database.GetDB().Save(u).Error
}

func (s *MieruService) DeleteUser(id int) error {
	return database.GetDB().Delete(&model.MieruUser{}, id).Error
}

func (s *MieruService) SetUserEnable(id int, enable bool) error {
	return database.GetDB().Model(&model.MieruUser{}).Where("id = ?", id).Update("enable", enable).Error
}

// ── Config generation + apply ────────────────────────────────────────────────

// ApplyInboundConfig generates the mita config for inbound id and applies it.
// Safe to call even when mita is not yet started — `mita apply` handles that.
func (s *MieruService) ApplyInboundConfig(inboundID int) error {
	ib, err := s.GetInbound(inboundID)
	if err != nil {
		return fmt.Errorf("mieru: get inbound %d: %w", inboundID, err)
	}
	users, err := s.ListUsers(inboundID)
	if err != nil {
		return fmt.Errorf("mieru: list users for inbound %d: %w", inboundID, err)
	}
	cfg, err := mieru.BuildConfig(ib, users)
	if err != nil {
		return err
	}
	return mieru.ApplyConfig(cfg)
}

// ApplyAllEnabledInbounds finds the first enabled inbound (currently mita
// supports a single server config) and applies it. If no inbound is enabled,
// it attempts to stop mita gracefully.
func (s *MieruService) ApplyAllEnabledInbounds() error {
	inbounds, err := s.ListInbounds()
	if err != nil {
		return err
	}
	for _, ib := range inbounds {
		if !ib.Enable {
			continue
		}
		users, uErr := s.ListUsers(ib.Id)
		if uErr != nil {
			logger.Warningf("mieru: list users for inbound %d: %v", ib.Id, uErr)
			continue
		}
		cfg, cErr := mieru.BuildConfig(&ib, users)
		if cErr != nil {
			logger.Warningf("mieru: build config for inbound %d: %v", ib.Id, cErr)
			continue
		}
		if aErr := mieru.ApplyConfig(cfg); aErr != nil {
			logger.Errorf("mieru: apply config for inbound %d: %v", ib.Id, aErr)
			return aErr
		}
		return nil // mita only has one server config globally
	}
	// No enabled inbound — stop mita if it is running.
	if mieru.IsMitaAvailable() {
		if err := mieru.Stop(); err != nil {
			logger.Warningf("mieru: stop (no enabled inbounds): %v", err)
		}
	}
	return nil
}

// ── Traffic accounting ────────────────────────────────────────────────────────

// SyncTraffic queries `mita get users` and updates the DB traffic counters.
// Counter resets (new < stored) are treated as a reset baseline so usage is
// never subtracted. Returns false when mita does not expose per-user stats.
func (s *MieruService) SyncTraffic() (bool, error) {
	stats, err := mieru.GetUserStats()
	if err != nil {
		return false, err
	}
	if len(stats) == 0 {
		return false, nil
	}
	usageAvailable := false
	for _, stat := range stats {
		if !stat.UsageAvailable {
			continue
		}
		usageAvailable = true
		var u model.MieruUser
		if err := database.GetDB().
			Where("username = ? AND enable = ?", stat.Username, true).
			First(&u).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			logger.Warningf("mieru: traffic sync lookup for %s: %v", stat.Username, err)
			continue
		}
		// Safe delta: if counter reset, use new value as delta.
		addDown := stat.DownloadBytes
		addUp := stat.UploadBytes
		if stat.DownloadBytes < u.Down {
			addDown = stat.DownloadBytes
		} else {
			addDown = stat.DownloadBytes - u.Down
		}
		if stat.UploadBytes < u.Up {
			addUp = stat.UploadBytes
		} else {
			addUp = stat.UploadBytes - u.Up
		}
		updates := map[string]any{
			"down":            u.Down + addDown,
			"up":              u.Up + addUp,
			"usage_available": true,
		}
		if stat.LastActive > 0 {
			updates["last_online"] = stat.LastActive
		}
		if err := database.GetDB().Model(&model.MieruUser{}).Where("id = ?", u.Id).Updates(updates).Error; err != nil {
			logger.Warningf("mieru: traffic sync update for %s: %v", stat.Username, err)
		}
	}
	return usageAvailable, nil
}

// ── Enforcement ──────────────────────────────────────────────────────────────

// EnforceExpiry disables users that are past their expiry time or over their
// traffic limit (only when usage data is available). Returns the number of
// users disabled and whether a config apply is needed.
func (s *MieruService) EnforceExpiry() (disabled int, needApply bool, err error) {
	now := time.Now().UnixMilli()
	var users []model.MieruUser
	if err = database.GetDB().Where("enable = ?", true).Find(&users).Error; err != nil {
		return 0, false, err
	}
	for _, u := range users {
		shouldDisable := false
		if u.ExpiryTime > 0 && u.ExpiryTime <= now {
			shouldDisable = true
			logger.Infof("mieru: disabling expired user %q (expired %s ago)",
				u.Username, time.Since(time.UnixMilli(u.ExpiryTime)).Round(time.Second))
		}
		if !shouldDisable && u.TrafficLimitGB > 0 && u.UsageAvailable {
			limitBytes := u.TrafficLimitGB * 1024 * 1024 * 1024
			if u.Up+u.Down >= limitBytes {
				shouldDisable = true
				logger.Infof("mieru: disabling over-quota user %q (%d/%d bytes)",
					u.Username, u.Up+u.Down, limitBytes)
			}
		}
		if shouldDisable {
			if dbErr := database.GetDB().Model(&model.MieruUser{}).
				Where("id = ?", u.Id).Update("enable", false).Error; dbErr != nil {
				logger.Warningf("mieru: disable user %q: %v", u.Username, dbErr)
				continue
			}
			disabled++
			needApply = true
		}
	}
	return disabled, needApply, nil
}

// ── Status ────────────────────────────────────────────────────────────────────

func (s *MieruService) GetStatus() mieru.Status {
	return mieru.GetStatus()
}

// ── Client export ─────────────────────────────────────────────────────────────

func (s *MieruService) ExportClientJSON(inboundID, userID int, serverAddr string) ([]byte, error) {
	ib, err := s.GetInbound(inboundID)
	if err != nil {
		return nil, err
	}
	u, err := s.GetUser(userID)
	if err != nil {
		return nil, err
	}
	if u.InboundId != inboundID {
		return nil, fmt.Errorf("user %d does not belong to inbound %d", userID, inboundID)
	}
	return mieru.ClientExportJSON(serverAddr, ib, u)
}

func (s *MieruService) ExportClientText(inboundID, userID int, serverAddr string) (string, error) {
	ib, err := s.GetInbound(inboundID)
	if err != nil {
		return "", err
	}
	u, err := s.GetUser(userID)
	if err != nil {
		return "", err
	}
	if u.InboundId != inboundID {
		return "", fmt.Errorf("user %d does not belong to inbound %d", userID, inboundID)
	}
	return mieru.ClientExportText(serverAddr, ib, u), nil
}

// ── Validation ────────────────────────────────────────────────────────────────

func validateInbound(ib *model.MieruInbound) error {
	if strings.TrimSpace(ib.Name) == "" {
		return errors.New("inbound name is required")
	}
	if strings.TrimSpace(ib.TCPPortRange) == "" && strings.TrimSpace(ib.UDPPortRange) == "" {
		return errors.New("at least one of tcpPortRange or udpPortRange is required")
	}
	if udp := strings.TrimSpace(ib.UDPPortRange); udp != "" {
		if err := mieru.ValidateUDPPortRange(udp); err != nil {
			return err
		}
	}
	return nil
}

func validateUser(u *model.MieruUser) error {
	if u.InboundId <= 0 {
		return errors.New("inboundId is required")
	}
	if strings.TrimSpace(u.Username) == "" {
		return errors.New("username is required")
	}
	if strings.TrimSpace(u.Password) == "" {
		return errors.New("password is required")
	}
	return nil
}
