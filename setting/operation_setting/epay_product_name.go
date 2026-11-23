package operation_setting

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const DefaultEpayProductNameTemplate = "米醋API({username})，{type}{amount}元"

var epayMoneyPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.[0-9]{2}$`)

func ValidateEpayProductNameTemplate(template string) error {
	if template == "" {
		return nil
	}
	remaining := strings.NewReplacer("{username}", "", "{amount}", "", "{order_no}", "", "{type}", "").Replace(template)
	if !utf8.ValidString(template) || utf8.RuneCountInString(template) > 200 ||
		!strings.Contains(template, "{username}") || !strings.Contains(template, "{amount}") ||
		strings.ContainsAny(remaining, "{}") || strings.IndexFunc(template, unicode.IsControl) >= 0 {
		return errors.New("Invalid Epay product name template")
	}
	return nil
}

// Money is the exact two-decimal RMB string sent to the gateway, not credits or net invoice value.
func RenderEpayProductName(template, username, money, orderNo, kind string) (string, error) {
	if err := ValidateEpayProductNameTemplate(template); err != nil {
		return "", err
	}
	if username == "" || !epayMoneyPattern.MatchString(money) || money == "0.00" {
		return "", errors.New("invalid Epay product name values")
	}
	if template == "" {
		template = DefaultEpayProductNameTemplate
	}
	// A single pass keeps placeholder-like text in usernames literal.
	return strings.NewReplacer("{username}", username, "{amount}", money, "{order_no}", orderNo, "{type}", kind).Replace(template), nil
}
