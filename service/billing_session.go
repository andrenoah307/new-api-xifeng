package service

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// BillingSession — 统一计费会话
// ---------------------------------------------------------------------------

// BillingSession 封装单次请求的预扣费/结算/退款生命周期。
// 实现 relaycommon.BillingSettler 接口。
type BillingSession struct {
	relayInfo        *relaycommon.RelayInfo
	funding          FundingSource
	preConsumedQuota int  // 实际预扣额度（信任用户可能为 0）
	tokenConsumed    int  // 令牌额度实际扣减量
	extraReserved    int  // 发送前补充预扣的额度（订阅退款时需要单独回滚）
	trusted          bool // 是否命中信任额度旁路
	fundingSettled   bool // funding.Settle 已成功，资金来源已提交
	settled          bool // Settle 全部完成（资金 + 令牌）
	refunded         bool // Refund 已调用
	mu               sync.Mutex
}

// Settle 根据实际消耗额度进行结算。
// 资金来源和令牌额度分两步提交：若资金来源已提交但令牌调整失败，
// 会标记 fundingSettled 防止 Refund 对已提交的资金来源执行退款。
func (s *BillingSession) Settle(actualQuota int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settled {
		return nil
	}
	delta := actualQuota - s.preConsumedQuota
	if delta == 0 {
		s.settled = true
		return nil
	}
	// 1) 调整资金来源（仅在尚未提交时执行，防止重复调用）
	if !s.fundingSettled {
		if err := s.funding.Settle(delta); err != nil {
			return err
		}
		s.fundingSettled = true
	}
	// 2) 调整令牌额度
	var tokenErr error
	if !s.relayInfo.IsPlayground {
		if delta > 0 {
			tokenErr = model.DecreaseTokenQuota(s.relayInfo.TokenId, s.relayInfo.TokenKey, delta)
		} else {
			tokenErr = model.IncreaseTokenQuota(s.relayInfo.TokenId, s.relayInfo.TokenKey, -delta)
		}
		if tokenErr != nil {
			// 资金来源已提交，令牌调整失败只能记录日志；标记 settled 防止 Refund 误退资金
			common.SysLog(fmt.Sprintf("error adjusting token quota after funding settled (userId=%d, tokenId=%d, delta=%d): %s",
				s.relayInfo.UserId, s.relayInfo.TokenId, delta, tokenErr.Error()))
		}
	}
	// 3) 更新 relayInfo 上的订阅 PostDelta（用于日志）
	if s.funding.Source() == BillingSourceSubscription {
		s.relayInfo.SubscriptionPostDelta += int64(delta)
	}
	s.settled = true
	return tokenErr
}

// Refund 退还所有预扣费，幂等安全，异步执行。
func (s *BillingSession) Refund(c *gin.Context) {
	s.mu.Lock()
	if s.settled || s.refunded || !s.needsRefundLocked() {
		s.mu.Unlock()
		return
	}
	s.refunded = true
	s.mu.Unlock()

	logger.LogInfo(c, fmt.Sprintf("用户 %d 请求失败, 返还预扣费（token_quota=%s, funding=%s）",
		s.relayInfo.UserId,
		logger.FormatQuota(s.tokenConsumed),
		s.funding.Source(),
	))

	// 复制需要的值到闭包中
	tokenId := s.relayInfo.TokenId
	tokenKey := s.relayInfo.TokenKey
	isPlayground := s.relayInfo.IsPlayground
	tokenConsumed := s.tokenConsumed
	extraReserved := s.extraReserved
	subscriptionId := s.relayInfo.SubscriptionId
	funding := s.funding

	gopool.Go(func() {
		// 1) 退还资金来源
		if err := funding.Refund(); err != nil {
			common.SysLog("error refunding billing source: " + err.Error())
		}
		if extraReserved > 0 && funding.Source() == BillingSourceSubscription && subscriptionId > 0 {
			if err := model.PostConsumeUserSubscriptionDelta(subscriptionId, -int64(extraReserved)); err != nil {
				common.SysLog("error refunding subscription extra reserved quota: " + err.Error())
			}
		}
		// 2) 退还令牌额度
		if tokenConsumed > 0 && !isPlayground {
			if err := model.IncreaseTokenQuota(tokenId, tokenKey, tokenConsumed); err != nil {
				common.SysLog("error refunding token quota: " + err.Error())
			}
		}
	})
}

