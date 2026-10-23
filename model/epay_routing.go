package model

import (
	"crypto/rand"
	"math/big"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
)

var epayGatewayIDPattern = regexp.MustCompile(`^[a-zA-Z0-9-]{1,48}$`)

func saveInvoiceEpayRouting(config InvoiceEpayConfig) error {
	config.Gateways = slices.Clone(config.Gateways)
	config.PrivateGatewayName = strings.TrimSpace(config.PrivateGatewayName)
	if config.PrivateGatewayName == "" || len(config.PrivateGatewayName) > 200 {
		return ErrInvoiceTaxInvalid
	}
	if len(config.Gateways) > 20 || config.PrivateBelowCents < 0 || config.PrivateBelowCents > 100000000000 || (config.Strategy != "round_robin" && config.Strategy != "random") {
		return ErrInvoiceTaxInvalid
	}
	return DB.Session(&gorm.Session{Logger: gormlogger.Discard}).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Option{Key: InvoiceEpayConfigKey, Value: "{}"}).Error; err != nil {
			return err
		}
		previous, err := GetInvoiceEpayConfig(lockForUpdate(tx))
		if err != nil {
			return err
		}
		old := map[string]InvoiceEpayGateway{}
		for _, gateway := range previous.Gateways {
			old[gateway.ID] = gateway
		}
		if previous.Gateways == nil {
			old["legacy"] = InvoiceEpayGateway{ID: "legacy", Address: previous.Address, PartnerID: previous.PartnerID, Key: previous.Key, PaymentMethods: previous.CheckoutMethods()}
		}
		seen := map[string]bool{}
		for i := range config.Gateways {
			gateway := &config.Gateways[i]
			gateway.Name = strings.TrimSpace(gateway.Name)
			if gateway.Name == "" || len(gateway.Name) > 200 {
				return ErrInvoiceTaxInvalid
			}
			gateway.AccountName = strings.TrimSpace(gateway.AccountName)
			gateway.Address = strings.TrimRight(strings.TrimSpace(gateway.Address), "/")
			gateway.PartnerID = strings.TrimSpace(gateway.PartnerID)
			gateway.Key = strings.TrimSpace(gateway.Key)
			address, err := url.Parse(gateway.Address)
			if !epayGatewayIDPattern.MatchString(gateway.ID) || gateway.ID == "private" || seen[gateway.ID] || gateway.AccountName == "" || len(gateway.AccountName) > 200 || err != nil || address.Host == "" || (address.Scheme != "https" && address.Scheme != "http") || address.User != nil || address.RawQuery != "" || address.Fragment != "" || gateway.PartnerID == "" {
				return ErrInvoiceTaxInvalid
			}
			seen[gateway.ID] = true
			for _, method := range gateway.PaymentMethods {
				if method != "alipay" && method != "wxpay" {
					return ErrInvoiceTaxInvalid
				}
			}
			if gateway.PaymentMethods == nil {
				gateway.PaymentMethods = []string{}
			}
			if gateway.Key == "" {
				prior, ok := old[gateway.ID]
				if !ok || prior.Address != gateway.Address || prior.PartnerID != gateway.PartnerID || prior.Key == "" {
					return ErrInvoiceTaxInvalid
				}
				gateway.Key = prior.Key
			}
		}
		// New routing settings replace the legacy singleton atomically.
		config.Address, config.PartnerID, config.Key, config.GatewayID, config.AccountName = "", "", "", "", ""
		config.GatewayName = ""
		config.PaymentMethods = nil
		raw, err := common.Marshal(config)
		if err != nil {
			return err
		}
		oldRaw, err := common.Marshal(previous)
		if err != nil {
			return err
		}
		if string(raw) == string(oldRaw) {
			return nil
		}
		result := tx.Model(&Option{}).Where(Option{Key: InvoiceEpayConfigKey}).Update("value", string(raw))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInvoiceTaxInvalid
		}
		return nil
	})
}

// Returns only routes compatible with the amount. Selection happens at order creation.
func GetEpayCheckoutCandidates(db *gorm.DB, invoiceType string, totalCents int64) ([]InvoiceEpayConfig, string, error) {
	if !slices.Contains([]string{"none", "ordinary", "special"}, invoiceType) || totalCents < 0 || totalCents > 100000000000 {
		return nil, "", ErrInvoiceTaxInvalid
	}
	config, err := GetInvoiceEpayConfig(db)
	if err != nil {
		return nil, "", err
	}
	routes := []InvoiceEpayConfig{}
	strategy := config.Strategy
	if strategy == "" {
		strategy = "round_robin"
	}
	if invoiceType == "none" || totalCents < config.PrivateBelowCents {
		name := config.PrivateGatewayName
		if name == "" {
			name = "Epay"
		}
		routes = append(routes, InvoiceEpayConfig{Address: operation_setting.PayAddress, PartnerID: operation_setting.EpayId, Key: operation_setting.EpayKey, GatewayID: "private", GatewayName: name, PaymentMethods: (InvoiceEpayConfig{}).CheckoutMethods()})
	} else if config.Gateways == nil {
		config.GatewayID = "legacy"
		if config.GatewayName == "" {
			config.GatewayName = "Epay"
		}
		config.PaymentMethods = config.CheckoutMethods()
		routes = append(routes, config)
	} else {
		for _, gateway := range config.Gateways {
			routes = append(routes, InvoiceEpayConfig{Address: gateway.Address, PartnerID: gateway.PartnerID, Key: gateway.Key, PaymentMethods: gateway.PaymentMethods, GatewayID: gateway.ID, GatewayName: gateway.Name, AccountName: gateway.AccountName})
		}
	}
	eligible := []InvoiceEpayConfig{}
	for _, route := range routes {
		if route.Address != "" && route.PartnerID != "" && route.Key != "" {
			eligible = append(eligible, route)
		}
	}
	return eligible, strategy, nil
}

func selectEpayCheckoutGateway(tx *gorm.DB, routes []InvoiceEpayConfig, strategy, method string) (InvoiceEpayConfig, error) {
	if len(routes) == 0 {
		return InvoiceEpayConfig{}, ErrInvoiceTaxInvalid
	}
	if len(routes) == 1 {
		return routes[0], nil
	}
	if strategy == "random" {
		index, err := rand.Int(rand.Reader, big.NewInt(int64(len(routes))))
		if err != nil {
			return InvoiceEpayConfig{}, err
		}
		return routes[index.Int64()], nil
	}
	if strategy != "round_robin" {
		return InvoiceEpayConfig{}, ErrInvoiceTaxInvalid
	}
	key := "EpayRouteCursor" + method + "Secret"
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Option{Key: key, Value: "0"}).Error; err != nil {
		return InvoiceEpayConfig{}, err
	}
	var cursor Option
	if err := lockForUpdate(tx).Where(Option{Key: key}).Take(&cursor).Error; err != nil {
		return InvoiceEpayConfig{}, err
	}
	index, err := strconv.ParseUint(cursor.Value, 10, 64)
	if err != nil {
		return InvoiceEpayConfig{}, err
	}
	route := routes[index%uint64(len(routes))]
	next := strconv.FormatUint((index%uint64(len(routes))+1)%uint64(len(routes)), 10)
	result := tx.Model(&Option{}).Where(Option{Key: key, Value: cursor.Value}).Update("value", next)
	if result.Error != nil {
		return InvoiceEpayConfig{}, result.Error
	}
	if result.RowsAffected != 1 {
		return InvoiceEpayConfig{}, ErrInvoiceTaxInvalid
	}
	return route, nil
}
