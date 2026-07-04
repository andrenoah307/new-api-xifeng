# 28 · 操练场预扣令牌门控：IsPlayground 必须豁免

> 适用范围：`service/billing_session.go` 非受信任预扣硬门控（`shouldTrust` + `computePartialTarget` 调用）。
> 关联坑点：#139。关联：[26](26-preconsume-sentinel-and-partial.md)（#137 部分预扣）、[27](27-preconsume-token-gate-attribution.md)（#138 fresh 令牌复核）。

## 1. 现象
操练场（`/pg/chat/completions`）提交后被预扣硬门控 403 拦截、无法请求。生产 micu-prod-do-us-1 / new-api-third 部署最近两次预扣提交（`54b353c8`、`07fd94cf`）后出现，回退上一提交可用。

## 2. 根因
操练场经 `middleware.UserAuth()`（非 TokenAuth），`controller/playground.go` 合成 tempToken：`UnlimitedQuota=false`、`Key=""`、`RemainQuota=0`；`SetupContextForToken` 据此写入 `token_unlimited_quota=false`、`token_quota=0`、`token_key=""`。

- `shouldTrust`：tokenTrusted 需 `TokenUnlimited` 或 `token_quota>trustQuota`，两者皆否 → 不信任 → 走非受信路径。
- `computePartialTarget`（#137 新增令牌分支）：`!tokenUnlimited && tokenQuota(0) < minQuota` → `preConsumeRejectToken`。
- `reconcileTokenReject`（#138 新增）：`GetTokenByKey("", true)` → `ErrRecordNotFound` → 403。

**回归源**：此前 `NewBillingSession` 只有钱包门控（`userQuota-preConsumedQuota<0`），从不卡 token_quota，操练场一直可用；#137 令牌分支 + #138 fresh 复核未豁免 IsPlayground。

**一致性缺口**：令牌消费侧 `PreConsumeTokenQuota`(`service/quota.go`)、`reserveToken`、`preConsume` 回滚**三处均已 `if IsPlayground` 跳过**，唯独新门控漏了。

## 3. 修复（后端 only）
`service/billing_session.go` 新增纯函数并在两处使用：

    func tokenNonGating(tokenUnlimited, isPlayground bool) bool { return tokenUnlimited || isPlayground }

- `shouldTrust`：`tokenTrusted := tokenNonGating(TokenUnlimited, IsPlayground)`（再叠加 `token_quota>trustQuota`）。
- `computePartialTarget` 调用：第三参传 `tokenNonGating(TokenUnlimited, IsPlayground)` → 令牌分支跳过、仅钱包门控；富额操练场用户享信任旁路 0 预扣，低额用户按钱包正确门控；永不触发空 Key `reconcileTokenReject`。

## 4. 前后对比
| 场景 | 修复前 | 修复后 |
|---|---|---|
| 操练场任意用户 | 令牌分支 → 空 Key fresh 复核 → 403 | 令牌不门控，按钱包判定 |
| 操练场富额（钱包>trust） | 403 | 信任旁路，0 预扣，结算补正 |
| 操练场钱包不足 | 403（误报令牌/空 Key） | 正确报「用户额度不足」 |
| 普通限额令牌 | 不变 | 不变（IsPlayground=false） |

## 5. 前端
无需改动：操练场 UI 仅透传网关响应，无余额/预扣预检（已核 Classic/Default）。

## 6. 测试
`service/billing_session_test.go:TestTokenNonGating` 覆盖 4 组合 100%；`TestComputePartialTarget`/`TestResolveFreshTokenTarget` 不变仍过；`go build ./...` 通过。

## 7. 红线
任何"预扣硬门控"新增判定，必须同时豁免 `TokenUnlimited` 与 `IsPlayground`；令牌消费侧的 IsPlayground 跳过已是既定约定，门控侧必须对齐。
