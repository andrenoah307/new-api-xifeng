package billing_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

func loadTieredBillingSettingForTest(t *testing.T) {
	t.Helper()

	oldBillingSetting := BillingSetting{
		BillingMode: cloneStringMap(billingSetting.BillingMode),
		BillingExpr: cloneStringMap(billingSetting.BillingExpr),
	}
	billingSetting = BillingSetting{
		BillingMode: make(map[string]string),
		BillingExpr: make(map[string]string),
	}
	t.Cleanup(func() {
		billingSetting = oldBillingSetting
	})

	billingModeJSON := mustJSONString(t, map[string]string{"gpt-5.5": BillingModeTieredExpr})
	billingExprJSON := mustJSONString(t, map[string]string{"gpt-5.5": "p*5"})
	if err := config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting." + BillingModeField: billingModeJSON,
		"billing_setting." + BillingExprField: billingExprJSON,
	}); err != nil {
		t.Fatalf("LoadFromDB failed: %v", err)
	}
}

func cloneStringMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func mustJSONString(t *testing.T, value any) string {
	t.Helper()

	jsonBytes, err := common.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return string(jsonBytes)
}

func TestGetBillingMode_NormalizesSuffix(t *testing.T) {
	loadTieredBillingSettingForTest(t)

	tests := []struct {
		model string
		want  string
	}{
		{model: "gpt-5.5[1m]", want: BillingModeTieredExpr},
		{model: "gpt-5.5", want: BillingModeTieredExpr},
		{model: "unknown-model", want: BillingModeRatio},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if got := GetBillingMode(tt.model); got != tt.want {
				t.Fatalf("GetBillingMode(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

func TestGetBillingExpr_NormalizesSuffix(t *testing.T) {
	loadTieredBillingSettingForTest(t)

	expr, ok := GetBillingExpr("gpt-5.5[1m]")
	if !ok {
		t.Fatalf("GetBillingExpr(%q) ok = false", "gpt-5.5[1m]")
	}
	if expr != "p*5" {
		t.Fatalf("GetBillingExpr(%q) = %q, want %q", "gpt-5.5[1m]", expr, "p*5")
	}

	expr, ok = GetBillingExpr("unknown-model")
	if ok {
		t.Fatalf("GetBillingExpr(%q) ok = true", "unknown-model")
	}
	if expr != "" {
		t.Fatalf("GetBillingExpr(%q) = %q, want empty", "unknown-model", expr)
	}
}
