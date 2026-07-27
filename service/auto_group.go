package service

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type compiledAutoGroupRule struct {
	Raw        *model.AutoGroupRule
	Conditions []model.AutoGroupCondition
}

const groupChangeCooldownSeconds int64 = 60

type autoGroupEngine struct {
	started       atomic.Bool
	sweeping      atomic.Bool
	rules         atomic.Value // []*compiledAutoGroupRule
	enrolledUsers atomic.Value // map[int]struct{}
	evalCh        chan int
	stopCh        chan struct{}
}

var globalAutoGroupEngine = &autoGroupEngine{}

func StartAutoGroupEngine() {
	globalAutoGroupEngine.start()
}

func ReloadAutoGroupRules() {
	if err := globalAutoGroupEngine.reloadRules(); err != nil {
		common.SysError("auto group reload rules failed: " + err.Error())
	}
}

func ReloadAutoGroupEnrollments() {
	if err := globalAutoGroupEngine.reloadEnrolledUsers(); err != nil {
		common.SysError("auto group reload enrollments failed: " + err.Error())
	}
}

func NotifyAutoGroupEvaluation(userId int) {
	if !globalAutoGroupEngine.started.Load() {
		return
	}
	if !globalAutoGroupEngine.isUserEnrolled(userId) {
		return
	}
	select {
	case globalAutoGroupEngine.evalCh <- userId:
	default:
	}
}

func TriggerAutoGroupSweep() {
	if !globalAutoGroupEngine.started.Load() {
		return
	}
	go globalAutoGroupEngine.sweepAll()
}

func (e *autoGroupEngine) start() {
	if e.started.Load() {
		return
	}
	e.evalCh = make(chan int, 1000)
	e.stopCh = make(chan struct{})

	if err := e.reloadRules(); err != nil {
		common.SysError("auto group engine reload rules failed: " + err.Error())
	}
	if err := e.reloadEnrolledUsers(); err != nil {
		common.SysError("auto group engine reload enrollments failed: " + err.Error())
	}

	go e.runWorker()
	e.started.Store(true)
	common.SysLog("auto group engine started")

	if common.IsMasterNode {
		go e.sweepAll()
	}
}

func (e *autoGroupEngine) reloadRules() error {
	rules, err := model.ListEnabledAutoGroupRules()
	if err != nil {
		return err
	}
	compiled := make([]*compiledAutoGroupRule, 0, len(rules))
	for _, rule := range rules {
		conditions := rule.ParsedConditions()
		if len(conditions) == 0 {
			common.SysLog(fmt.Sprintf("auto group rule %q skipped: no conditions", rule.Name))
			continue
		}
		if strings.TrimSpace(rule.TargetGroup) == "" {
			common.SysLog(fmt.Sprintf("auto group rule %q skipped: no target group", rule.Name))
			continue
		}
		compiled = append(compiled, &compiledAutoGroupRule{
			Raw:        rule,
			Conditions: conditions,
		})
	}
	e.rules.Store(compiled)
	return nil
}

func (e *autoGroupEngine) reloadEnrolledUsers() error {
	userIds, err := model.ListAllEnrollmentUserIds()
	if err != nil {
		return err
	}
	set := make(map[int]struct{}, len(userIds))
	for _, id := range userIds {
		set[id] = struct{}{}
	}
	e.enrolledUsers.Store(set)
	return nil
}

func (e *autoGroupEngine) isUserEnrolled(userId int) bool {
	raw := e.enrolledUsers.Load()
	if raw == nil {
		return false
	}
	users, ok := raw.(map[int]struct{})
	if !ok {
		return false
	}
	_, enrolled := users[userId]
	return enrolled
}

func (e *autoGroupEngine) currentRules() []*compiledAutoGroupRule {
	if raw := e.rules.Load(); raw != nil {
		if rules, ok := raw.([]*compiledAutoGroupRule); ok {
			return rules
		}
	}
	return nil
}

