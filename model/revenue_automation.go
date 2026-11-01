package model

import (
	"context"
	"errors"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrRevenueAutomationConflict = errors.New("Revenue automation changed or is busy")
var revenueFieldKey = regexp.MustCompile(`^[a-z][a-z0-9_]{0,47}$`)
var RevenueTimezone = time.FixedZone("Asia/Shanghai", 8*3600)

type RevenueField struct {
	Key       string `json:"key"`
	Metric    string `json:"metric"`
	Period    string `json:"period"`
	Days      int    `json:"days"`
	ChannelID int    `json:"channel_id"`
}

type RevenueAutomationConfig struct {
	Enabled     bool           `json:"enabled"`
	Time        string         `json:"time"`
	Periods     []string       `json:"periods"`
	MailEnabled bool           `json:"mail_enabled"`
	MailPeriod  string         `json:"mail_period"`
	Recipients  []string       `json:"recipients"`
	Subject     string         `json:"subject"`
	Body        string         `json:"body"`
	Fields      []RevenueField `json:"fields"`
	Fingerprint string         `json:"fingerprint"`
}

type RevenueAutomation struct {
	ID         int    `gorm:"primaryKey;autoIncrement:false" json:"-"`
	Revision   int64  `json:"revision"`
	OwnerID    int    `json:"owner_id"`
	ConfigJSON string `gorm:"size:262144" json:"-"`
	PolicyJSON string `gorm:"size:262144" json:"-"`
	NextAt     int64  `json:"next_at"`
	Lease      string `gorm:"type:varchar(36)" json:"-"`
	LeaseUntil int64  `json:"-"`
}

type RevenueAutomationRun struct {
	ID          string `gorm:"type:varchar(36);primaryKey" json:"id"`
	ScheduledAt int64  `gorm:"uniqueIndex" json:"scheduled_at"`
	Revision    int64  `json:"revision"`
	Status      string `gorm:"type:varchar(20);index" json:"status"`
	Data        string `gorm:"size:262144" json:"-"`
	Error       string `gorm:"type:varchar(160)" json:"error"`
	UpdatedAt   int64  `json:"updated_at"`
	LastActorID int    `json:"last_actor_id"`
}

// Ownership is explicit: retention must never infer automatic jobs from owner ID.
type RevenueAutomationJob struct {
	RequestKey string `gorm:"type:varchar(36);primaryKey"`
	CreatedAt  int64  `gorm:"index"`
}

func DefaultRevenueAutomationConfig() RevenueAutomationConfig {
	return RevenueAutomationConfig{Time: "04:00", Periods: []string{"day", "week", "month", "quarter", "year"}, MailPeriod: "day", Recipients: []string{}, Subject: "{{system_name}} - {{scheduled_at}}", Body: "{{summary_table}}", Fields: []RevenueField{{Key: "yesterday_profit", Metric: "profit", Period: "day", Days: 1}}}
}

func RevenuePeriodDue(period string, at time.Time) bool {
	at = at.In(RevenueTimezone)
	switch period {
	case "day":
		return true
	case "week":
		return at.Weekday() == time.Monday
	case "month":
		return at.Day() == 1
	case "quarter":
		return at.Day() == 1 && (int(at.Month())-1)%3 == 0
	case "year":
		return at.Day() == 1 && at.Month() == time.January
	}
	return false
}

func RevenueWindow(period string, days int, at time.Time) (CostQueryRange, error) {
	at = at.In(RevenueTimezone)
	end := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, RevenueTimezone)
	var start time.Time
	switch period {
	case "day":
		start = end.AddDate(0, 0, -1)
	case "days":
		if days < 1 || days > 366 {
			return CostQueryRange{}, errors.New("Invalid revenue window")
		}
		start = end.AddDate(0, 0, -days)
	case "week":
		end = end.AddDate(0, 0, -(int(end.Weekday())+6)%7)
		start = end.AddDate(0, 0, -7)
	case "month":
		end = time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, RevenueTimezone)
		start = end.AddDate(0, -1, 0)
	case "quarter":
		end = time.Date(at.Year(), time.Month((int(at.Month())-1)/3*3+1), 1, 0, 0, 0, 0, RevenueTimezone)
		start = end.AddDate(0, -3, 0)
	case "year":
		end = time.Date(at.Year(), 1, 1, 0, 0, 0, 0, RevenueTimezone)
		start = end.AddDate(-1, 0, 0)
	default:
		return CostQueryRange{}, errors.New("Invalid revenue window")
	}
	return CostQueryRange{Start: start.Unix(), End: end.Unix()}, nil
}

