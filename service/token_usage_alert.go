package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"net/mail"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/go-redis/redis/v8"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const tokenAlertPrefix = "notify:{token-usage}:"

var tokenAlertStart sync.Once
var tokenAlertChanges = make(chan int, 4096)
var tokenAlertDropped atomic.Uint64
var sendTokenUsageAlertEmail = common.SendRevenueEmail

// No network, database, goroutine creation or blocking wait on the quota path.
func StartTokenUsageAlerts() {
	tokenAlertStart.Do(func() {
		model.SetTokenUsageAlertObserver(func(id int) {
			if id <= 0 {
				return
			}
			select {
			case tokenAlertChanges <- id:
			default:
				tokenAlertDropped.Add(1)
			}
		})
		go runTokenUsageAlertSampler()
		go runTokenUsageAlertSender()
	})
}

const cacheTokenUsageAlertLua = `
local old = redis.call('ZSCORE', KEYS[2], ARGV[1])
if not old or tonumber(old) <= tonumber(ARGV[3]) then
 redis.call('HSET', KEYS[1], ARGV[1], ARGV[2])
 redis.call('ZADD', KEYS[2], ARGV[3], ARGV[1])
end
local n = redis.call('ZCARD', KEYS[2])
if n > 10000 then
 local expired = redis.call('ZRANGE', KEYS[2], 0, n-10001)
 for _,id in ipairs(expired) do redis.call('HDEL', KEYS[1], id); redis.call('ZREM', KEYS[2], id) end
end
redis.call('EXPIRE', KEYS[1], 172800)
redis.call('EXPIRE', KEYS[2], 172800)
return 1`

var enqueueTokenUsageAlert = redis.NewScript(`
if ARGV[3] ~= 'retry' and redis.call('EXISTS', KEYS[2]) == 1 then return 0 end
if redis.call('ZSCORE', KEYS[1], ARGV[1]) then return 0 end
if redis.call('ZCARD', KEYS[1]) >= 10000 then return -1 end
redis.call('ZADD', KEYS[1], ARGV[2], ARGV[1])
redis.call('EXPIRE', KEYS[1], 172800)
return 1`)

var popTokenUsageAlert = redis.NewScript(`
local ids = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', ARGV[1], 'LIMIT', 0, 1)
if #ids == 0 then return '' end
if not redis.call('SET', KEYS[2], '1', 'NX', 'EX', 6) then return '' end
redis.call('ZREM', KEYS[1], ids[1])
return ids[1]`)

func tokenUsageAlertAvailable() bool {
	return setting.TokenUsageAlertsEnabled() && common.RedisEnabled && common.RDB != nil
}

