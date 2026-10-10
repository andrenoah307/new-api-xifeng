package model

import (
	"strings"

	"gorm.io/gorm"
)

type MonitoringCandidate struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// MonitoringCandidates only projects display names, never credentials or user profiles.
func MonitoringCandidates(users bool, keyword string, ids []int, selected bool) ([]MonitoringCandidate, error) {
	rows := make([]MonitoringCandidate, 0)
	query := DB.Model(&Channel{}).Select("id, name")
	if users {
		query = DB.Model(&User{}).Select("id, username as name")
	}
	if selected {
		for start := 0; start < len(ids); start += 500 {
			end := min(start+500, len(ids))
			var batch []MonitoringCandidate
			if err := query.Session(&gorm.Session{}).Where("id IN ?", ids[start:end]).Find(&batch).Error; err != nil {
				return nil, err
			}
			rows = append(rows, batch...)
		}
		return rows, nil
	}
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return rows, nil
	}
	// Explicit escape character is portable across SQLite, MySQL and PostgreSQL.
	pattern := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(keyword) + "%"
	if users {
		query = query.Where("username LIKE ? ESCAPE '!' OR display_name LIKE ? ESCAPE '!'", pattern, pattern)
	} else {
		query = query.Where("name LIKE ? ESCAPE '!'", pattern)
	}
	err := query.Order("id DESC").Limit(20).Find(&rows).Error
	return rows, err
}
