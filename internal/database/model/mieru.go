package model

// MieruInbound holds the configuration for one Mieru server listener managed
// by this panel. It is NOT an Xray inbound — it is never injected into the
// Xray config. The lifecycle is driven through the external `mita` binary.
type MieruInbound struct {
	Id           int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Name         string `json:"name" gorm:"uniqueIndex;not null"`
	Enable       bool   `json:"enable" gorm:"default:true"`
	TCPPortRange string `json:"tcpPortRange" gorm:"column:tcp_port_range"` // e.g. "34787-34790", empty = no TCP
	UDPPortRange string `json:"udpPortRange" gorm:"column:udp_port_range"` // e.g. "33177-33180", empty = no UDP
	MTU          int    `json:"mtu" gorm:"default:1400"`
	LoggingLevel string `json:"loggingLevel" gorm:"column:logging_level;default:INFO"`
	CreatedAt    int64  `json:"createdAt" gorm:"autoCreateTime:milli"`
	UpdatedAt    int64  `json:"updatedAt" gorm:"autoUpdateTime:milli"`
}

func (MieruInbound) TableName() string { return "mieru_inbounds" }

// MieruUser is a Mieru client credential bound to one MieruInbound. The
// plaintext password is stored because mita's config requires it verbatim.
// Traffic counters are best-effort — they are only updated when mita exposes
// per-user stats via `mita get users`.
type MieruUser struct {
	Id              int    `json:"id" gorm:"primaryKey;autoIncrement"`
	InboundId       int    `json:"inboundId" gorm:"column:inbound_id;index;not null"`
	Username        string `json:"username" gorm:"not null"`
	Password        string `json:"password" gorm:"not null"`
	Enable          bool   `json:"enable" gorm:"default:true"`
	TrafficLimitGB  int64  `json:"trafficLimitGB" gorm:"column:traffic_limit_gb;default:0"` // 0 = unlimited
	ExpiryTime      int64  `json:"expiryTime" gorm:"column:expiry_time;default:0"`          // unix ms, 0 = never
	Up              int64  `json:"up" gorm:"default:0"`
	Down            int64  `json:"down" gorm:"default:0"`
	LastOnline      int64  `json:"lastOnline" gorm:"column:last_online;default:0"`
	UsageAvailable  bool   `json:"usageAvailable" gorm:"column:usage_available;default:false"`
	CreatedAt       int64  `json:"createdAt" gorm:"autoCreateTime:milli"`
	UpdatedAt       int64  `json:"updatedAt" gorm:"autoUpdateTime:milli"`
}

func (MieruUser) TableName() string { return "mieru_users" }