func runTokenUsageAlertSampler() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	pending := map[int]int64{}
	var ticks int
	for now := range ticker.C {
		if !tokenUsageAlertAvailable() {
			clear(pending)
			continue
		}
		// Bound both memory and work per tick. The keyset sweep repairs dropped
		// notices and refreshes inactive keys after policy changes/period resets.
		for n := 0; n < 4096; n++ {
			select {
			case id := <-tokenAlertChanges:
				if len(pending) < 4096 {
					if _, ok := pending[id]; !ok {
						delay := int64(1)
						if common.BatchUpdateEnabled {
							delay += int64(common.BatchUpdateInterval)
						}
						pending[id] = now.Unix() + delay
					}
				}
			default:
				n = 4096
			}
		}
		ids := make([]int, 0, 200)
		for id, due := range pending {
			if due <= now.Unix() {
				ids = append(ids, id)
				delete(pending, id)
				if len(ids) == 200 {
					break
				}
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if len(ids) > 0 {
			tokens, err := model.LoadTokenUsageAlertTokens(ctx, ids, 0)
			if err == nil {
				err = cacheAndQueueTokenUsageAlerts(ctx, tokens, now)
			}
			if err != nil {
				common.SysError("API Key usage alert sampling failed")
				for _, id := range ids {
					if len(pending) < 4096 {
						pending[id] = now.Unix() + 10
					}
				}
			}
		}
		cancel()
		ticks++
		if ticks%5 == 0 {
			sweepTokenUsageAlerts(now)
		}
	}
}

func cacheAndQueueTokenUsageAlerts(ctx context.Context, tokens []model.Token, now time.Time) error {
	config := setting.GetTokenUsageAlerts()
	if !config.Enabled {
		return nil
	}
	pipe := common.RDB.Pipeline()
	var queued []*redis.Cmd
	for _, token := range tokens {
		usages := model.TokenUsageAlertUsages(token, now)
		data, err := common.Marshal(usages)
		if err != nil {
			return err
		}
		id := fmt.Sprintf("%d:%d", token.UserId, token.Id)
		// Eval (rather than EvalSha) also works after Redis SCRIPT FLUSH inside
		// a pipeline, without retrying a partially executed batch.
		pipe.Eval(ctx, cacheTokenUsageAlertLua, []string{tokenAlertPrefix + "usage", tokenAlertPrefix + "usage-index"}, id, string(data), now.UnixMilli())
		for _, rule := range config.Rules {
			if _, matches := model.MatchTokenUsageAlert(usages, rule); !matches {
				continue
			}
			candidate := model.TokenUsageAlertID(token.UserId, token.Id, rule.ID)
			queued = append(queued, enqueueTokenUsageAlert.Eval(ctx, pipe, []string{tokenAlertPrefix + "queue", tokenAlertPrefix + "cooldown:" + candidate}, candidate, now.Unix()))
		}
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	for _, result := range queued {
		if n, _ := result.Int(); n < 0 {
			tokenAlertDropped.Add(1)
		}
	}
	return nil
}

func sweepTokenUsageAlerts(now time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	// A short shared slot bounds recovery scans across replicas; cursor updates
	// are only committed after the read and cache writes succeed.
	locked, err := common.RDB.SetNX(ctx, tokenAlertPrefix+"sweep-slot", "1", 5*time.Second).Result()
	if err != nil || !locked {
		return
	}
	after, err := common.RDB.Get(ctx, tokenAlertPrefix+"cursor").Int()
	if err != nil && err != redis.Nil {
		return
	}
	tokens, err := model.LoadTokenUsageAlertTokens(ctx, nil, after)
	if err != nil {
		return
	}
	if err = cacheAndQueueTokenUsageAlerts(ctx, tokens, now); err != nil {
		return
	}
	after = 0
	if len(tokens) == 200 {
		after = tokens[len(tokens)-1].Id
	}
	_ = common.RDB.Set(ctx, tokenAlertPrefix+"cursor", after, 24*time.Hour).Err()
	maintenance, maintenanceErr := common.RDB.SetNX(ctx, tokenAlertPrefix+"maintenance-slot", "1", time.Minute).Result()
	if maintenanceErr == nil && maintenance {
		retries, err := model.MaintainTokenUsageAlerts(ctx, now.Unix())
		if err != nil {
			common.SysError("API Key usage alert maintenance failed")
			return
		}
		for _, delivery := range retries {
			_, _ = enqueueTokenUsageAlert.Run(ctx, common.RDB, []string{tokenAlertPrefix + "queue", tokenAlertPrefix + "cooldown:" + delivery.ID}, delivery.ID, now.Unix()).Result()
		}
	}
}

func runTokenUsageAlertSender() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for now := range ticker.C {
		if !tokenUsageAlertAvailable() {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		id, err := popTokenUsageAlert.Run(ctx, common.RDB, []string{tokenAlertPrefix + "queue", tokenAlertPrefix + "send-slot"}, now.Unix()).Text()
		cancel()
		if err != nil || id == "" {
			continue
		}
		processTokenUsageAlert(id)
	}
}

func processTokenUsageAlert(id string) {
	parts := strings.Split(id, ":")
	if len(parts) != 3 {
		return
	}
	userID, err := strconv.Atoi(parts[0])
	if err != nil {
		return
	}
	tokenID, err := strconv.Atoi(parts[1])
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// Read persisted settings before each send: disabling a rule must not wait
	// for the process-local option refresh interval.
	var option model.Option
	if model.DB.WithContext(ctx).Where(&model.Option{Key: setting.TokenUsageAlertOption}).First(&option).Error != nil {
		return
	}
	config, err := setting.ParseTokenUsageAlerts(option.Value)
	if err != nil || !config.Enabled {
		return
	}
	var rule setting.TokenUsageAlertRule
	for _, candidate := range config.Rules {
		if candidate.ID == parts[2] {
			rule = candidate
			break
		}
	}
	if rule.ID == "" {
		return
	}
	tokens, err := model.LoadTokenUsageAlertTokens(ctx, []int{tokenID}, 0)
	if err != nil || len(tokens) != 1 || tokens[0].UserId != userID {
		return
	}
	token := tokens[0]
	usage, matched := model.MatchTokenUsageAlert(model.TokenUsageAlertUsages(token, time.Now()), rule)
	if !matched {
		return
	}
	var user model.User
	if model.DB.WithContext(ctx).Select("id,status,email,username").Where("id = ?", userID).First(&user).Error != nil || user.Status != common.UserStatusEnabled {
		return
	}
	address, err := mail.ParseAddress(user.Email)
	if err != nil || address.Address != user.Email {
		return
	}
	var prior model.TokenUsageAlertDelivery
	readErr := model.DB.WithContext(ctx).Where("id = ?", id).First(&prior).Error
	if readErr != nil && !errors.Is(readErr, gorm.ErrRecordNotFound) {
		return
	}
	if readErr == nil && prior.CooldownUntil > time.Now().Unix() && (prior.Status != "retryable" || prior.NextAttemptAt > time.Now().Unix() || prior.Attempts >= 3) {
		until := prior.CooldownUntil
		if prior.Status == "retryable" && prior.Attempts < 3 && prior.NextAttemptAt > time.Now().Unix() {
			until = prior.NextAttemptAt
		}
		_ = common.RDB.Set(ctx, tokenAlertPrefix+"cooldown:"+id, "1", time.Until(time.Unix(until, 0))).Err()
		return
	}
	subject, body, err := RenderTokenUsageAlertEmail(config, token, user, usage, rule)
	if err != nil {
		common.SysError("API Key usage alert template rendering failed")
		return
	}
	// A user can own many keys. Recipient pacing delays other rules without
	// consuming their independent cooldowns.
	recipient := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ToLower(user.Email))))
	allowed, err := common.RDB.SetNX(ctx, tokenAlertPrefix+"recipient:"+recipient, "1", time.Minute).Result()
	if err != nil {
		return
	}
	if !allowed {
		_, _ = enqueueTokenUsageAlert.Run(ctx, common.RDB, []string{tokenAlertPrefix + "queue", tokenAlertPrefix + "cooldown:" + id}, id, time.Now().Unix()+60).Result()
		return
	}
	delivery, claimed, err := model.ClaimTokenUsageAlert(ctx, userID, tokenID, rule, time.Now().Unix())
	if err != nil {
		common.SysError("API Key usage alert claim failed")
		return
	}
	if !claimed {
		if delivery.CooldownUntil > time.Now().Unix() {
			_ = common.RDB.Set(ctx, tokenAlertPrefix+"cooldown:"+id, "1", time.Until(time.Unix(delivery.CooldownUntil, 0))).Err()
		}
		return
	}
	// Durable claim precedes SMTP; losing this Redis write cannot permit a
	// duplicate because every sender also checks the database claim.
	_ = common.RDB.Set(ctx, tokenAlertPrefix+"cooldown:"+id, "1", time.Until(time.Unix(delivery.CooldownUntil, 0))).Err()
	cancel()
	sendCtx, sendCancel := context.WithTimeout(context.Background(), 30*time.Second)
	state, _ := sendTokenUsageAlertEmail(sendCtx, subject, user.Email, body)
	sendCancel()
	if state == "accepted" {
		state = "sent"
	}
	finishCtx, finishCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer finishCancel()
	next, err := model.FinishTokenUsageAlert(finishCtx, delivery, state, time.Now().Unix())
	if err != nil {
		common.SysError("API Key usage alert delivery result uncertain")
		return
	}
	if next > 0 {
		_ = common.RDB.Set(finishCtx, tokenAlertPrefix+"cooldown:"+id, "1", time.Until(time.Unix(next, 0))).Err()
		// Recovery also reloads retryable rows, including after Redis loss.
		_, _ = enqueueTokenUsageAlert.Run(finishCtx, common.RDB, []string{tokenAlertPrefix + "queue", tokenAlertPrefix + "cooldown:" + id}, id, next, "retry").Result()
	}
}

