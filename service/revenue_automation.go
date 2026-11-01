package service

import (
	"context"
	"errors"
	"fmt"
	"html"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

type RevenueMailDelivery struct {
	Recipient string `json:"recipient"`
	Status    string `json:"status"`
	Attempts  int    `json:"attempts"`
	NextAt    int64  `json:"next_at"`
}
type RevenueAutomationData struct {
	Source     string                 `json:"source"`
	Windows    []model.CostQueryRange `json:"windows"`
	Index      int                    `json:"index"`
	JobID      string                 `json:"job_id,omitempty"`
	Subject    string                 `json:"subject"`
	Body       string                 `json:"body"`
	Values     map[string]string      `json:"values"`
	Deliveries []RevenueMailDelivery  `json:"deliveries"`
}

// Template values are scalar strings except for the server-built summary table.
func RenderRevenueMail(c model.RevenueAutomationConfig, vars map[string]string) (string, string, error) {
	for _, template := range []string{c.Subject, c.Body} {
		rest := template
		for {
			start := strings.Index(rest, "{{")
			if start < 0 {
				if strings.Contains(rest, "}}") {
					return "", "", errors.New("Invalid mail placeholder")
				}
				break
			}
			end := strings.Index(rest[start+2:], "}}")
			if end < 0 {
				return "", "", errors.New("Invalid mail placeholder")
			}
			key := strings.TrimSpace(rest[start+2 : start+2+end])
			if _, ok := vars[key]; !ok {
				return "", "", fmt.Errorf("Unknown mail placeholder: %s", key)
			}
			rest = rest[start+end+4:]
		}
	}
	safe := map[string]string{}
	for key, value := range vars {
		safe[key] = html.EscapeString(value)
	}
	safe["summary_table"] = vars["summary_table"]
	subjectVars := map[string]string{}
	for key, value := range vars {
		subjectVars[key] = value
	}
	subjectVars["summary_table"] = ""
	subject := common.RenderPlaceholders(c.Subject, subjectVars)
	if strings.ContainsAny(subject, "\r\n") || len(subject) > 1000 {
		return "", "", errors.New("Invalid mail subject")
	}
	body := common.RenderPlaceholders(c.Body, safe)
	if len(body) > 128*1024 {
		return "", "", errors.New("Mail body is too large")
	}
	// Reuse the ticket notification information-table appearance; custom HTML is
	// supported inside this shell and isolated by the frontend preview sandbox.
	body = `<div style="background:#fafafa;padding:24px;font-family:Arial,sans-serif;color:#1d1d1f"><div style="max-width:640px;margin:auto;background:white;padding:24px;border:1px solid #ebebeb">` + body + `</div></div>`
	return subject, body, nil
}

func RevenueMailPreview(c model.RevenueAutomationConfig) (string, string, error) {
	if err := model.ValidateRevenueAutomationConfig(c); err != nil {
		return "", "", err
	}
	now := time.Now().In(model.RevenueTimezone)
	vars := map[string]string{"system_name": common.SystemNameOrDefault(), "scheduled_at": now.Format("2006-01-02 15:04:05"), "generated_at": now.Format("2006-01-02 15:04:05"), "run_id": "preview"}
	rows := []common.EmailTemplateRow{}
	for _, f := range c.Fields {
		window, _ := model.RevenueWindow(f.Period, f.Days, now)
		prefix := "fields." + f.Key + "."
		vars[prefix+"value"] = "123.456789"
		if f.Metric == "profit_margin" {
			vars[prefix+"value"] = "12.34%"
		}
		vars[prefix+"start"] = time.Unix(window.Start, 0).In(model.RevenueTimezone).Format("2006-01-02 15:04:05")
		vars[prefix+"end"] = time.Unix(window.End, 0).In(model.RevenueTimezone).Format("2006-01-02 15:04:05")
		vars[prefix+"unit"] = "report units"
		vars[prefix+"coverage"] = "95.00%"
		rows = append(rows, common.EmailTemplateRow{Label: f.Key, Value: html.EscapeString(vars[prefix+"value"])})
	}
	vars["summary_table"] = common.RenderInfoTableHTML(rows)
	return RenderRevenueMail(c, vars)
}

var revenueAutomationBusy atomic.Bool

func triggerRevenueAutomation() {
	if !revenueAutomationBusy.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer revenueAutomationBusy.Store(false)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := RunRevenueAutomationPass(ctx); err != nil {
			logger.LogWarn(ctx, "revenue automation: "+err.Error())
		}
	}()
}

