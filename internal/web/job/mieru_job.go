package job

import (
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// MieruJob runs periodically to sync traffic counters from mita and enforce
// expiry/quota limits. It is only registered when ENABLE_MIERU_PROVIDER=true.
type MieruJob struct {
	mieruService service.MieruService
}

// NewMieruJob returns a new MieruJob.
func NewMieruJob() *MieruJob {
	return new(MieruJob)
}

// Run syncs traffic then enforces expiry/quota, applying new mita config if any
// users were disabled. Errors are logged but never propagate so a mita failure
// cannot take down the cron scheduler.
func (j *MieruJob) Run() {
	// Sync traffic first so enforcement uses up-to-date counters.
	if _, err := j.mieruService.SyncTraffic(); err != nil {
		logger.Warningf("mieru job: traffic sync: %v", err)
	}

	disabled, needApply, err := j.mieruService.EnforceExpiry()
	if err != nil {
		logger.Warningf("mieru job: enforce: %v", err)
		return
	}
	if disabled > 0 {
		logger.Infof("mieru job: disabled %d user(s) due to expiry or quota", disabled)
	}
	if needApply {
		if err := j.mieruService.ApplyAllEnabledInbounds(); err != nil {
			logger.Warningf("mieru job: apply config after enforcement: %v", err)
		}
	}
}