func (e *autoGroupEngine) runWorker() {
	reloadTicker := time.NewTicker(1 * time.Minute)
	sweepTicker := time.NewTicker(5 * time.Minute)
	defer reloadTicker.Stop()
	defer sweepTicker.Stop()

	pending := make(map[int]struct{})
	var debounceTimer *time.Timer
	var debounceCh <-chan time.Time

	for {
		select {
		case userId := <-e.evalCh:
			pending[userId] = struct{}{}
			if debounceTimer == nil {
				debounceTimer = time.NewTimer(30 * time.Second)
				debounceCh = debounceTimer.C
			}

		case <-debounceCh:
			e.evaluateBatch(pending)
			pending = make(map[int]struct{})
			debounceTimer = nil
			debounceCh = nil

		case <-reloadTicker.C:
			if err := e.reloadRules(); err != nil {
				common.SysError("auto group periodic reload rules failed: " + err.Error())
			}
			if err := e.reloadEnrolledUsers(); err != nil {
				common.SysError("auto group periodic reload enrollments failed: " + err.Error())
			}

		case <-sweepTicker.C:
			e.sweepAll()

		case <-e.stopCh:
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return
		}
	}
}

func (e *autoGroupEngine) evaluateBatch(userIds map[int]struct{}) {
	rules := e.currentRules()
	if len(rules) == 0 {
		return
	}
	for userId := range userIds {
		e.evaluateUser(userId, rules)
	}
}

func (e *autoGroupEngine) sweepAll() {
	if !e.sweeping.CompareAndSwap(false, true) {
		common.SysLog("auto group sweep skipped: already running")
		return
	}
	defer e.sweeping.Store(false)

	rules := e.currentRules()
	enrollments, err := model.ListAllEnrollments()
	if err != nil {
		common.SysError("auto group sweep list enrollments failed: " + err.Error())
		return
	}
	for _, enrollment := range enrollments {
		e.evaluateUser(enrollment.UserId, rules)
	}
}

func (e *autoGroupEngine) evaluateUser(userId int, rules []*compiledAutoGroupRule) {
	enrollment, err := model.GetEnrollmentByUserId(userId)
	if err != nil {
		return
	}

	now := common.GetTimestamp()
	if enrollment.CurrentRuleId != 0 && now-enrollment.UpdatedAt < groupChangeCooldownSeconds {
		return
	}

	hasSubscription, err := model.HasActiveSubscriptionWithUpgrade(userId)
	if err != nil {
		common.SysError(fmt.Sprintf("auto group check subscription for user %d failed: %v", userId, err))
		return
	}
	if hasSubscription {
		return
	}

	metricsCache := make(map[string]float64)
	strMetricsCache := make(map[string]string)
	var matchedRule *compiledAutoGroupRule

	for _, rule := range rules {
		if e.matchConditions(userId, rule, metricsCache, strMetricsCache) {
			matchedRule = rule
			break
		}
	}

	if matchedRule != nil {
		targetGroup := matchedRule.Raw.TargetGroup
		if targetGroup != enrollment.CurrentGroup {
			if err := model.TransactionalGroupChange(userId, targetGroup, matchedRule.Raw.Id); err != nil {
				common.SysError(fmt.Sprintf("auto group update user %d group to %s failed: %v", userId, targetGroup, err))
				return
			}
			if err := model.RefreshUserGroupCache(userId); err != nil {
				common.SysError(fmt.Sprintf("auto group refresh user %d group cache failed: %v", userId, err))
			}
			common.SysLog(fmt.Sprintf("auto group: user %d group changed %s -> %s (rule: %s)", userId, enrollment.CurrentGroup, targetGroup, matchedRule.Raw.Name))
		}
	} else {
		if enrollment.CurrentRuleId != 0 {
			if err := model.TransactionalGroupChange(userId, enrollment.OriginalGroup, 0); err != nil {
				common.SysError(fmt.Sprintf("auto group revert user %d group to %s failed: %v", userId, enrollment.OriginalGroup, err))
				return
			}
			if err := model.RefreshUserGroupCache(userId); err != nil {
				common.SysError(fmt.Sprintf("auto group refresh user %d group cache failed: %v", userId, err))
			}
			common.SysLog(fmt.Sprintf("auto group: user %d group reverted %s -> %s (no matching rule)", userId, enrollment.CurrentGroup, enrollment.OriginalGroup))
		}
	}
}