func RenderTokenUsageAlertEmail(config setting.TokenUsageAlertConfig, token model.Token, user model.User, usage model.TokenUsageAlertUsage, rule setting.TokenUsageAlertRule) (string, string, error) {
	if usage.Limit <= 0 || common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) {
		return "", "", errors.New("Invalid API Key usage alert settings")
	}
	percent := decimal.NewFromInt(usage.Used).Mul(decimal.NewFromInt(100)).Div(decimal.NewFromInt(usage.Limit)).StringFixed(2)
	scope := "累计额度"
	if usage.Scope == "period" {
		scope = "当前周期额度"
	}
	used, limit := decimal.NewFromInt(usage.Used).Div(decimal.NewFromFloat(common.QuotaPerUnit)).StringFixed(6), decimal.NewFromInt(usage.Limit).Div(decimal.NewFromFloat(common.QuotaPerUnit)).StringFixed(6)
	reset := "不适用（累计额度）"
	if usage.ResetAt > 0 {
		reset = time.Unix(usage.ResetAt, 0).In(model.RevenueTimezone).Format("2006-01-02 15:04:05") + "（北京时间）"
	}
	if config.Subject == "" {
		config.Subject = setting.TokenUsageAlertSubject
	}
	if config.Body == "" {
		config.Body = setting.TokenUsageAlertBody
	}
	vars := map[string]string{"system_name": common.SystemNameOrDefault(), "username": user.Username, "key_name": token.Name, "key_id": strconv.Itoa(token.Id), "scope": scope, "used_quota": used, "quota_limit": limit, "quota_unit": "quota / " + strconv.FormatFloat(common.QuotaPerUnit, 'f', -1, 64), "usage_percent": percent, "threshold_percent": strconv.Itoa(rule.Percent), "cooldown_days": strconv.Itoa(rule.CooldownDays), "reset_at": reset, "generated_at": time.Now().In(model.RevenueTimezone).Format("2006-01-02 15:04:05")}
	return RenderRevenueMail(model.RevenueAutomationConfig{Subject: config.Subject, Body: config.Body}, vars)
}

