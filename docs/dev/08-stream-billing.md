# 08 · 流式异常计费与 usage 捕获

> 适用范围：流式中继在「客户端断开 / 上游 usage 不全 / 空输出」等异常下的计费口径。
> 关联坑点：#94（上游不返回 usage → 零计费）、#122（Claude/Gemini 空输出归零）、#125（预扣钳制 max_tokens）、#132（断流 + 上游 usage 完全缺失禁止估算计费）。

## 1. 设计原则（红线）

1. **上游 usage 可信时**：以上游 usage 为准计费。
2. **上游 usage 不全/缺失时**：**零计费 + 标记 `LocalCountTokens`**，交管理员识别异常渠道，**绝不用本地估算值实扣**。
3. 本地估算（`service/token_estimator.go:EstimateTokenByModel` = `len([]rune)*3/2`）**仅用于**「上游本就不报 usage 的渠道」（Cloudflare 等），且仅用于**预扣**确保余额充足；预扣可膨胀，结算必须回到真实 usage 或零计费。
4. 预扣估算必须钳制客户端 `max_tokens`（#125，`common.ClampPreConsumeCompletionTokens`），防虚高顶穿。

## 2. 断流后抢 usage（drain）

提交 `76b4f721`：`relay/helper/stream_scanner.go` 在客户端断开（`client_gone`）后**不立即退出**，置 `clientGone` 标志继续读上游至拿到 usage 事件或 `DrainAfterClientGone=15s` 超时。

- **共享逻辑，OpenAI 与 Claude 两路都生效**（OpenAI 的 `stream_options.include_usage` 与 Claude 的 `message_delta.usage`）。
- 局限：当上游仍在生成（长请求，`frt` 可达数百秒），15s 内拿不到最终 usage，则进入各 handler 的「无 usage 兜底」。

## 3. 无 usage 兜底：两路必须对齐零计费

| 路径 | 入口 | 无上游 usage 时 | 结果 |
|---|---|---|---|
| OpenAI 流式 | `relay/channel/openai/relay-openai.go:183` | `!containStreamUsage → usage=&dto.Usage{}` + `LocalCountTokens` | TotalTokens=0 → **零计费** |
| Claude 流式 | `relay/channel/claude/relay-claude.go:HandleStreamFinalResponse` | 见 #132 修复 | **对齐零计费** |

`calculateTextQuotaSummary`（`service/text_quota.go`）：`TotalTokens==0 → Quota=0`；`LocalCountTokens=true` 时跳过「保底 1 quota」，确保零计费意图不被覆盖。

## 4. 坑点 #132：Claude 断流 + 上游 usage 完全缺失的巨额误扣

### 现象（生产取证）
- 实例 micu-prod-do-us-1，渠道 255（type 14 Anthropic，上游 aimuxr.com），模型 `glm-5.2-fast`。
- 异常请求 `prompt_tokens=737400`、`completion=3220`、`quota=2,246,010 ≈ $4.49`，`other.local_count_tokens=true`、`stream_status=client_gone/context canceled`、`frt=262s`。
- 同用户正常完成请求的真实 prompt 稳定 **~170k**；本地估算把它膨胀到 **~740k（~4.3x）**。24h 同类（lct=true + 大 prompt）批量误扣可观。

### 根因
上游 usage 仅在结尾 `message_delta` 给出，客户端在此之前断开 → `claudeInfo.Usage.PromptTokens==0` → 旧代码回退 `GetEstimatePromptTokens()`（膨胀估算）；又因 `completion>0`（有部分输出）未命中 #122 的「`completion==0 && ResponseText 为空`」归零 → 按膨胀估算实扣。

### 修复
`HandleStreamFinalResponse`：
```go
upstreamUsageMissing := !claudeInfo.Done &&
    claudeInfo.Usage.PromptTokens == 0 &&
    claudeInfo.Usage.PromptTokensDetails.CachedTokens == 0 &&
    claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens == 0
```
- `upstreamUsageMissing` 为真 → 跳过估算 fallback；零计费块条件追加 `|| upstreamUsageMissing`（整份归零含 completion + `LocalCountTokens`），对齐 OpenAI handler。
- 保留两类合法场景：① message_start 已给真实 prompt 的断流请求（`PromptTokens>0`，按真实 prompt + 估算 completion 计费）；② 全缓存命中（`Done=true && CachedTokens>0`，不误零）。
- 回填 completion 改用 `service.EstimateTokenByModel`（不再走 `ResponseText2Usage`，避免其首行无条件置 `LocalCountTokens` 误标真实 prompt 路径）。

### 测试
`relay/channel/claude/relay_claude_zero_charge_test.go`：
- `TestHandleStreamFinalResponse_ZeroChargesClientGoneNoUpstreamUsage`
- `TestHandleStreamFinalResponse_PreservesRealPromptOnClientGone`
- `TestHandleStreamFinalResponse_CacheOnlyNotZeroed`

### 配置级规避（即时止血）
把渠道改为上游 **OpenAI 请求格式**（`request_conversion` 链尾为 `OpenAI Compatible` 而非 `Claude Messages`）→ 走 openai 语义，断流时零计费。代码级修复是根治。

### 既有失败登记（非本次引入）
`relay/channel/claude/relay_claude_test.go:TestRequestOpenAI2ClaudeMessage_ConvertsTextFileContentToText` 在基线即 FAIL（text vs image），与本修复无关，待单独处理。
