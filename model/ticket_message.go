package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"strings"
)

type TicketMessage struct {
	Id          int    `json:"id"`
	TicketId    int    `json:"ticket_id" gorm:"index;not null"`
	UserId      int    `json:"user_id" gorm:"index;not null"`
	Username    string `json:"username" gorm:"type:varchar(64)"`
	Role        int    `json:"role" gorm:"type:int"`
	Content     string `json:"content" gorm:"type:text;not null"`
	Internal    bool   `json:"internal"`
	CreatedTime int64  `json:"created_time" gorm:"bigint"`
}

func (message *TicketMessage) AfterFind(tx *gorm.DB) error {
	if message.Role != common.RoleFinanceUser && message.Role < common.RoleAdminUser {
		return nil
	}
	for _, prefix := range []string{"Financial reconciliation #", "因退款工单计算扣回历史优惠赠送额度：", "按原申请余额口径核验退款：", "管理员选择退款计算方式", "Invoice correction recorded:"} {
		if strings.HasPrefix(message.Content, prefix) {
			message.Internal = true
			break
		}
	}
	return nil
}

// Internal operations share the business transaction and never count as replies.
func recordTicketOperationTx(tx *gorm.DB, ticketID, actorID int, content string) error {
	actor := User{Role: common.RoleAdminUser}
	if actorID != 0 {
		if err := tx.Select("id, username, role").Where("id = ?", actorID).Find(&actor).Error; err != nil {
			return err
		}
	}
	return tx.Create(&TicketMessage{TicketId: ticketID, UserId: actorID, Username: actor.Username, Role: actor.Role, Internal: true, Content: content}).Error
}

func (message *TicketMessage) BeforeCreate(tx *gorm.DB) error {
	if message.CreatedTime == 0 {
		message.CreatedTime = common.GetTimestamp()
	}
	return nil
}