// NeedsRefund 返回是否存在需要退还的预扣状态。
func (s *BillingSession) NeedsRefund() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.needsRefundLocked()
}

func (s *BillingSession) needsRefundLocked() bool {
	if s.settled || s.refunded || s.fundingSettled {
		// fundingSettled 时资金来源已提交结算，不能再退预扣费
		return false
	}
	if s.tokenConsumed > 0 {
		return true
	}
	// 订阅可能在 tokenConsumed=0 时仍预扣了额度
	if sub, ok := s.funding.(*SubscriptionFunding); ok && sub.preConsumed > 0 {
		return true
	}
	return false
}

// GetPreConsumedQuota 返回实际预扣的额度。
func (s *BillingSession) GetPreConsumedQuota() int {
	return s.preConsumedQuota
}

func (s *BillingSession) Reserve(targetQuota int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.settled || s.refunded || s.trusted || targetQuota <= s.preConsumedQuota {
		return nil
	}

	delta := targetQuota - s.preConsumedQuota
	if delta <= 0 {
		return nil
	}

	if err := s.reserveFunding(delta); err != nil {
		return err
	}
	if err := s.reserveToken(delta); err != nil {
		s.rollbackFundingReserve(delta)
		return err
	}

	s.preConsumedQuota += delta
	s.tokenConsumed += delta
	s.extraReserved += delta
	s.syncRelayInfo()
	return nil
}

// ---------------------------------------------------------------------------
// PreConsume — 统一预扣费入口（含信任额度旁路）
// ---------------------------------------------------------------------------

