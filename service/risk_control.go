package service

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/requestip"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type localRiskGateCacheEntry struct {
	Decision  *types.RiskDecision
	ExpiresAt int64
}

type riskControlCenter struct {
	started   atomic.Bool
	store     riskMetricStore
	queue     chan *RiskEvent
	rules     atomic.Value
	cache     sync.Map
	dropCount atomic.Int64
}

var globalRiskCenter = &riskControlCenter{}

func StartRiskControlCenter() {
	globalRiskCenter.start()
}

func ReloadRiskRules() error {
	return globalRiskCenter.reloadRules()
}

func GetRiskControlConfig() *operation_setting.RiskControlSetting {
	return operation_setting.GetRiskControlSetting()
}

func isRiskControlCollectEnabled(cfg *operation_setting.RiskControlSetting) bool {
	return cfg != nil && cfg.Enabled && cfg.Mode != operation_setting.RiskControlModeOff
}

func RiskControlBeforeRelay(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	decision := globalRiskCenter.beforeRelay(c, info)
	if decision == nil {
		return nil
	}
	if info != nil {
		info.RiskAudit = &types.RiskAudit{
			FinalDecision: decision.Decision,
			FinalReason:   decision.Reason,
		}
		if decision.Scope == RiskSubjectTypeToken {
			info.RiskAudit.TokenDecision = decision
		} else {
			info.RiskAudit.UserDecision = decision
		}
	}
	message := normalizeRiskMessage(decision.ResponseMessage)
	statusCode := decision.StatusCode
	if statusCode <= 0 {
		statusCode = operation_setting.GetRiskControlSetting().DefaultStatusCode
	}
	return types.NewErrorWithStatusCode(
		errors.New(message),
		types.ErrorCodeRiskControlBlocked,
		statusCode,
		types.ErrOptionWithSkipRetry(),
	)
}

func RiskControlAfterRelay(c *gin.Context, info *relaycommon.RelayInfo, relayErr *types.NewAPIError) {
	globalRiskCenter.afterRelay(c, info, relayErr)
}

func GetRiskOverview() (map[string]any, error) {
	observed, err := model.CountRiskSubjectSnapshotsByStatus(RiskStatusObserve)
	if err != nil {
		return nil, err
	}
	blocked, err := model.CountRiskSubjectSnapshotsByStatus(RiskStatusBlocked)
	if err != nil {
		return nil, err
	}
	highRisk, err := model.CountHighRiskSubjectSnapshots(60)
	if err != nil {
		return nil, err
	}
	ruleCount, err := model.CountRiskRules()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"observed_subjects":  observed,
		"blocked_subjects":   blocked,
		"high_risk_subjects": highRisk,
		"rule_count":         ruleCount,
		"queue_dropped":      globalRiskCenter.dropCount.Load(),
		"mode":               operation_setting.GetRiskControlSetting().Mode,
		"enabled":            operation_setting.GetRiskControlSetting().Enabled,
	}, nil
}

func ListRiskRules() ([]*model.RiskRule, error) {
	return model.ListRiskRules()
}

func CreateRiskRule(rule *model.RiskRule) error {
	if err := validateRiskRule(rule); err != nil {
		return err
	}
	if err := model.CreateRiskRule(rule); err != nil {
		return err
	}
	return ReloadRiskRules()
}

func UpdateRiskRule(rule *model.RiskRule) error {
	if err := validateRiskRule(rule); err != nil {
		return err
	}
	if err := model.UpdateRiskRule(rule); err != nil {
		return err
	}
	return ReloadRiskRules()
}

func DeleteRiskRule(id int) error {
	if err := model.DeleteRiskRule(id); err != nil {
		return err
	}
	return ReloadRiskRules()
}