func RunRevenueAutomationPass(ctx context.Context) error {
	config, c, err := model.ClaimRevenueAutomation(ctx)
	if err != nil || config == nil {
		return err
	}
	defer model.ReleaseRevenueAutomation(context.Background(), config)
	run, err := model.NextRevenueAutomationRun(ctx, config, "{}")
	if err != nil || run == nil {
		return err
	}
	if run.Status == "failed" {
		return nil
	}
	var data RevenueAutomationData
	if err = common.UnmarshalJsonStr(run.Data, &data); err != nil {
		return err
	}
	err = advanceRevenueAutomation(ctx, config, c, run, &data)
	if err != nil {
		run.Status = "failed"
		for _, delivery := range data.Deliveries {
			if delivery.Status == "sending" || delivery.Status == "unknown" {
				run.Status = "unknown"
			}
		}
		run.Error = "Revenue automation failed; review configuration and report dependencies"
		if errors.Is(err, model.ErrCostReportConfigChanged) {
			run.Error = "Channel cost settings changed; confirm the automation policy again"
		}
		if errors.Is(err, model.ErrCostQueryTooLarge) {
			run.Error = "Too many summaries. Narrow the time range."
		}
		if errors.Is(err, model.ErrChannelCostReportSourceChanged) {
			run.Error = "Report source changed; create a new automation run"
		}
		logger.LogWarn(ctx, fmt.Sprintf("revenue automation run %s: %v", run.ID, err))
	}
	encoded, encodeErr := common.Marshal(data)
	if encodeErr != nil {
		return encodeErr
	}
	if len(encoded) > 240*1024 {
		run.Status = "failed"
		run.Error = "Automation result is too large; reduce template or fields"
		for _, delivery := range data.Deliveries {
			if delivery.Status == "sending" || delivery.Status == "unknown" {
				run.Status = "unknown"
			}
		}
		// Never discard delivery evidence, even when reducing an oversized snapshot.
		data.Values = nil
		encoded, encodeErr = common.Marshal(data)
		if encodeErr != nil {
			return encodeErr
		}
	}
	run.Data = string(encoded)
	return model.SaveRevenueAutomationRun(ctx, config, run)
}

