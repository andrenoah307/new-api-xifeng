package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// LoadEpayRoutingExportNotes must receive only orders authorized by the export query.
// Decode a metadata whitelist, never return credentials or consult mutable settings.
func LoadEpayRoutingExportNotes(db *gorm.DB, owners map[int]int) (map[int]string, error) {
	notes := make(map[int]string)
	ids := make([]int, 0, len(owners))
	for id := range owners {
		ids = append(ids, id)
	}
	for start := 0; start < len(ids); start += 500 {
		var orders []TopUp
		if err := db.Select("id", "user_id", "trade_no", "payment_method", "epay_credentials").Where("id IN ?", ids[start:min(start+500, len(ids))]).Find(&orders).Error; err != nil {
			return nil, err
		}
		for _, order := range orders {
			if owners[order.Id] != order.UserId || order.EpayCredentials == "" {
				continue
			}
			var snapshot struct {
				GatewayID   string `json:"gateway_id"`
				GatewayName string `json:"gateway_name"`
				AccountName string `json:"account_name"`
			}
			if err := common.UnmarshalJsonStr(order.EpayCredentials, &snapshot); err != nil {
				return nil, err
			}
			if snapshot.GatewayID == "" {
				continue
			}
			if snapshot.GatewayID == "private" {
				snapshot.AccountName = ""
			}
			notes[order.Id] = fmt.Sprintf("%s | %s | %s | %s", order.TradeNo, order.PaymentMethod, snapshot.GatewayName, snapshot.AccountName)
		}
	}
	return notes, nil
}
