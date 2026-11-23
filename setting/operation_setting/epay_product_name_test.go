package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEpayProductName(t *testing.T) {
	for _, tc := range []struct{ template, username, money, kind, want string }{
		{"", "demo_user", "10.00", "充值", "米醋API(demo_user)，充值10.00元"},
		{"", "demo_user", "9.50", "订阅", "米醋API(demo_user)，订阅9.50元"},
		{"", "demo_user", "0.60", "补税", "米醋API(demo_user)，补税0.60元"},
		{"{username}/{amount}/{order_no}/{type}", "{amount}", "100000.01", "充值", "{amount}/100000.01/ORDER123/充值"},
	} {
		got, err := RenderEpayProductName(tc.template, tc.username, tc.money, "ORDER123", tc.kind)
		require.NoError(t, err)
		assert.Equal(t, tc.want, got)
	}
	for _, template := range []string{"{username}", "{amount}", "{username}{amount}{unknown}", "{{username}}{amount}", "{username}{amount}\n"} {
		assert.Error(t, ValidateEpayProductNameTemplate(template), template)
	}
	for _, money := range []string{"10", "10.001", "-1.00", "NaN", "0.00"} {
		_, err := RenderEpayProductName("", "demo_user", money, "ORDER123", "充值")
		assert.Error(t, err, money)
	}
}
