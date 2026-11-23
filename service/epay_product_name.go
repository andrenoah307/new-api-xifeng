package service

import (
	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// PurchaseEpay binds every Epay product name to the authenticated payer and signed payment amount.
func PurchaseEpay(client *epay.Client, userID int, kind string, args *epay.PurchaseArgs) (string, map[string]string, error) {
	var user model.User
	if err := model.DB.Select("username").First(&user, userID).Error; err != nil {
		return "", nil, err
	}
	common.OptionMapRWMutex.RLock()
	template := common.OptionMap["EpayProductNameTemplate"]
	common.OptionMapRWMutex.RUnlock()
	name, err := operation_setting.RenderEpayProductName(template, user.Username, args.Money, args.ServiceTradeNo, kind)
	if err != nil {
		return "", nil, err
	}
	args.Name = name
	return client.Purchase(args)
}
