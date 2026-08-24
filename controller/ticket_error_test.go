package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleTicketError(t *testing.T) {
	require.NoError(t, i18n.Init())
	oldMinInvoiceAmount := operation_setting.MinInvoiceAmount
	operation_setting.MinInvoiceAmount = 50
	defer func() { operation_setting.MinInvoiceAmount = oldMinInvoiceAmount }()

	testCases := []struct {
		name       string
		err        error
		messageKey string
		args       map[string]any
		message    string
	}{
		{name: "subject empty", err: model.ErrTicketSubjectEmpty, messageKey: i18n.MsgTicketSubjectEmpty},
		{name: "content empty", err: model.ErrTicketContentEmpty, messageKey: i18n.MsgTicketContentEmpty},
		{name: "ticket not found", err: model.ErrTicketNotFound, messageKey: i18n.MsgTicketNotFound},
		{name: "forbidden", err: model.ErrTicketForbidden, messageKey: i18n.MsgForbidden},
		{name: "closed", err: model.ErrTicketClosed, messageKey: i18n.MsgTicketClosed},
		{name: "invalid status", err: model.ErrTicketInvalidStatus, messageKey: i18n.MsgTicketInvalidStatus},
		{name: "invalid type", err: model.ErrTicketInvalidType, messageKey: i18n.MsgTicketInvalidType},
		{name: "invoice not found", err: model.ErrTicketInvoiceNotFound, messageKey: i18n.MsgTicketInvoiceNotFound},
		{name: "invoice status invalid", err: model.ErrTicketInvoiceStatusInvalid, messageKey: i18n.MsgTicketInvoiceStatusInvalid},
		{name: "invoice order empty", err: model.ErrTicketInvoiceOrderEmpty, messageKey: i18n.MsgTicketInvoiceOrderEmpty},
		{name: "invoice order invalid", err: model.ErrTicketInvoiceOrderInvalid, messageKey: i18n.MsgTicketInvoiceOrderInvalid},
		{name: "invoice order duplicate", err: model.ErrTicketInvoiceOrderDuplicate, messageKey: i18n.MsgTicketInvoiceOrderDuplicate},
		{name: "invoice company empty", err: model.ErrTicketInvoiceCompanyEmpty, messageKey: i18n.MsgTicketInvoiceCompanyEmpty},
		{name: "invoice tax number empty", err: model.ErrTicketInvoiceTaxNumberEmpty, messageKey: i18n.MsgTicketInvoiceTaxNumberEmpty},
		{name: "invoice tax number format", err: model.ErrTicketInvoiceTaxNumberFormat, messageKey: i18n.MsgTicketInvoiceTaxNumberFormat},
		{name: "invoice email empty", err: model.ErrTicketInvoiceEmailEmpty, messageKey: i18n.MsgTicketInvoiceEmailEmpty},
		{
			name:       "invoice remark too long",
			err:        model.ErrTicketInvoiceRemarkTooLong,
			messageKey: i18n.MsgTicketInvoiceRemarkTooLong,
			args:       map[string]any{"Max": model.MaxInvoiceRemarkLength},
		},
		{
			name:       "invoice amount below minimum",
			err:        model.ErrTicketInvoiceAmountBelowMin,
			messageKey: i18n.MsgTicketInvoiceAmountBelowMin,
			args:       map[string]any{"Amount": 50},
		},
		{name: "refund not found", err: model.ErrTicketRefundNotFound, messageKey: i18n.MsgTicketRefundNotFound},
		{name: "refund status invalid", err: model.ErrTicketRefundStatusInvalid, messageKey: i18n.MsgTicketRefundStatusInvalid},
		{name: "refund quota invalid", err: model.ErrTicketRefundQuotaInvalid, messageKey: i18n.MsgTicketRefundQuotaInvalid},
		{name: "refund quota exceed", err: model.ErrTicketRefundQuotaExceed, messageKey: i18n.MsgTicketRefundQuotaExceed},
		{name: "refund payee type empty", err: model.ErrTicketRefundPayeeTypeEmpty, messageKey: i18n.MsgTicketRefundPayeeTypeEmpty},
		{name: "refund payee name empty", err: model.ErrTicketRefundPayeeNameEmpty, messageKey: i18n.MsgTicketRefundPayeeNameEmpty},
		{name: "refund payee account empty", err: model.ErrTicketRefundPayeeAccountEmpty, messageKey: i18n.MsgTicketRefundPayeeAccountEmpty},
		{name: "refund payee bank empty", err: model.ErrTicketRefundPayeeBankEmpty, messageKey: i18n.MsgTicketRefundPayeeBankEmpty},
		{name: "refund contact empty", err: model.ErrTicketRefundContactEmpty, messageKey: i18n.MsgTicketRefundContactEmpty},
		{name: "refund not pending", err: model.ErrTicketRefundNotPending, messageKey: i18n.MsgTicketRefundNotPending},
		{name: "refund quota mode invalid", err: model.ErrTicketRefundQuotaModeInvalid, messageKey: i18n.MsgTicketRefundQuotaModeInvalid},
		{name: "refund clawback quota invalid", err: model.ErrTicketRefundClawbackQuotaInvalid, messageKey: i18n.MsgTicketRefundClawbackQuotaInvalid},
		{name: "assignee invalid", err: model.ErrTicketAssigneeInvalid, messageKey: i18n.MsgTicketAssigneeInvalid},
		{name: "attachment not found", err: model.ErrAttachmentNotFound, message: "attachment not found"},
		{name: "attachment forbidden", err: model.ErrAttachmentForbidden, message: "attachment belongs to another user"},
		{name: "attachment bound", err: model.ErrAttachmentBound, message: "attachment already bound"},
		{name: "attachment belongs to another ticket", err: model.ErrAttachmentBindTicket, message: "attachment belongs to another ticket"},
		{name: "unexpected error", err: errors.New("unexpected ticket error"), message: "unexpected ticket error"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodGet, "/", nil)

			handleTicketError(context, testCase.err)

			assert.Equal(t, http.StatusOK, recorder.Code)
			var body struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
			assert.False(t, body.Success)

			expectedMessage := testCase.message
			if testCase.messageKey != "" {
				expectedMessage = i18n.Translate(i18n.LangEn, testCase.messageKey, testCase.args)
			}
			assert.Equal(t, expectedMessage, body.Message)
		})
	}
}
