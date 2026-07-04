# 27 · 预扣令牌门控：归因纠正 + fresh 令牌复核

> 适用范围：`service/billing_session.go` 钱包路径 `tryWallet` 的非受信任预扣硬门控。
> 关联坑点：#138（令牌分支拒绝误报成钱包 + 上下文过期误判无限令牌为限额）。
> 关联：[26-preconsume-sentinel-and-partial.md](26-preconsume-sentinel-and-partial.md)（#137 优雅部分预扣，本文在其 `computePartialTarget` 上继续）。

## 1. 现象（生产取证 micu-prod-do-us-1 / new-api-third）

请求 `20260703180433...RcgYJDUI` 报：

```
预扣费额度失败, 用户剩余额度: ¥153.731172, 需要预扣费额度: ¥0.002674
```

余额 ¥153.73 **远大于**所需 ¥0.002674，却被拒——与 #137 的「哨兵虚高」完全相反（金额极小）。

**反推定位**：`¥153.731172 × 500000 = 76,865,586 quota`，唯一命中用户 **4795 (proecheng@163.com)**（现 quota `76,861,903`，差 3683≈之后几笔小额消费）。其令牌全部 `unlimited_quota=1`、`remain_quota` 深度为负（无限令牌 remain 仅为计数器，`TokenUnlimited=true` 时从不校验，负值正常）。

## 2. 根因

拒绝串仅出现在 `computePartialTarget`（#137）。在 `userQuota(76.8M) ≫ fullQuota(1337=¥0.0027)` 下，钱包分支 `userQuota<target` **不可能**触发 → **唯一解是令牌分支**：

```go
if !tokenUnlimited && tokenQuota < target { if tokenQuota < minQuota { return 0, false } }
```

即运行时 `relayInfo.TokenUnlimited==false` 且 `c.GetInt("token_quota")` 为负/极小。无限令牌在 `middleware/auth.go` **不设** `token_quota`（`if !token.UnlimitedQuota`），故只要 `TokenUnlimited` 被误判为 false，令牌就被当成「限额且余额耗尽」→ 拒。

两类触发（本修复同时覆盖，无需二选一）：
1. **令牌确为限额且 `remain_quota` 走负**：`DecreaseTokenQuota`/结算无条件递减，限额令牌可走负；`remain < min` → 拒。此拒**语义正确**，但 #137 的消息把约束**误报成「用户剩余额度」（钱包）**，用户遂误以为「有钱却被拒」。
2. **令牌实为无限，但上下文 `token_unlimited_quota` 过期为 false**（Redis 令牌缓存/上下文滞后）→ 无限令牌被误判限额 → **误拒**钱包充裕用户。

`shouldTrust` 亦因 `tokenTrusted=relayInfo.TokenUnlimited=false` 而未走信任旁路（否则 76.8M 钱包早该旁路成功）；`PreConsumeTokenQuota`（`service/quota.go`）用 `relayInfo.TokenUnlimited` 判跳过、却用 fresh `token.RemainQuota` 判额度——同源过期隐患。**正确范式**：`service/quota.go:147` 实时路径用 **fresh `token.UnlimitedQuota`**，不信上下文。

## 3. 修复（后端 only）

`service/billing_session.go`：

1. `computePartialTarget` 返回值由 `(int, bool)` 改为 `(int, preConsumeReject)`，区分 `preConsumeRejectWallet` / `preConsumeRejectToken` / `preConsumeOK`。
2. 纯函数 `resolveFreshTokenTarget(tokenUnlimited, userQuota, full, min) (target, ok)`：无限→按钱包口径 `computePartialTarget(userQuota,0,true,...)`；限额→`ok=false`。
3. 方法 `reconcileTokenReject`：令牌分支拒绝时以 `model.GetTokenByKey(TokenKey, true)`（`fromDB=true` 绕过缓存、并借 defer 自愈缓存）复核：
   - 真无限 → 修复 `relayInfo.TokenUnlimited=true` + 上下文 `ContextKeyTokenUnlimited`（令下游 `PreConsumeTokenQuota` 一并跳过），返回钱包口径目标 → **放行**；
   - 真限额 → 报 `令牌额度不足, 令牌剩余额度: X, 需要预扣费额度: Y`（**正确归因到令牌**）。
4. `tryWallet` 按 `reject` 分流：钱包不足报「用户额度不足」、令牌不足走 `reconcileTokenReject`。**热路径零新增开销**（fresh 读仅在即将硬拒的罕见路径）。

## 4. 前后对比

| 场景 | 修复前(#137) | 修复后(#138) |
|---|---|---|
| 限额令牌 remain 走负、钱包充裕 | 拒，报「用户剩余额度 ¥153」（误导） | 拒，报「令牌额度不足, 令牌剩余额度 …」（正确归因） |
| 无限令牌但上下文过期为 false | **误拒**「预扣费额度失败」 | fresh 复核识别无限 → **放行** + 修复标志 + 自愈缓存 |
| 钱包连输入下限都不够 | 报「用户剩余额度」（打印 full） | 报「用户额度不足」（打印 min，真实门槛） |

## 5. 测试

- `service/billing_session_test.go:TestComputePartialTarget`：8 例更新为原因断言（`computePartialTarget` 覆盖 100%）。
- `TestResolveFreshTokenTarget`：4 例——限额拒 / 无限全覆盖（本 bug 回归守卫）/ 无限钱包部分 / 无限钱包低于 min（`resolveFreshTokenTarget` 覆盖 100%）。

## 6. 前端

**无需改动**：该 403 仅返回 API 客户端（CLI），Classic/Default 均不渲染；用量日志的「预扣」金额展示不受影响。

## 7. 备注（预扣不落盘）

预扣 403 拒绝携带 `ErrOptionWithNoRecordErrorLog()`，**不写 oneapi 日志、不写 DB error log、无 SysLog**。取证只能靠客户端错误串反推（余额 × `QuotaPerUnit` 定位用户）。排查同类问题时请让用户提供完整错误行。