// preConsume 执行预扣费：信任检查 -> 令牌预扣 -> 资金来源预扣。
// 任一步骤失败时原子回滚已完成的步骤。
func (s *BillingSession) preConsume(c *gin.Context, quota int) *types.NewAPIError {
	effectiveQuota := quota

	// ---- 信任额度旁路 ----
	if s.shouldTrust(c) {
		s.trusted = true
		effectiveQuota = 0
		logger.LogInfo(c, fmt.Sprintf("用户 %d 额度充足, 信任且不需要预扣费 (funding=%s)", s.relayInfo.UserId, s.funding.Source()))
	} else if effectiveQuota > 0 {
		logger.LogInfo(c, fmt.Sprintf("用户 %d 需要预扣费 %s (funding=%s)", s.relayInfo.UserId, logger.FormatQuota(effectiveQuota), s.funding.Source()))
	}

	// ---- 1) 预扣令牌额度 ----
	if effectiveQuota > 0 {
		if err := PreConsumeTokenQuota(s.relayInfo, effectiveQuota); err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		s.tokenConsumed = effectiveQuota
	}

	// ---- 2) 预扣资金来源 ----
	if err := s.funding.PreConsume(effectiveQuota); err != nil {
		// 预扣费失败，回滚令牌额度
		if s.tokenConsumed > 0 && !s.relayInfo.IsPlayground {
			if rollbackErr := model.IncreaseTokenQuota(s.relayInfo.TokenId, s.relayInfo.TokenKey, s.tokenConsumed); rollbackErr != nil {
				common.SysLog(fmt.Sprintf("error rolling back token quota (userId=%d, tokenId=%d, amount=%d, fundingErr=%s): %s",
					s.relayInfo.UserId, s.relayInfo.TokenId, s.tokenConsumed, err.Error(), rollbackErr.Error()))
			}
			s.tokenConsumed = 0
		}
		// TODO: model 层应定义哨兵错误（如 ErrNoActiveSubscription），用 errors.Is 替代字符串匹配
		errMsg := err.Error()
		if strings.Contains(errMsg, "no active subscription") || strings.Contains(errMsg, "subscription quota insufficient") {
			return types.NewErrorWithStatusCode(fmt.Errorf("订阅额度不足或未配置订阅: %s", errMsg), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}

	s.preConsumedQuota = effectiveQuota

	// ---- 同步 RelayInfo 兼容字段 ----
	s.syncRelayInfo()

	return nil
}

func (s *BillingSession) reserveFunding(delta int) error {
	switch funding := s.funding.(type) {
	case *WalletFunding:
		if err := model.DecreaseUserQuota(funding.userId, delta, false); err != nil {
			return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
		}
		funding.consumed += delta
		return nil
	case *SubscriptionFunding:
		if err := model.PostConsumeUserSubscriptionDelta(funding.subscriptionId, int64(delta)); err != nil {
			return types.NewErrorWithStatusCode(
				fmt.Errorf("订阅额度不足或未配置订阅: %s", err.Error()),
				types.ErrorCodeInsufficientUserQuota,
				http.StatusForbidden,
				types.ErrOptionWithSkipRetry(),
				types.ErrOptionWithNoRecordErrorLog(),
			)
		}
		return nil
	default:
		return types.NewError(fmt.Errorf("unsupported funding source: %s", s.funding.Source()), types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
}

func (s *BillingSession) rollbackFundingReserve(delta int) {
	switch funding := s.funding.(type) {
	case *WalletFunding:
		if err := model.IncreaseUserQuota(funding.userId, delta, false); err != nil {
			common.SysLog("error rolling back wallet funding reserve: " + err.Error())
		} else {
			funding.consumed -= delta
		}
	case *SubscriptionFunding:
		if err := model.PostConsumeUserSubscriptionDelta(funding.subscriptionId, -int64(delta)); err != nil {
			common.SysLog("error rolling back subscription funding reserve: " + err.Error())
		}
	}
}

func (s *BillingSession) reserveToken(delta int) error {
	if delta <= 0 || s.relayInfo.IsPlayground {
		return nil
	}
	if err := PreConsumeTokenQuota(s.relayInfo, delta); err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	return nil
}

// shouldTrust 统一信任额度检查，适用于钱包和订阅。
func (s *BillingSession) shouldTrust(c *gin.Context) bool {
	// 异步任务（ForcePreConsume=true）必须预扣全额，不允许信任旁路
	if s.relayInfo.ForcePreConsume {
		return false
	}

	trustQuota := common.GetTrustQuota()
	if trustQuota <= 0 {
		return false
	}

	// 检查令牌是否充足（坑点 #139：操练场合成令牌与无限令牌一样不参与令牌门控）
	tokenTrusted := tokenNonGating(s.relayInfo.TokenUnlimited, s.relayInfo.IsPlayground)
	if !tokenTrusted {
		tokenQuota := c.GetInt("token_quota")
		tokenTrusted = tokenQuota > trustQuota
	}
	if !tokenTrusted {
		return false
	}

	switch s.funding.Source() {
	case BillingSourceWallet:
		return s.relayInfo.UserQuota > trustQuota
	case BillingSourceSubscription:
		// 订阅不能启用信任旁路。原因：
		// 1. PreConsumeUserSubscription 要求 amount>0 来创建预扣记录并锁定订阅
		// 2. SubscriptionFunding.PreConsume 忽略参数，始终用 s.amount 预扣
		// 3. 若信任旁路将 effectiveQuota 设为 0，会导致 preConsumedQuota 与实际订阅预扣不一致
		return false
	default:
		return false
	}
}

// syncRelayInfo 将 BillingSession 的状态同步到 RelayInfo 的兼容字段上。
func (s *BillingSession) syncRelayInfo() {
	info := s.relayInfo
	info.FinalPreConsumedQuota = s.preConsumedQuota
	info.BillingSource = s.funding.Source()

	if sub, ok := s.funding.(*SubscriptionFunding); ok {
		info.SubscriptionId = sub.subscriptionId
		info.SubscriptionPreConsumed = sub.preConsumed + int64(s.extraReserved)
		info.SubscriptionPostDelta = 0
		info.SubscriptionAmountTotal = sub.AmountTotal
		info.SubscriptionAmountUsedAfterPreConsume = sub.AmountUsedAfter + int64(s.extraReserved)
		info.SubscriptionPlanId = sub.PlanId
		info.SubscriptionPlanTitle = sub.PlanTitle
	} else {
		info.SubscriptionId = 0
		info.SubscriptionPreConsumed = 0
	}
}

// ---------------------------------------------------------------------------
// buildInsufficientQuotaMessage 组装「余额/令牌额度不足」的友好错误串，用于预扣硬拒。
// 预扣 403 携 ErrOptionWithNoRecordErrorLog 不落盘（坑点 #138），因此把模型 / 分组(倍率) /
// 上下文估算 / 最低需预扣成本 / 当前余额都写进错误串——既让终端用户理解并自助（充值或减小
// 上下文 / 降低 max_tokens），也让双前端据此渲染友好提示，并为事后取证保留关键量。
func buildInsufficientQuotaMessage(info *relaycommon.RelayInfo, remainQuota, minQuota int, isToken bool) string {
	group := info.UsingGroup
	if group == "" {
		group = info.UserGroup
	}
	subject, remainLabel := "用户额度不足", "当前余额"
	if isToken {
		subject, remainLabel = "令牌额度不足", "令牌剩余额度"
	}
	return fmt.Sprintf("%s：模型 %s（分组 %s，分组倍率 %g），预估上下文约 %d tokens，最低需预扣 %s，%s %s。请充值，或减小上下文 / 降低 max_tokens 后重试。",
		subject, info.OriginModelName, group, info.PriceData.GroupRatioInfo.GroupRatio,
		info.GetEstimatePromptTokens(), logger.FormatQuota(minQuota), remainLabel, logger.FormatQuota(remainQuota))
}

// NewBillingSession 工厂 — 根据计费偏好创建会话并处理回退
// ---------------------------------------------------------------------------

// preConsumeReject 标记优雅部分预扣的拒绝归因（坑点 #138）。
type preConsumeReject int

const (
	preConsumeOK           preConsumeReject = iota
	preConsumeRejectWallet                  // 钱包余额连输入下限都付不起
	preConsumeRejectToken                   // 令牌额度连输入下限都付不起
)

// tokenNonGating 判定令牌是否不参与预扣硬门控（坑点 #139）。
// 无限令牌（TokenUnlimited）与操练场合成令牌（IsPlayground，无 Key/token_quota=0）均不以令牌额度卡预扣：
// 前者本就无限；后者令牌消费在 PreConsumeTokenQuota/reserveToken/preConsume 回滚已按 IsPlayground 跳过，
// 门控侧须对齐，否则操练场令牌分支必拒并触发空 Key 的 reconcileTokenReject 硬拒。
func tokenNonGating(tokenUnlimited, isPlayground bool) bool {
	return tokenUnlimited || isPlayground
}

// computePartialTarget 计算非受信任用户的实际预扣目标额（坑点 #137 优雅部分预扣）。
// 当余额/令牌不足以覆盖最坏估算 fullQuota、但仍能覆盖仅输入的预扣下限 minQuota 时，
// 预扣「可用额」而非硬拒；结算回真（可短暂走负，有界，下一请求 userQuota<=0 兜底）。
// 返回 (target, reject)；reject 标记连输入下限都付不起时的拒绝归因。
func computePartialTarget(userQuota, tokenQuota int, tokenUnlimited bool, fullQuota, minQuota int) (int, preConsumeReject) {
	target := fullQuota
	if userQuota < target {
		if userQuota < minQuota {
			return 0, preConsumeRejectWallet
		}
		target = userQuota
	}
	if !tokenUnlimited && tokenQuota < target {
		if tokenQuota < minQuota {
			return 0, preConsumeRejectToken
		}
		target = tokenQuota
	}
	return target, preConsumeOK
}

// resolveFreshTokenTarget 依据 fresh 令牌的真实无限状态，决定令牌分支拒绝后的目标额（坑点 #138）。
// tokenUnlimited=true：令牌实为无限（上下文/缓存曾过期误判为限额），令牌不参与门控，
// 仅按钱包口径 computePartialTarget(userQuota,0,true,...) 计算目标，ok=true。
// tokenUnlimited=false：令牌确为限额且余额不足以覆盖输入下限，ok=false（调用方报「令牌额度不足」）。
func resolveFreshTokenTarget(tokenUnlimited bool, userQuota, fullQuota, minQuota int) (int, bool) {
	if !tokenUnlimited {
		return 0, false
	}
	target, reject := computePartialTarget(userQuota, 0, true, fullQuota, minQuota)
	return target, reject == preConsumeOK
}

// reconcileTokenReject 在令牌分支即将硬拒时，用 fresh DB 令牌复核真实无限状态（坑点 #138）。
// fromDB=true 绕过可能过期的 Redis 令牌缓存，并借 GetTokenByKey 的 defer 自愈缓存。
// 真无限：修复 relayInfo.TokenUnlimited 与上下文，令下游 PreConsumeTokenQuota 一并跳过令牌校验；返回钱包口径目标。
// 真限额：返回「令牌额度不足」错误，正确归因到令牌而非钱包。
func (s *BillingSession) reconcileTokenReject(c *gin.Context, userQuota, fullQuota, minQuota int) (int, *types.NewAPIError) {
	token, err := model.GetTokenByKey(s.relayInfo.TokenKey, true)
	if err != nil {
		return 0, types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
	}
	target, ok := resolveFreshTokenTarget(token.UnlimitedQuota, userQuota, fullQuota, minQuota)
	if !ok {
		return 0, types.NewErrorWithStatusCode(
			fmt.Errorf("%s", buildInsufficientQuotaMessage(s.relayInfo, token.RemainQuota, minQuota, true)),
			types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
			types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	// 令牌实为无限：修复过期标志（下游 PreConsumeTokenQuota 用 relayInfo.TokenUnlimited 判跳过）
	s.relayInfo.TokenUnlimited = true
	c.Set(string(constant.ContextKeyTokenUnlimited), true)
	return target, nil
}

// NewBillingSession 根据用户计费偏好创建 BillingSession，处理 subscription_first / wallet_first 的回退。
func NewBillingSession(c *gin.Context, relayInfo *relaycommon.RelayInfo, preConsumedQuota int, minPreConsumedQuota int) (*BillingSession, *types.NewAPIError) {
	if relayInfo == nil {
		return nil, types.NewError(fmt.Errorf("relayInfo is nil"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	pref := common.NormalizeBillingPreference(relayInfo.UserSetting.BillingPreference)

	// 钱包路径需要先检查用户额度
	tryWallet := func() (*BillingSession, *types.NewAPIError) {
		userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
		}
		if userQuota <= 0 {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("用户额度不足, 剩余额度: %s", logger.FormatQuota(userQuota)),
				types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
				types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		// 必须在 shouldTrust 之前赋值：shouldTrust 依赖 relayInfo.UserQuota 判定信任旁路。
		relayInfo.UserQuota = userQuota

		session := &BillingSession{
			relayInfo: relayInfo,
			funding:   &WalletFunding{userId: relayInfo.UserId},
		}

		// 预扣硬门控必须放在信任判定之后。受信任用户（余额 > TrustQuota 且令牌额度充足）
		// 实际预扣为 0，真实成本由结算补正，不能因为「虚高的预扣估算 > 当前余额」而误杀
		// （历史 bug：大输入估算冲高时，余额上百的信任用户仍被拒 "预扣费额度失败"）。
		// 仅当用户不被信任时，才用预扣估算去卡余额。
		preConsumeTarget := preConsumedQuota
		if !session.shouldTrust(c) {
			// 坑点 #137：优雅部分预扣——余额/令牌不足以覆盖最坏估算但能覆盖输入下限时，
			// 预扣可用额而非硬拒，避免临界拒绝与「末位余额不可花费」。
			tokenQuota := c.GetInt("token_quota")
			// 坑点 #139：操练场合成令牌（无 Key/非无限/token_quota=0）令牌侧不参与门控，
			// 否则令牌分支必拒并触发空 Key 的 reconcileTokenReject 硬拒；与既有 IsPlayground 跳过一致。
			target, reject := computePartialTarget(userQuota, tokenQuota, tokenNonGating(relayInfo.TokenUnlimited, relayInfo.IsPlayground), preConsumedQuota, minPreConsumedQuota)
			switch reject {
			case preConsumeRejectWallet:
				return nil, types.NewErrorWithStatusCode(
					fmt.Errorf("%s", buildInsufficientQuotaMessage(relayInfo, userQuota, minPreConsumedQuota, false)),
					types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
					types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
			case preConsumeRejectToken:
				// 坑点 #138：令牌分支拒绝时上下文 token_quota 可能过期，以 fresh DB 令牌为权威复核，
				// 避免误拒钱包充裕用户，并正确归因（限额耗尽报令牌，无限则放行并修复标志）。
				freshTarget, apiErr := session.reconcileTokenReject(c, userQuota, preConsumedQuota, minPreConsumedQuota)
				if apiErr != nil {
					return nil, apiErr
				}
				preConsumeTarget = freshTarget
			default:
				preConsumeTarget = target
			}
		}

		if apiErr := session.preConsume(c, preConsumeTarget); apiErr != nil {
			return nil, apiErr
		}
		return session, nil
	}

	trySubscription := func() (*BillingSession, *types.NewAPIError) {
		subConsume := int64(preConsumedQuota)
		if subConsume <= 0 {
			subConsume = 1
		}
		session := &BillingSession{
			relayInfo: relayInfo,
			funding: &SubscriptionFunding{
				requestId: relayInfo.RequestId,
				userId:    relayInfo.UserId,
				modelName: relayInfo.OriginModelName,
				amount:    subConsume,
			},
		}
		// 必须传 subConsume 而非 preConsumedQuota，保证 SubscriptionFunding.amount、
		// preConsume 参数和 FinalPreConsumedQuota 三者一致，避免订阅多扣费。
		if apiErr := session.preConsume(c, int(subConsume)); apiErr != nil {
			return nil, apiErr
		}
		return session, nil
	}

	switch pref {
	case "subscription_only":
		return trySubscription()
	case "wallet_only":
		return tryWallet()
	case "wallet_first":
		session, err := tryWallet()
		if err != nil {
			if err.GetErrorCode() == types.ErrorCodeInsufficientUserQuota {
				return trySubscription()
			}
			return nil, err
		}
		return session, nil
	case "subscription_first":
		fallthrough
	default:
		hasSub, subCheckErr := model.HasActiveUserSubscription(relayInfo.UserId)
		if subCheckErr != nil {
			return nil, types.NewError(subCheckErr, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
		}
		if !hasSub {
			return tryWallet()
		}
		session, apiErr := trySubscription()
		if apiErr != nil {
			if apiErr.GetErrorCode() == types.ErrorCodeInsufficientUserQuota {
				return tryWallet()
			}
			return nil, apiErr
		}
		return session, nil
	}
}
