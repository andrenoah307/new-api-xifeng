package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

type LogExportSetting struct {
	Enabled           bool `json:"enabled"`
	WorkerCount       int  `json:"worker_count"`
	QueueSize         int  `json:"queue_size"`
	MaxFileSizeMB     int  `json:"max_file_size_mb"`
	MaxTotalStorageMB int  `json:"max_total_storage_mb"`
	UserCooldownHours int  `json:"user_cooldown_hours"`
	RetentionHours    int  `json:"retention_hours"`
}

var logExportSetting = LogExportSetting{
	Enabled:           true,
	WorkerCount:       2,
	QueueSize:         100,
	MaxFileSizeMB:     200,
	MaxTotalStorageMB: 2048,
	UserCooldownHours: 24,
	RetentionHours:    72,
}

func init() {
	config.GlobalConfig.Register("log_export", &logExportSetting)
}

func GetLogExportSetting() *LogExportSetting {
	NormalizeLogExportSetting(&logExportSetting)
	return &logExportSetting
}

func NormalizeLogExportSetting(s *LogExportSetting) {
	if s == nil {
		return
	}
	if s.WorkerCount <= 0 {
		s.WorkerCount = 2
	}
	if s.QueueSize <= 0 {
		s.QueueSize = 100
	}
	if s.MaxFileSizeMB <= 0 {
		s.MaxFileSizeMB = 200
	}
	if s.MaxTotalStorageMB <= 0 {
		s.MaxTotalStorageMB = 2048
	}
	if s.UserCooldownHours <= 0 {
		s.UserCooldownHours = 24
	}
	if s.RetentionHours <= 0 {
		s.RetentionHours = 72
	}
}
