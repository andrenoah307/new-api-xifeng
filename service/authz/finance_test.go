package authz

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFinanceRoleOnlyGrantsFinancialReads(t *testing.T) {
	require.NoError(t, Init(newAuthzTestDB(t)))
	for _, permission := range []Permission{FinanceRead, FinanceExport} {
		assert.True(t, Can(7, common.RoleFinanceUser, permission))
		assert.True(t, Can(8, common.RoleAdminUser, permission))
		assert.False(t, Can(9, common.RoleCommonUser, permission))
		assert.False(t, Can(10, common.RoleCustomerServiceUser, permission))
	}
	for _, permission := range []Permission{ChannelRead, ChannelWrite, ChannelOperate, ChannelSecretView} {
		assert.False(t, Can(7, common.RoleFinanceUser, permission))
	}
}