func (e *autoGroupEngine) matchConditions(userId int, rule *compiledAutoGroupRule, metricsCache map[string]float64, strMetricsCache map[string]string) bool {
	isAll := rule.Raw.MatchMode != "any"

	for _, cond := range rule.Conditions {
		var matched bool

		if isStringMetric(cond.Metric) {
			strValue, ok := strMetricsCache[cond.Metric]
			if !ok {
				var err error
				strValue, err = e.computeStringMetric(userId, cond.Metric)
				if err != nil {
					common.SysError(fmt.Sprintf("auto group compute string metric %s for user %d failed: %v", cond.Metric, userId, err))
					if isAll {
						return false
					}
					continue
				}
				strMetricsCache[cond.Metric] = strValue
			}
			matched = compareString(strValue, cond.Op, cond.ValueStr)
		} else {
			metricKey := cond.Metric
			if cond.Param > 0 {
				metricKey = fmt.Sprintf("%s_%d", cond.Metric, cond.Param)
			}

			value, ok := metricsCache[metricKey]
			if !ok {
				var err error
				value, err = e.computeMetric(userId, cond.Metric, cond.Param)
				if err != nil {
					common.SysError(fmt.Sprintf("auto group compute metric %s for user %d failed: %v", cond.Metric, userId, err))
					if isAll {
						return false
					}
					continue
				}
				metricsCache[metricKey] = value
			}
			matched = compareValue(value, cond.Op, cond.Value)
		}

		if isAll && !matched {
			return false
		}
		if !isAll && matched {
			return true
		}
	}

	return isAll
}

func (e *autoGroupEngine) computeMetric(userId int, metric string, param int) (float64, error) {
	switch metric {
	case "total_spend":
		user, err := model.GetUserById(userId, false)
		if err != nil {
			return 0, err
		}
		return float64(user.UsedQuota) / common.QuotaPerUnit, nil

	case "recent_spend":
		if param <= 0 {
			param = 24
		}
		return model.GetUserRecentSpend(userId, param)

	case "yesterday_spend":
		return model.GetUserYesterdaySpend(userId)

	case "total_topup":
		quota, err := model.GetUserNetTopupQuota(userId)
		if err != nil {
			return 0, err
		}
		return float64(quota) / common.QuotaPerUnit, nil

	case "recent_topup":
		if param <= 0 {
			param = 24
		}
		quota, err := model.GetUserRecentTopupQuota(userId, param)
		if err != nil {
			return 0, err
		}
		return float64(quota) / common.QuotaPerUnit, nil

	case "yesterday_topup":
		quota, err := model.GetUserYesterdayTopupQuota(userId)
		if err != nil {
			return 0, err
		}
		return float64(quota) / common.QuotaPerUnit, nil

	case "total_request_count":
		user, err := model.GetUserById(userId, false)
		if err != nil {
			return 0, err
		}
		return float64(user.RequestCount), nil

	case "recent_request_count":
		if param <= 0 {
			param = 24
		}
		count, err := model.GetUserRecentRequestCount(userId, param)
		if err != nil {
			return 0, err
		}
		return float64(count), nil

	default:
		return 0, fmt.Errorf("unknown metric: %s", metric)
	}
}

func compareValue(actual float64, op string, target float64) bool {
	switch op {
	case ">=":
		return actual >= target
	case ">":
		return actual > target
	case "<=":
		return actual <= target
	case "<":
		return actual < target
	case "==":
		return actual == target
	case "!=":
		return actual != target
	default:
		return false
	}
}

func isStringMetric(metric string) bool {
	return metric == "current_group"
}

func compareString(actual, op, target string) bool {
	switch op {
	case "==":
		return actual == target
	case "!=":
		return actual != target
	default:
		return false
	}
}

func (e *autoGroupEngine) computeStringMetric(userId int, metric string) (string, error) {
	switch metric {
	case "current_group":
		user, err := model.GetUserById(userId, false)
		if err != nil {
			return "", err
		}
		return user.Group, nil
	default:
		return "", fmt.Errorf("unknown string metric: %s", metric)
	}
}