func RevenueNextDue(clock string, now time.Time) int64 {
	t, _ := time.Parse("15:04", clock)
	local := now.In(RevenueTimezone)
	next := time.Date(local.Year(), local.Month(), local.Day(), t.Hour(), t.Minute(), 0, 0, RevenueTimezone)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next.Unix()
}

func ValidateRevenueAutomationConfig(c RevenueAutomationConfig) error {
	if _, err := time.Parse("15:04", c.Time); err != nil || len(c.Time) != 5 {
		return errors.New("Invalid execution time")
	}
	if len(c.Periods) > 5 || len(c.Fields) > 20 || len(c.Recipients) > 10 || len(c.Subject) > 800 || len(c.Body) > 32768 || strings.ContainsAny(c.Subject, "\r\n") {
		return errors.New("Invalid automation configuration")
	}
	seen := map[string]bool{}
	for _, p := range c.Periods {
		if p == "days" || seen[p] {
			return errors.New("Invalid revenue period")
		}
		if _, err := RevenueWindow(p, 1, time.Now()); err != nil {
			return err
		}
		seen[p] = true
	}
	if _, err := RevenueWindow(c.MailPeriod, 1, time.Now()); err != nil || c.MailPeriod == "days" {
		return errors.New("Invalid mail period")
	}
	seen = map[string]bool{}
	for _, f := range c.Fields {
		if !revenueFieldKey.MatchString(f.Key) || seen[f.Key] || f.ChannelID < 0 {
			return errors.New("Invalid field key")
		}
		seen[f.Key] = true
		if f.Metric != "cost" && f.Metric != "consumption" && f.Metric != "profit" && f.Metric != "profit_margin" {
			return errors.New("Invalid revenue metric")
		}
		if _, err := RevenueWindow(f.Period, f.Days, time.Now()); err != nil {
			return err
		}
	}
	seen = map[string]bool{}
	for _, recipient := range c.Recipients {
		address, err := mail.ParseAddress(recipient)
		if err != nil || address.Address != recipient || strings.ContainsAny(recipient, "\r\n;") || seen[strings.ToLower(recipient)] {
			return errors.New("Invalid recipient")
		}
		seen[strings.ToLower(recipient)] = true
	}
	if c.MailEnabled && (len(c.Fields) == 0 || len(c.Recipients) == 0 || strings.TrimSpace(c.Subject) == "" || strings.TrimSpace(c.Body) == "") {
		return errors.New("Mail configuration is incomplete")
	}
	return nil
}

func GetRevenueAutomation(ctx context.Context) (*RevenueAutomation, RevenueAutomationConfig, error) {
	var row RevenueAutomation
	c := DefaultRevenueAutomationConfig()
	err := DB.WithContext(ctx).First(&row, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &RevenueAutomation{ID: 1}, c, nil
	}
	if err == nil {
		err = common.UnmarshalJsonStr(row.ConfigJSON, &c)
	}
	return &row, c, err
}

