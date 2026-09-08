package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// 预扣拒绝一律带 ErrOptionWithNoRecordErrorLog，不落 logs 表（坑点 #138 的既有决定）。
// 后台日志里那条 "relay error: 用户额度不足" 因此不含 user_id / token_id / 模型名，
// 运维无法定位是谁被拒。这里补上后台侧的结构化归因，客户端可见文案与错误码一字不动。
type PreConsumeRejectReason string

const (
	// 余额本身已耗尽（<= 0），与估算无关。
	PreConsumeRejectReasonWalletExhausted PreConsumeRejectReason = "wallet_exhausted"
	// 余额付不起「仅输入」的预扣下限，被估算门控拒绝。
	PreConsumeRejectReasonWalletGate PreConsumeRejectReason = "wallet_estimate_gate"
	// 条件扣减未命中：读到的余额够，真正 UPDATE 时已不够（并发或缓存漂移）。
	PreConsumeRejectReasonWalletReserve PreConsumeRejectReason = "wallet_reserve_miss"
	// 令牌限额不足以覆盖预扣下限。
	PreConsumeRejectReasonTokenQuota PreConsumeRejectReason = "token_quota_insufficient"
	// 令牌预扣写库失败。
	PreConsumeRejectReasonTokenPreConsume PreConsumeRejectReason = "token_pre_consume_failed"
	// 令牌周期限额门控拒绝。
	PreConsumeRejectReasonTokenPeriodLimit PreConsumeRejectReason = "token_period_limit"
	// 订阅额度不足或未配置订阅。
	PreConsumeRejectReasonSubscriptionQuota PreConsumeRejectReason = "subscription_quota_insufficient"
)

// PreConsumeRejectDetails 是允许写进后台日志的字段白名单。
// 禁止扩充为携带 token key、Authorization、邮箱、请求体或上游原始错误全文；
// 分组 / 倍率 / 渠道也不带——预扣发生在选定渠道之前，这些字段通常不存在。
type PreConsumeRejectDetails struct {
	Reason         PreConsumeRejectReason
	ErrorCode      string
	BillingSource  string
	UserQuota      int
	TokenQuota     int
	FullQuota      int
	MinQuota       int
	EstimateTokens int
}

const preConsumeRejectContextKey = "preconsume_reject_details"

// markPreConsumeReject 记录本次预扣尝试的拒绝归因，供最终边界取用。
// 只有边界才落日志：wallet_first 钱包不足转订阅并成功时，边界不会拿到错误，
// 因此这条中间归因不会被记成最终拒绝；订阅随后也失败时则被订阅归因覆盖。
func markPreConsumeReject(c *gin.Context, details PreConsumeRejectDetails) {
	if c == nil {
		return
	}
	c.Set(preConsumeRejectContextKey, details)
}

func takePreConsumeRejectMark(c *gin.Context) (PreConsumeRejectDetails, bool) {
	if c == nil {
		return PreConsumeRejectDetails{}, false
	}
	value, ok := c.Get(preConsumeRejectContextKey)
	if !ok {
		return PreConsumeRejectDetails{}, false
	}
	details, ok := value.(PreConsumeRejectDetails)
	if !ok {
		return PreConsumeRejectDetails{}, false
	}
	// 取走即清除，保证同一请求内一次拒绝只记一行。
	c.Set(preConsumeRejectContextKey, nil)
	return details, true
}

// LogPreConsumeReject 在最终拒绝边界写一行稳定的 key=value 文本，便于 grep。
// request id 由 logger 自动带上；c 为 nil 时（无 gin 上下文的路径）显式退回 SYSTEM，
// 不能把 typed-nil 的 *gin.Context 当作 context.Context 传给 logger（会在 Value 里空指针）。
func LogPreConsumeReject(c *gin.Context, info *relaycommon.RelayInfo, details PreConsumeRejectDetails) {
	if info == nil {
		return
	}
	msg := fmt.Sprintf(
		"preconsume_reject reason=%s error_code=%s billing_source=%s user_id=%d token_id=%d model=%q estimate_tokens=%d full_quota=%d min_quota=%d user_quota=%d token_quota=%d",
		details.Reason, details.ErrorCode, details.BillingSource,
		info.UserId, info.TokenId, info.OriginModelName,
		details.EstimateTokens, details.FullQuota, details.MinQuota,
		details.UserQuota, details.TokenQuota,
	)
	if c == nil {
		logger.LogWarn(nil, msg+fmt.Sprintf(" request_id=%q", info.RequestId))
		return
	}
	logger.LogWarn(c, msg)
}

// logPreConsumeRejectBoundary 是 BillingSession 链路唯一的落盘点：
// 取出失败构造点留下的归因并写一行。没有归因时不写（例如参数校验类错误）。
func logPreConsumeRejectBoundary(c *gin.Context, info *relaycommon.RelayInfo, apiErr *types.NewAPIError) {
	if c == nil || info == nil || apiErr == nil {
		return
	}
	details, marked := takePreConsumeRejectMark(c)
	// 周期门控在 BillingSession 之外拒绝，没有构造点归因；错误码本身即唯一判据。
	// 它又先于钱包/令牌门控执行，所以此刻任何残留归因都不是最终原因，直接覆盖。
	if apiErr.GetErrorCode() == types.ErrorCodeTokenPeriodQuotaExceeded {
		details = PreConsumeRejectDetails{
			Reason:     PreConsumeRejectReasonTokenPeriodLimit,
			UserQuota:  info.UserQuota,
			TokenQuota: c.GetInt("token_quota"),
		}
	} else if !marked {
		return
	}
	details.ErrorCode = string(apiErr.GetErrorCode())
	LogPreConsumeReject(c, info, details)
}
