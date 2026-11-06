package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RecoverPressureCoolingChannel commits routing eligibility with the status.
// Cache state is not evidence that a previous database write succeeded.
func RecoverPressureCoolingChannel(id int, reason string) (bool, error) {
	channelStatusLock.Lock()
	defer channelStatusLock.Unlock()
	changed := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var channel Channel
		if err := tx.Select("id", "status", "other_info").Clauses(clause.Locking{Strength: "UPDATE"}).First(&channel, id).Error; err != nil {
			return err
		}
		if channel.Status != common.ChannelStatusAutoDisabled {
			return nil
		}
		info := channel.GetOtherInfo()
		info["status_reason"] = reason
		info["status_time"] = common.GetTimestamp()
		channel.SetOtherInfo(info)
		result := tx.Model(&Channel{}).Where("id = ? AND status = ?", id, common.ChannelStatusAutoDisabled).
			Updates(map[string]any{"status": common.ChannelStatusEnabled, "other_info": channel.OtherInfo})
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		if err := tx.Model(&Ability{}).Where("channel_id = ?", id).Update("enabled", true).Error; err != nil {
			return err
		}
		changed = true
		return nil
	})
	if err != nil {
		return false, err
	}
	if changed {
		CacheUpdateChannelStatus(id, common.ChannelStatusEnabled)
	}
	return changed, nil
}