func ListRiskSubjectSnapshots(query model.RiskSubjectQuery, pageInfo *common.PageInfo) ([]*model.RiskSubjectSnapshot, int64, error) {
	return model.ListRiskSubjectSnapshots(query, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
}

func ListRiskIncidents(query model.RiskIncidentQuery, pageInfo *common.PageInfo) ([]*model.RiskIncident, int64, error) {
	return model.ListRiskIncidents(query, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
}

func UnblockRiskSubject(scope string, subjectID int, operator string) error {
	if scope != RiskSubjectTypeToken && scope != RiskSubjectTypeUser {
		return errors.New("invalid subject scope")
	}
	if subjectID <= 0 {
		return errors.New("invalid subject id")
	}
	if err := globalRiskCenter.store.ClearBlock(scope, subjectID); err != nil {
		return err
	}
	globalRiskCenter.cache.Delete(riskMemoryKey(scope, subjectID))
	snapshot, err := model.GetRiskSubjectSnapshot(scope, subjectID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	now := common.GetTimestamp()
	if snapshot != nil {
		status := RiskStatusNormal
		if snapshot.RiskScore > 0 {
			status = RiskStatusObserve
		}
		snapshot.Status = status
		snapshot.BlockUntil = 0
		snapshot.RecoverAt = now
		snapshot.LastAction = RiskActionManual
		snapshot.LastDecision = RiskDecisionAllow
		snapshot.LastReason = "管理员手动解除"
		snapshot.LastIncidentAt = now
		snapshot.LastEvaluatedAt = now
		if err = model.UpsertRiskSubjectSnapshot(snapshot); err != nil {
			return err
		}
		incident := &model.RiskIncident{
			CreatedAt:          now,
			SubjectType:        scope,
			SubjectID:          subjectID,
			UserID:             snapshot.UserID,
			TokenID:            snapshot.TokenID,
			Username:           snapshot.Username,
			TokenName:          snapshot.TokenName,
			TokenMaskedKey:     snapshot.TokenMaskedKey,
			Action:             RiskActionManual,
			Decision:           RiskDecisionAllow,
			Status:             status,
			ResponseStatusCode: 0,
			ResponseMessage:    "管理员手动解除",
			AutoRecover:        false,
			RecoverAt:          now,
			ResolvedAt:         now,
			RiskScore:          snapshot.RiskScore,
			Reason:             fmt.Sprintf("operator=%s", operator),
		}
		return model.CreateRiskIncident(incident)
	}
	return nil
}

func (r *riskControlCenter) start() {
	if r.started.Load() {
		return
	}
	r.store = newRiskMetricStore()
	cfg := operation_setting.GetRiskControlSetting()
	r.queue = make(chan *RiskEvent, cfg.EventQueueSize)
	if err := r.reloadRules(); err != nil {
		common.SysError("risk control reload rules failed: " + err.Error())
	}
	if common.IsMasterNode {
		if err := seedDefaultRiskRules(); err != nil {
			common.SysError("risk control seed rules failed: " + err.Error())
		}
		if err := r.reloadRules(); err != nil {
			common.SysError("risk control reload rules after seed failed: " + err.Error())
		}
	}
	workerCount := cfg.WorkerCount
	if workerCount <= 0 {
		workerCount = 2
	}
	for i := 0; i < workerCount; i++ {
		go r.runWorker()
	}
	if common.IsMasterNode {
		go r.runRecoveryLoop()
	}
	r.started.Store(true)
}

func (r *riskControlCenter) reloadRules() error {
	rules, err := model.ListEnabledRiskRules()
	if err != nil {
		return err
	}
	compiled := make([]*compiledRiskRule, 0, len(rules))
	for _, rule := range rules {
		conditions := make([]types.RiskCondition, 0)
		if strings.TrimSpace(rule.Conditions) != "" {
			if err = common.UnmarshalJsonStr(rule.Conditions, &conditions); err != nil {
				return fmt.Errorf("rule %s conditions invalid: %w", rule.Name, err)
			}
		}
		compiled = append(compiled, &compiledRiskRule{
			Raw:        rule,
			Conditions: conditions,
		})
	}
	r.rules.Store(compiled)
	return nil
}

func (r *riskControlCenter) currentRules() []*compiledRiskRule {
	if raw := r.rules.Load(); raw != nil {
		if rules, ok := raw.([]*compiledRiskRule); ok {
			return rules
		}
	}
	return nil
}

func (r *riskControlCenter) beforeRelay(c *gin.Context, info *relaycommon.RelayInfo) *types.RiskDecision {
	cfg := operation_setting.GetRiskControlSetting()
	if info == nil || !isRiskControlCollectEnabled(cfg) {
		return nil
	}
	if cfg.Mode != operation_setting.RiskControlModeEnforce {
		if info != nil {
			r.enqueueStart(c, info)
		}
		return nil
	}
	if decision := r.getCachedDecision(RiskSubjectTypeUser, info.UserId); decision != nil {
		return decision
	}
	if decision := r.getCachedDecision(RiskSubjectTypeToken, info.TokenId); decision != nil {
		return decision
	}
	r.enqueueStart(c, info)
	return nil
}

func (r *riskControlCenter) afterRelay(c *gin.Context, info *relaycommon.RelayInfo, relayErr *types.NewAPIError) {
	if info == nil || !isRiskControlCollectEnabled(operation_setting.GetRiskControlSetting()) {
		return
	}
	now := common.GetTimestamp()
	statusCode := 0
	if relayErr != nil {
		statusCode = relayErr.StatusCode
	} else if c != nil && c.Writer != nil {
		statusCode = c.Writer.Status()
	}
	event := r.buildEvent(c, info, RiskEventTypeFinish, now, statusCode)
	r.enqueueEvent(event)
}

func (r *riskControlCenter) buildEvent(c *gin.Context, info *relaycommon.RelayInfo, eventType string, now int64, statusCode int) *RiskEvent {
	if info == nil {
		return nil
	}
	clientIP := ""
	userAgent := ""
	requestPath := info.RequestURLPath
	username := ""
	tokenName := ""
	if c != nil && c.Request != nil {
		clientIP = requestip.GetClientIP(c)
		userAgent = c.GetHeader("User-Agent")
		username = c.GetString("username")
		tokenName = c.GetString("token_name")
		if c.Request.URL != nil && c.Request.URL.Path != "" {
			requestPath = c.Request.URL.Path
		}
	}
	return &RiskEvent{
		Type:           eventType,
		OccurAt:        now,
		RequestID:      info.RequestId,
		RequestPath:    requestPath,
		UserID:         info.UserId,
		Username:       username,
		TokenID:        info.TokenId,
		TokenName:      tokenName,
		TokenMaskedKey: model.MaskTokenKey(info.TokenKey),
		Group:          info.UsingGroup,
		ClientIPHash:   common.GenerateHMAC(clientIP),
		UserAgentHash:  common.GenerateHMAC(normalizeUserAgent(userAgent)),
		StatusCode:     statusCode,
	}
}

func (r *riskControlCenter) enqueueStart(c *gin.Context, info *relaycommon.RelayInfo) {
	event := r.buildEvent(c, info, RiskEventTypeStart, common.GetTimestamp(), 0)
	r.enqueueEvent(event)
}

func (r *riskControlCenter) enqueueEvent(event *RiskEvent) {
	if event == nil || r.queue == nil || !isRiskControlCollectEnabled(operation_setting.GetRiskControlSetting()) {
		return
	}
	select {
	case r.queue <- event:
	default:
		r.dropCount.Add(1)
		if r.dropCount.Load()%100 == 1 {
			common.SysLog("risk control event queue is full, dropping events")
		}
	}
}

func (r *riskControlCenter) runWorker() {
	for event := range r.queue {
		if event == nil {
			continue
		}
		switch event.Type {
		case RiskEventTypeStart:
			if err := r.handleStartEvent(event); err != nil {
				common.SysError("risk control handle start event failed: " + err.Error())
			}
		case RiskEventTypeFinish:
			if err := r.handleFinishEvent(event); err != nil {
				common.SysError("risk control handle finish event failed: " + err.Error())
			}
		}
	}
}

func (r *riskControlCenter) handleStartEvent(event *RiskEvent) error {
	now := time.Unix(event.OccurAt, 0)
	tokenMetrics, err := r.store.RecordStart(RiskSubjectTypeToken, event.TokenID, event.ClientIPHash, event.UserAgentHash, now)
	if err != nil {
		return err
	}
	userMetrics, err := r.store.RecordStart(RiskSubjectTypeUser, event.UserID, event.ClientIPHash, "", now)
	if err != nil {
		return err
	}
	tokenMetrics.RuleHitCount24H, _ = r.store.GetRuleHitCount(RiskSubjectTypeToken, event.TokenID, now)
	userMetrics.RuleHitCount24H, _ = r.store.GetRuleHitCount(RiskSubjectTypeUser, event.UserID, now)

	tokenDecision, tokenMatched, err := r.evaluateAndPersistSubject(event, RiskSubjectTypeToken, event.TokenID, tokenMetrics)
	if err != nil {
		return err
	}
	userDecision, userMatched, err := r.evaluateAndPersistSubject(event, RiskSubjectTypeUser, event.UserID, userMetrics)
	if err != nil {
		return err
	}

	if tokenDecision != nil && tokenDecision.Decision == RiskDecisionBlock && len(tokenMatched) > 0 {
		r.updateGateCache(tokenDecision)
	}
	if userDecision != nil && userDecision.Decision == RiskDecisionBlock && len(userMatched) > 0 {
		r.updateGateCache(userDecision)
	}
	return nil
}

func (r *riskControlCenter) handleFinishEvent(event *RiskEvent) error {
	now := time.Unix(event.OccurAt, 0)
	if _, err := r.store.RecordFinish(RiskSubjectTypeToken, event.TokenID, now); err != nil {
		return err
	}
	if _, err := r.store.RecordFinish(RiskSubjectTypeUser, event.UserID, now); err != nil {
		return err
	}
	return nil
}

func (r *riskControlCenter) evaluateAndPersistSubject(event *RiskEvent, scope string, subjectID int, metrics types.RiskMetrics) (*types.RiskDecision, []*compiledRiskRule, error) {
	if subjectID <= 0 {
		return nil, nil, nil
	}
	rules := r.currentRules()
	matched := evaluateRiskRules(rules, scope, metrics)
	if len(matched) > 0 {
		hitCount, hitErr := r.store.IncrementRuleHit(scope, subjectID, time.Unix(event.OccurAt, 0))
		if hitErr == nil {
			metrics.RuleHitCount24H = hitCount
		}
	}
	metrics.RiskScore = computeRiskScore(metrics, matched)
	decision := buildRiskDecision(scope, subjectID, matched, metrics)
	previous, err := model.GetRiskSubjectSnapshot(scope, subjectID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	}
	if decision != nil && decision.Action == RiskActionBlock {
		if decision.AutoRecover {
			decision.BlockUntil = time.Unix(event.OccurAt, 0).Add(time.Duration(decision.RecoverAfterSecond) * time.Second).Unix()
		}
		if operation_setting.GetRiskControlSetting().Mode == operation_setting.RiskControlModeEnforce {
			if err = r.store.SetBlock(scope, subjectID, decision); err != nil {
				common.SysError("risk control set block failed: " + err.Error())
			}
		}
	}
	snapshot := buildRiskSubjectSnapshot(event, scope, subjectID, metrics, decision, matched, previous)
	if err = model.UpsertRiskSubjectSnapshot(snapshot); err != nil {
		return nil, nil, err
	}
	if shouldCreateRiskIncident(previous, snapshot, decision) {
		incident := buildRiskIncident(event, snapshot, decision, matched)
		if incident != nil {
			if err = model.CreateRiskIncident(incident); err != nil {
				return nil, nil, err
			}
		}
	}
	return decision, matched, nil
}

func evaluateRiskRules(rules []*compiledRiskRule, scope string, metrics types.RiskMetrics) []*compiledRiskRule {
	if len(rules) == 0 {
		return nil
	}
	matched := make([]*compiledRiskRule, 0)
	for _, rule := range rules {
		if rule == nil || rule.Raw == nil || !rule.Raw.Enabled || rule.Raw.Scope != scope {
			continue
		}
		if matchesRiskRule(rule, metrics) {
			matched = append(matched, rule)
		}
	}
	return matched
}

func matchesRiskRule(rule *compiledRiskRule, metrics types.RiskMetrics) bool {
	if rule == nil || rule.Raw == nil {
		return false
	}
	if len(rule.Conditions) == 0 {
		return false
	}
	matchMode := strings.ToLower(strings.TrimSpace(rule.Raw.MatchMode))
	if matchMode == "" {
		matchMode = "all"
	}
	matchedCount := 0
	for _, condition := range rule.Conditions {
		actual := getMetricValue(metrics, condition.Metric)
		result := compareRiskMetric(actual, condition.Op, condition.Value)
		if result {
			matchedCount++
		}
		if matchMode == "any" && result {
			return true
		}
		if matchMode == "all" && !result {
			return false
		}
	}
	if matchMode == "any" {
		return matchedCount > 0
	}
	return matchedCount == len(rule.Conditions)
}

func getMetricValue(metrics types.RiskMetrics, metric string) float64 {
	switch metric {
	case "distinct_ip_10m":
		return float64(metrics.DistinctIP10M)
	case "distinct_ip_1h":
		return float64(metrics.DistinctIP1H)
	case "distinct_ua_10m":
		return float64(metrics.DistinctUA10M)
	case "request_count_1m", "req_1m":
		return float64(metrics.RequestCount1M)
	case "request_count_10m", "req_10m":
		return float64(metrics.RequestCount10M)
	case "inflight_now":
		return float64(metrics.InflightNow)
	case "rule_hit_count_24h":
		return float64(metrics.RuleHitCount24H)
	case "risk_score":
		return float64(metrics.RiskScore)
	default:
		return 0
	}
}

func compareRiskMetric(actual float64, op string, expected float64) bool {
	switch op {
	case ">", "gt":
		return actual > expected
	case ">=", "gte":
		return actual >= expected
	case "<", "lt":
		return actual < expected
	case "<=", "lte":
		return actual <= expected
	case "==", "=":
		return actual == expected
	case "!=", "<>":
		return actual != expected
	default:
		return false
	}
}

func computeRiskScore(metrics types.RiskMetrics, matched []*compiledRiskRule) int {
	score := 0
	score += minInt(metrics.DistinctIP10M*8, 40)
	score += minInt(metrics.DistinctIP1H*2, 10)
	score += minInt(metrics.InflightNow*4, 20)
	score += minInt(metrics.RequestCount1M/5, 15)
	score += minInt(metrics.DistinctUA10M*3, 10)
	score += minInt(metrics.RuleHitCount24H*4, 15)
	for _, rule := range matched {
		if rule != nil && rule.Raw != nil && rule.Raw.ScoreWeight > 0 {
			score += rule.Raw.ScoreWeight
		}
	}
	if score > 100 {
		return 100
	}
	if score < 0 {
		return 0
	}
	return score
}

func buildRiskDecision(scope string, subjectID int, matched []*compiledRiskRule, metrics types.RiskMetrics) *types.RiskDecision {
	if subjectID <= 0 {
		return nil
	}
	cfg := operation_setting.GetRiskControlSetting()
	if len(matched) == 0 {
		return &types.RiskDecision{
			Scope:           scope,
			SubjectID:       subjectID,
			Decision:        RiskDecisionAllow,
			Action:          RiskDecisionAllow,
			Status:          RiskStatusNormal,
			StatusCode:      http.StatusOK,
			ResponseMessage: "",
			RiskScore:       metrics.RiskScore,
			Metrics:         metrics,
		}
	}
	primary := matched[0]
	responseStatusCode := cfg.DefaultStatusCode
	responseMessage := cfg.DefaultResponseMessage
	autoRecover := false
	recoverMode := cfg.DefaultRecoverMode
	recoverAfter := cfg.DefaultRecoverAfterSecs
	action := RiskActionObserve
	decisionType := RiskDecisionObserve
	status := RiskStatusObserve
	matchedNames := make([]string, 0, len(matched))
	for _, rule := range matched {
		if rule == nil || rule.Raw == nil {
			continue
		}
		matchedNames = append(matchedNames, rule.Raw.Name)
		if action != RiskActionBlock && strings.EqualFold(rule.Raw.Action, RiskActionBlock) {
			action = RiskActionBlock
			decisionType = RiskDecisionBlock
			status = RiskStatusBlocked
			primary = rule
		}
	}
	if primary != nil && primary.Raw != nil {
		if primary.Raw.ResponseStatusCode > 0 {
			responseStatusCode = primary.Raw.ResponseStatusCode
		}
		if strings.TrimSpace(primary.Raw.ResponseMessage) != "" {
			responseMessage = primary.Raw.ResponseMessage
		}
		autoRecover = primary.Raw.AutoRecover
		if strings.TrimSpace(primary.Raw.RecoverMode) != "" {
			recoverMode = primary.Raw.RecoverMode
		}
		if primary.Raw.RecoverAfterSeconds > 0 {
			recoverAfter = primary.Raw.RecoverAfterSeconds
		}
		if action == RiskActionBlock && !primary.Raw.AutoBlock {
			action = RiskActionObserve
			decisionType = RiskDecisionObserve
			status = RiskStatusObserve
		}
	}
	if action == RiskActionBlock && cfg.Mode != operation_setting.RiskControlModeEnforce {
		action = RiskActionObserve
		decisionType = RiskDecisionObserve
		status = RiskStatusObserve
	}
	return &types.RiskDecision{
		Scope:              scope,
		SubjectID:          subjectID,
		Decision:           decisionType,
		Action:             action,
		Status:             status,
		RuleID:             primary.Raw.Id,
		RuleName:           primary.Raw.Name,
		Detector:           primary.Raw.Detector,
		Reason:             fmt.Sprintf("matched_rules=%s", strings.Join(matchedNames, ",")),
		MatchedRules:       matchedNames,
		StatusCode:         responseStatusCode,
		ResponseMessage:    responseMessage,
		AutoRecover:        autoRecover,
		RecoverMode:        recoverMode,
		RecoverAfterSecond: recoverAfter,
		RiskScore:          metrics.RiskScore,
		Metrics:            metrics,
	}
}

func buildRiskSubjectSnapshot(event *RiskEvent, scope string, subjectID int, metrics types.RiskMetrics, decision *types.RiskDecision, matched []*compiledRiskRule, previous *model.RiskSubjectSnapshot) *model.RiskSubjectSnapshot {
	status := RiskStatusNormal
	lastDecision := RiskDecisionAllow
	lastAction := RiskDecisionAllow
	lastRule := ""
	lastReason := ""
	blockUntil := int64(0)
	recoverAt := int64(0)
	autoRecover := false
	statusCode := 0
	if decision != nil {
		status = decision.Status
		lastDecision = decision.Decision
		lastAction = decision.Action
		lastRule = decision.RuleName
		lastReason = decision.Reason
		blockUntil = decision.BlockUntil
		if decision.AutoRecover && decision.BlockUntil > 0 {
			recoverAt = decision.BlockUntil
		}
		autoRecover = decision.AutoRecover
		statusCode = decision.StatusCode
	}
	if status == RiskStatusNormal && metrics.RiskScore >= 30 {
		status = RiskStatusObserve
	}
	activeRuleNames := make([]string, 0, len(matched))
	for _, rule := range matched {
		if rule != nil && rule.Raw != nil {
			activeRuleNames = append(activeRuleNames, rule.Raw.Name)
		}
	}
	snapshot := &model.RiskSubjectSnapshot{
		SubjectType:       scope,
		SubjectID:         subjectID,
		UserID:            event.UserID,
		TokenID:           event.TokenID,
		Username:          event.Username,
		TokenName:         event.TokenName,
		TokenMaskedKey:    event.TokenMaskedKey,
		Group:             event.Group,
		Status:            status,
		RiskScore:         metrics.RiskScore,
		DistinctIP10M:     metrics.DistinctIP10M,
		DistinctIP1H:      metrics.DistinctIP1H,
		DistinctUA10M:     metrics.DistinctUA10M,
		RequestCount1M:    metrics.RequestCount1M,
		RequestCount10M:   metrics.RequestCount10M,
		InflightNow:       metrics.InflightNow,
		RuleHitCount24H:   metrics.RuleHitCount24H,
		ActiveRuleNames:   encodeRiskJSON(activeRuleNames),
		LastRuleName:      lastRule,
		LastDecision:      lastDecision,
		LastAction:        lastAction,
		LastReason:        lastReason,
		LastRequestPath:   event.RequestPath,
		LastStatusCode:    statusCode,
		BlockUntil:        blockUntil,
		RecoverAt:         recoverAt,
		AutoRecover:       autoRecover,
		LastSeenAt:        event.OccurAt,
		LastEvaluatedAt:   event.OccurAt,
		LastIncidentAt:    event.OccurAt,
		SnapshotExtraData: encodeRiskJSON(map[string]any{"request_id": event.RequestID}),
	}
	if previous != nil && previous.Id > 0 {
		snapshot.Id = previous.Id
		if snapshot.Username == "" {
			snapshot.Username = previous.Username
		}
		if snapshot.TokenName == "" {
			snapshot.TokenName = previous.TokenName
		}
		if snapshot.TokenMaskedKey == "" {
			snapshot.TokenMaskedKey = previous.TokenMaskedKey
		}
	}
	return snapshot
}

func shouldCreateRiskIncident(previous *model.RiskSubjectSnapshot, current *model.RiskSubjectSnapshot, decision *types.RiskDecision) bool {
	if current == nil {
		return false
	}
	if previous == nil {
		return current.Status != RiskStatusNormal
	}
	if current.Status == RiskStatusBlocked && previous.Status != RiskStatusBlocked {
		return true
	}
	if current.Status == RiskStatusObserve && previous.Status == RiskStatusNormal {
		return true
	}
	if decision != nil && decision.Action == RiskActionBlock && current.BlockUntil != previous.BlockUntil {
		return true
	}
	return false
}

func buildRiskIncident(event *RiskEvent, snapshot *model.RiskSubjectSnapshot, decision *types.RiskDecision, matched []*compiledRiskRule) *model.RiskIncident {
	if snapshot == nil || decision == nil || decision.Decision == RiskDecisionAllow {
		return nil
	}
	return &model.RiskIncident{
		CreatedAt:          event.OccurAt,
		SubjectType:        snapshot.SubjectType,
		SubjectID:          snapshot.SubjectID,
		UserID:             snapshot.UserID,
		TokenID:            snapshot.TokenID,
		Username:           snapshot.Username,
		TokenName:          snapshot.TokenName,
		TokenMaskedKey:     snapshot.TokenMaskedKey,
		RuleID:             decision.RuleID,
		RuleName:           decision.RuleName,
		Detector:           decision.Detector,
		Action:             decision.Action,
		Decision:           decision.Decision,
		Status:             snapshot.Status,
		ResponseStatusCode: decision.StatusCode,
		ResponseMessage:    decision.ResponseMessage,
		AutoRecover:        decision.AutoRecover,
		RecoverAt:          snapshot.RecoverAt,
		RequestID:          event.RequestID,
		RequestPath:        event.RequestPath,
		RiskScore:          snapshot.RiskScore,
		Reason:             decision.Reason,
		Snapshot: encodeRiskJSON(map[string]any{
			"metrics":       decision.Metrics,
			"matched_rules": decision.MatchedRules,
		}),
	}
}

func (r *riskControlCenter) getCachedDecision(scope string, subjectID int) *types.RiskDecision {
	if subjectID <= 0 {
		return nil
	}
	key := riskMemoryKey(scope, subjectID)
	if cached, ok := r.cache.Load(key); ok {
		entry, ok := cached.(*localRiskGateCacheEntry)
		if ok && entry != nil && entry.ExpiresAt > time.Now().Unix() {
			if entry.Decision == nil {
				return nil
			}
			copied := *entry.Decision
			return &copied
		}
		r.cache.Delete(key)
	}
	decision, err := r.store.GetBlock(scope, subjectID)
	if err != nil {
		return nil
	}
	cacheSeconds := int64(operation_setting.GetRiskControlSetting().LocalCacheSeconds)
	if decision == nil {
		r.cache.Store(key, &localRiskGateCacheEntry{
			Decision:  nil,
			ExpiresAt: time.Now().Unix() + cacheSeconds,
		})
		return nil
	}
	expiresAt := decision.BlockUntil
	if expiresAt <= 0 {
		expiresAt = time.Now().Unix() + cacheSeconds
	}
	r.cache.Store(key, &localRiskGateCacheEntry{
		Decision:  decision,
		ExpiresAt: expiresAt,
	})
	return decision
}

func (r *riskControlCenter) updateGateCache(decision *types.RiskDecision) {
	if decision == nil {
		return
	}
	r.cache.Store(riskMemoryKey(decision.Scope, decision.SubjectID), &localRiskGateCacheEntry{
		Decision:  decision,
		ExpiresAt: decision.BlockUntil,
	})
}

func normalizeUserAgent(ua string) string {
	ua = strings.TrimSpace(strings.ToLower(ua))
	if len(ua) > 256 {
		return ua[:256]
	}
	return ua
}

func validateRiskRule(rule *model.RiskRule) error {
	if rule == nil {
		return errors.New("rule is nil")
	}
	rule.Name = strings.TrimSpace(rule.Name)
	if rule.Name == "" {
		return errors.New("rule name is required")
	}
	if rule.Scope != RiskSubjectTypeToken && rule.Scope != RiskSubjectTypeUser {
		return errors.New("invalid rule scope")
	}
	if strings.TrimSpace(rule.Detector) == "" {
		rule.Detector = "distribution"
	}
	if strings.TrimSpace(rule.MatchMode) == "" {
		rule.MatchMode = "all"
	}
	if strings.TrimSpace(rule.Action) == "" {
		rule.Action = RiskActionObserve
	}
	if rule.ResponseStatusCode <= 0 {
		rule.ResponseStatusCode = operation_setting.GetRiskControlSetting().DefaultStatusCode
	}
	if rule.RecoverAfterSeconds <= 0 {
		rule.RecoverAfterSeconds = operation_setting.GetRiskControlSetting().DefaultRecoverAfterSecs
	}
	if strings.TrimSpace(rule.RecoverMode) == "" {
		rule.RecoverMode = operation_setting.GetRiskControlSetting().DefaultRecoverMode
	}
	var conditions []types.RiskCondition
	if err := common.UnmarshalJsonStr(rule.Conditions, &conditions); err != nil {
		return errors.New("rule conditions must be valid JSON")
	}
	if len(conditions) == 0 {
		return errors.New("at least one condition is required")
	}
	return nil
}

func seedDefaultRiskRules() error {
	count, err := model.CountRiskRules()
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	defaultRules := []*model.RiskRule{
		newDefaultRiskRule(
			"token_multi_ip_observe",
			"单个 API key 在短时间内出现多个不同 IP，先进入观察状态",
			RiskSubjectTypeToken,
			RiskActionObserve,
			false,
			30,
			[]types.RiskCondition{{Metric: "distinct_ip_10m", Op: ">=", Value: 3}},
		),
		newDefaultRiskRule(
			"token_multi_ip_high_concurrency_block",
			"单个 API key 在短时间内出现多个不同 IP 且并发明显偏高，自动封禁 15 分钟",
			RiskSubjectTypeToken,
			RiskActionBlock,
			true,
			50,
			[]types.RiskCondition{
				{Metric: "distinct_ip_10m", Op: ">=", Value: 5},
				{Metric: "inflight_now", Op: ">=", Value: 8},
			},
		),
		newDefaultRiskRule(
			"token_multi_ip_burst_block",
			"单个 API key 在 10 分钟内请求量异常且伴随多 IP 轮询，自动封禁 1 小时",
			RiskSubjectTypeToken,
			RiskActionBlock,
			true,
			60,
			[]types.RiskCondition{
				{Metric: "distinct_ip_10m", Op: ">=", Value: 3},
				{Metric: "request_count_10m", Op: ">=", Value: 120},
			},
		),
		newDefaultRiskRule(
			"user_multi_ip_observe",
			"同一用户在 1 小时内出现异常多的不同 IP，进入用户级观察",
			RiskSubjectTypeUser,
			RiskActionObserve,
			false,
			25,
			[]types.RiskCondition{{Metric: "distinct_ip_1h", Op: ">=", Value: 8}},
		),
	}
	for _, rule := range defaultRules {
		if err = model.CreateRiskRule(rule); err != nil {
			return err
		}
	}
	return nil
}

func newDefaultRiskRule(name string, description string, scope string, action string, autoBlock bool, score int, conditions []types.RiskCondition) *model.RiskRule {
	return &model.RiskRule{
		Name:                name,
		Description:         description,
		Enabled:             true,
		Scope:               scope,
		Detector:            "distribution",
		MatchMode:           "all",
		Priority:            score,
		Action:              action,
		AutoBlock:           autoBlock,
		AutoRecover:         true,
		RecoverMode:         operation_setting.GetRiskControlSetting().DefaultRecoverMode,
		RecoverAfterSeconds: operation_setting.GetRiskControlSetting().DefaultRecoverAfterSecs,
		ResponseStatusCode:  operation_setting.GetRiskControlSetting().DefaultStatusCode,
		ResponseMessage:     operation_setting.GetRiskControlSetting().DefaultResponseMessage,
		ScoreWeight:         score,
		Conditions:          encodeRiskJSON(conditions),
		CreatedBy:           0,
		UpdatedBy:           0,
	}
}

func (r *riskControlCenter) runRecoveryLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	lastCleanupAt := int64(0)
	for range ticker.C {
		now := common.GetTimestamp()
		snapshots, err := model.MarkExpiredRiskSubjectSnapshotsAsRecovered(now)
		if err != nil {
			common.SysError("risk control recover expired subjects failed: " + err.Error())
			continue
		}
		for _, snapshot := range snapshots {
			if snapshot == nil {
				continue
			}
			_ = r.store.ClearBlock(snapshot.SubjectType, snapshot.SubjectID)
			r.cache.Delete(riskMemoryKey(snapshot.SubjectType, snapshot.SubjectID))
			incident := &model.RiskIncident{
				CreatedAt:      now,
				SubjectType:    snapshot.SubjectType,
				SubjectID:      snapshot.SubjectID,
				UserID:         snapshot.UserID,
				TokenID:        snapshot.TokenID,
				Username:       snapshot.Username,
				TokenName:      snapshot.TokenName,
				TokenMaskedKey: snapshot.TokenMaskedKey,
				RuleName:       snapshot.LastRuleName,
				Action:         RiskActionRecover,
				Decision:       RiskDecisionAllow,
				Status:         snapshot.Status,
				AutoRecover:    true,
				RecoverAt:      now,
				ResolvedAt:     now,
				RiskScore:      snapshot.RiskScore,
				Reason:         "自动恢复",
			}
			if err = model.CreateRiskIncident(incident); err != nil {
				common.SysError("risk control create auto recover incident failed: " + err.Error())
			}
		}
		if lastCleanupAt == 0 || now-lastCleanupAt >= 3600 {
			if err = cleanupExpiredRiskData(now); err != nil {
				common.SysError("risk control cleanup expired data failed: " + err.Error())
			} else {
				lastCleanupAt = now
			}
		}
	}
}

func cleanupExpiredRiskData(now int64) error {
	retentionHours := operation_setting.GetRiskControlSetting().SnapshotRetentionHours
	if retentionHours <= 0 {
		return nil
	}
	cutoff := now - int64(retentionHours)*3600
	if err := model.DeleteExpiredRiskSubjectSnapshots(cutoff); err != nil {
		return err
	}
	return model.DeleteExpiredRiskIncidents(cutoff)
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func AppendRiskAuditToOther(other map[string]interface{}, audit *types.RiskAudit) {
	if other == nil || audit == nil {
		return
	}
	other["risk_control"] = audit
}

func GetBlockingDecisionFromAudit(audit *types.RiskAudit) *types.RiskDecision {
	if audit == nil {
		return nil
	}
	if audit.UserDecision != nil && audit.UserDecision.Decision == RiskDecisionBlock {
		return audit.UserDecision
	}
	if audit.TokenDecision != nil && audit.TokenDecision.Decision == RiskDecisionBlock {
		return audit.TokenDecision
	}
	return nil
}

func RecordRiskBlockedAccess(ctx *gin.Context, info *relaycommon.RelayInfo, decision *types.RiskDecision) {
	if decision == nil {
		return
	}
	gopool.Go(func() {
		event := globalRiskCenter.buildEvent(ctx, info, RiskEventTypeStart, common.GetTimestamp(), decision.StatusCode)
		if event == nil {
			return
		}
		snapshot, err := model.GetRiskSubjectSnapshot(decision.Scope, decision.SubjectID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			common.SysError("risk control blocked access load snapshot failed: " + err.Error())
			return
		}
		if snapshot != nil {
			snapshot.LastSeenAt = event.OccurAt
			snapshot.LastRequestPath = event.RequestPath
			snapshot.LastStatusCode = decision.StatusCode
			snapshot.LastDecision = decision.Decision
			snapshot.LastAction = decision.Action
			snapshot.LastReason = decision.Reason
			snapshot.LastEvaluatedAt = event.OccurAt
			_ = model.UpsertRiskSubjectSnapshot(snapshot)
		}
	})
}