func advanceRevenueAutomation(ctx context.Context, config *model.RevenueAutomation, c model.RevenueAutomationConfig, run *model.RevenueAutomationRun, data *RevenueAutomationData) error {
	var policy model.CostReportPolicy
	if err := common.UnmarshalJsonStr(config.PolicyJSON, &policy); err != nil {
		return err
	}
	identity, err := model.ChannelCostReportSourceIdentity(ctx)
	if err != nil {
		return err
	}
	if data.Source != "" && data.Source != identity {
		return model.ErrChannelCostReportSourceChanged
	}
	at := time.Unix(run.ScheduledAt, 0).In(model.RevenueTimezone)
	mailDue := c.MailEnabled && model.RevenuePeriodDue(c.MailPeriod, at)
	if data.Source == "" {
		data.Source = identity
		seen := map[model.CostQueryRange]bool{}
		for _, period := range []string{"day", "week", "month", "quarter", "year"} {
			if slices.Contains(c.Periods, period) && model.RevenuePeriodDue(period, at) {
				window, _ := model.RevenueWindow(period, 1, at)
				if !seen[window] {
					data.Windows = append(data.Windows, window)
					seen[window] = true
				}
			}
		}
		if mailDue {
			for _, field := range c.Fields {
				window, _ := model.RevenueWindow(field.Period, field.Days, at)
				if !seen[window] {
					data.Windows = append(data.Windows, window)
					seen[window] = true
				}
			}
		}
	}
	if data.Index < len(data.Windows) {
		if data.JobID != "" {
			dependency, err := model.GetChannelCostReportJobState(ctx, data.JobID)
			if err != nil {
				return err
			}
			if dependency.Status == "pending" || dependency.Status == "running" {
				run.Status = "waiting"
				return nil
			}
			if dependency.Status != "completed" {
				return errors.New("Scheduled report is stopped; resume its summary task first")
			}
		}
		window := data.Windows[data.Index]
		queryCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		result, err := model.QueryChannelCostReportsWithPolicy(queryCtx, window.Start, window.End, 0, &policy)
		cancel()
		if err != nil {
			return err
		}
		if result.Complete {
			data.Index++
			data.JobID = ""
			run.Status = "waiting"
			return nil
		}
		preview, err := model.PreviewChannelCostReportConfig(ctx, 0)
		if err != nil {
			return err
		}
		if preview.Fingerprint != c.Fingerprint {
			return model.ErrCostReportConfigChanged
		}
		active, err := model.GetActiveSystemTask(model.SystemTaskTypeChannelCostReport)
		if err != nil {
			return err
		}
		if active != nil {
			run.Status = "waiting"
			return nil
		}
		policy.Start, policy.End = window.Start, window.End
		job, err := model.EnsureRevenueAutomationReport(ctx, config, policy)
		if errors.Is(err, model.ErrChannelCostReportConflict) {
			run.Status = "waiting"
			return nil
		}
		if err != nil {
			return err
		}
		data.JobID = job.ID
		job, err = model.GetChannelCostReportJob(ctx, job.ID)
		if err != nil {
			return err
		}
		if job.Status != "pending" && job.Status != "running" {
			return errors.New("Scheduled report is stopped or no longer covers its source")
		}
		run.Status = "waiting"
		notifySystemTaskRunner()
		return nil
	}
	if !mailDue {
		run.Status = "completed"
		return nil
	}
	if data.Subject == "" {
		vars := map[string]string{"system_name": common.SystemNameOrDefault(), "scheduled_at": at.Format("2006-01-02 15:04:05"), "generated_at": time.Now().In(model.RevenueTimezone).Format("2006-01-02 15:04:05"), "run_id": run.ID}
		rows := []common.EmailTemplateRow{}
		for _, field := range c.Fields {
			window, _ := model.RevenueWindow(field.Period, field.Days, at)
			queryCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			result, err := model.QueryChannelCostReportsWithPolicy(queryCtx, window.Start, window.End, field.ChannelID, &policy)
			cancel()
			if err != nil {
				return err
			}
			if !result.Complete {
				return errors.New("Email result coverage changed")
			}
			cost, revenue := decimal.Zero, decimal.Zero
			var known, total int64
			for _, row := range result.Rows {
				original, e := decimal.NewFromString(row.OriginalCost)
				if e != nil {
					return e
				}
				fallback, e := decimal.NewFromString(row.FallbackCost)
				if e != nil {
					return e
				}
				consumption, e := decimal.NewFromString(row.KnownRevenue)
				if e != nil {
					return e
				}
				cost = cost.Add(original).Add(fallback)
				revenue = revenue.Add(consumption)
				known += row.OriginalCount + row.FallbackCount
				total += row.ConsumeCount
			}
			value := cost.StringFixed(6)
			switch field.Metric {
			case "consumption":
				value = revenue.StringFixed(6)
			case "profit":
				value = revenue.Sub(cost).StringFixed(6)
			case "profit_margin":
				value = "-"
				if !revenue.IsZero() {
					value = revenue.Sub(cost).Div(revenue).Mul(decimal.NewFromInt(100)).StringFixed(2) + "%"
				}
			}
			prefix := "fields." + field.Key + "."
			vars[prefix+"value"] = value
			vars[prefix+"start"] = time.Unix(window.Start, 0).In(model.RevenueTimezone).Format("2006-01-02 15:04:05")
			vars[prefix+"end"] = time.Unix(window.End, 0).In(model.RevenueTimezone).Format("2006-01-02 15:04:05")
			vars[prefix+"unit"] = "quota / " + policy.QuotaPerUnit
			vars[prefix+"coverage"] = "-"
			if total > 0 {
				vars[prefix+"coverage"] = decimal.NewFromInt(known).Div(decimal.NewFromInt(total)).Mul(decimal.NewFromInt(100)).StringFixed(2) + "%"
			}
			rows = append(rows, common.EmailTemplateRow{Label: field.Key, Value: html.EscapeString(value + " (" + vars[prefix+"start"] + " - " + vars[prefix+"end"] + "; " + vars[prefix+"coverage"] + ")")})
		}
		vars["summary_table"] = common.RenderInfoTableHTML(rows)
		subject, body, err := RenderRevenueMail(c, vars)
		if err != nil {
			return err
		}
		data.Subject, data.Body, data.Values = subject, body, vars
		for _, recipient := range c.Recipients {
			data.Deliveries = append(data.Deliveries, RevenueMailDelivery{Recipient: recipient, Status: "pending"})
		}
		run.Status = "sending"
		return nil
	}
	for i := range data.Deliveries {
		delivery := &data.Deliveries[i]
		if delivery.Status == "sending" {
			delivery.Status = "unknown"
			run.Status = "unknown"
			return nil
		}
		if delivery.Status != "pending" && delivery.Status != "retryable" {
			continue
		}
		if delivery.NextAt > time.Now().Unix() {
			return nil
		}
		delivery.Status = "sending"
		delivery.Attempts++
		encoded, err := common.Marshal(data)
		if err != nil {
			return err
		}
		run.Data = string(encoded)
		if err = model.SaveRevenueAutomationRun(ctx, config, run); err != nil {
			return err
		}
		if err = model.AuthorizeChannelCostReportOwner(ctx, config.OwnerID); err != nil {
			return err
		}
		state, _ := common.SendRevenueEmail(ctx, data.Subject, delivery.Recipient, data.Body)
		delivery.Status = state
		if state == "retryable" {
			if delivery.Attempts >= 4 {
				delivery.Status = "rejected"
			} else {
				delay := []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute}[delivery.Attempts-1]
				delivery.NextAt = time.Now().Add(delay).Unix()
			}
		}
		return nil
	}
	run.Status = "completed"
	for _, d := range data.Deliveries {
		if d.Status == "unknown" {
			run.Status = "unknown"
			break
		}
		if d.Status != "accepted" {
			run.Status = "failed"
		}
	}
	return nil
}