func SaveRevenueAutomation(ctx context.Context, actor int, revision int64, c RevenueAutomationConfig) error {
	if err := ValidateRevenueAutomationConfig(c); err != nil {
		return err
	}
	preview, err := PreviewChannelCostReportConfig(ctx, 0)
	if err != nil {
		return err
	}
	if c.Enabled && preview.Fingerprint != c.Fingerprint {
		return ErrCostReportConfigChanged
	}
	policy := CostReportPolicy{CostBasisVersion: 1, Start: 1, End: 2, QuotaPerUnit: preview.QuotaPerUnit, FallbackMode: "billed_basis", CostFingerprint: preview.Fingerprint}
	for _, channel := range preview.Channels {
		policy.Rates = append(policy.Rates, channel.CostReportRate)
	}
	if c.Enabled {
		if err := ValidateCostReportPolicy(policy); err != nil {
			return err
		}
	}
	encoded, err := common.Marshal(c)
	if err != nil {
		return err
	}
	frozen, err := common.Marshal(policy)
	if err != nil {
		return err
	}
	if c.Enabled && len(frozen) > 128*1024 {
		return errors.New("Automation policy exceeds storage limit")
	}
	if len(encoded)+len(frozen) > 256*1024 {
		return errors.New("Automation configuration is too large")
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := authorizeCostReportOwner(tx, actor); err != nil {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&RevenueAutomation{ID: 1}).Error; err != nil {
			return err
		}
		var current RevenueAutomation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, 1).Error; err != nil {
			return err
		}
		if current.Revision != revision || current.LeaseUntil > time.Now().Unix() {
			return ErrRevenueAutomationConflict
		}
		var active int64
		if err := tx.Model(&RevenueAutomationRun{}).Where("status IN ?", []string{"pending", "waiting", "sending"}).Count(&active).Error; err != nil {
			return err
		}
		if active > 0 && c.Enabled {
			return ErrRevenueAutomationConflict
		}
		if !c.Enabled {
			if err := tx.Model(&RevenueAutomationRun{}).Where("status IN ?", []string{"pending", "waiting", "sending"}).Updates(map[string]any{"status": gorm.Expr("CASE WHEN status = ? THEN ? ELSE ? END", "sending", "unknown", "cancelled"), "last_actor_id": actor, "updated_at": time.Now().Unix()}).Error; err != nil {
				return err
			}
		}
		next := RevenueNextDue(c.Time, time.Now())
		return tx.Model(&current).Updates(map[string]any{"revision": revision + 1, "owner_id": actor, "config_json": string(encoded), "policy_json": string(frozen), "next_at": next}).Error
	})
}

func ClaimRevenueAutomation(ctx context.Context) (*RevenueAutomation, RevenueAutomationConfig, error) {
	return claimRevenueAutomation(ctx, true)
}

func ClaimRevenueAutomationControl(ctx context.Context) (*RevenueAutomation, RevenueAutomationConfig, error) {
	return claimRevenueAutomation(ctx, false)
}

func claimRevenueAutomation(ctx context.Context, requireEnabled bool) (*RevenueAutomation, RevenueAutomationConfig, error) {
	row, c, err := GetRevenueAutomation(ctx)
	if err != nil || (requireEnabled && !c.Enabled) || row.Revision == 0 {
		return nil, c, err
	}
	if err = AuthorizeChannelCostReportOwner(ctx, row.OwnerID); err != nil {
		return nil, c, err
	}
	lease := uuid.NewString()
	r := DB.WithContext(ctx).Model(&RevenueAutomation{}).Where("id = ? AND revision = ? AND lease_until <= ?", 1, row.Revision, time.Now().Unix()).Updates(map[string]any{"lease": lease, "lease_until": time.Now().Add(90 * time.Second).Unix()})
	if r.Error != nil {
		return nil, c, r.Error
	}
	if r.RowsAffected != 1 {
		return nil, c, nil
	}
	row.Lease = lease
	return row, c, nil
}

func ReleaseRevenueAutomation(ctx context.Context, row *RevenueAutomation) {
	DB.WithContext(ctx).Model(&RevenueAutomation{}).Where("id = ? AND lease = ?", 1, row.Lease).Updates(map[string]any{"lease": "", "lease_until": 0})
}

func RevenueAutomationTransaction(ctx context.Context, row *RevenueAutomation, fn func(*gorm.DB) error) error {
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&RevenueAutomation{}).Where("id = ? AND revision = ? AND lease = ? AND lease_until > ?", 1, row.Revision, row.Lease, time.Now().Unix()).Update("lease_until", gorm.Expr("lease_until + ?", 1))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrRevenueAutomationConflict
		}
		if err := authorizeCostReportOwner(tx, row.OwnerID); err != nil {
			return err
		}
		return fn(tx)
	})
}

func SaveRevenueAutomationRun(ctx context.Context, row *RevenueAutomation, run *RevenueAutomationRun) error {
	if len(run.Data) > 256*1024 {
		return errors.New("Automation result is too large")
	}
	run.UpdatedAt = time.Now().Unix()
	return RevenueAutomationTransaction(ctx, row, func(tx *gorm.DB) error { return tx.Save(run).Error })
}

func ListRevenueAutomationRuns(ctx context.Context) ([]RevenueAutomationRun, error) {
	rows := []RevenueAutomationRun{}
	err := DB.WithContext(ctx).Omit("data").Order("scheduled_at DESC").Limit(128).Find(&rows).Error
	return rows, err
}