func TokenUsageAlertStatus(ctx context.Context) (map[string]any, error) {
	result := map[string]any{"redis_available": false, "pending": int64(0), "cached_keys": int64(0), "dropped_notices": tokenAlertDropped.Load()}
	result["builtin_template"] = map[string]string{"subject": setting.TokenUsageAlertSubject, "body": setting.TokenUsageAlertBody}
	result["placeholders"] = setting.TokenUsageAlertPlaceholders
	if common.RedisEnabled && common.RDB != nil {
		redisCtx, cancel := context.WithTimeout(ctx, time.Second)
		if common.RDB.Ping(redisCtx).Err() == nil {
			result["redis_available"] = true
			result["pending"], _ = common.RDB.ZCard(redisCtx, tokenAlertPrefix+"queue").Result()
			result["cached_keys"], _ = common.RDB.HLen(redisCtx, tokenAlertPrefix+"usage").Result()
		}
		cancel()
	}
	var states []struct {
		Status string `json:"status"`
		Count  int64  `json:"count"`
	}
	err := model.DB.WithContext(ctx).Model(&model.TokenUsageAlertDelivery{}).Select("status, COUNT(*) AS count").Group("status").Find(&states).Error
	if err != nil {
		return nil, errors.New("Unable to load API Key usage alert status")
	}
	result["deliveries"] = states
	return result, nil
}
