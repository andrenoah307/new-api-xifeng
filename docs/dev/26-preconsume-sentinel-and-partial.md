# 26 · 预扣去 37.5 哨兵 + 优雅部分预扣

> 适用范围：`controller/relay.go` 文本/多模态请求的预扣费估算与钱包/令牌硬门控。
> 关联坑点：#136（tiered/价格检测需与 ratio 一致做模型名归一）、#137（预扣去 37.5 哨兵 + 优雅部分预扣）。
> 关联：[08-stream-billing.md](08-stream-billing.md)（结算口径）、[25-model-suffix-pricing-normalization.md](25-model-suffix-pricing-normalization.md)（`[1m]` 定价归一）。

## 1. 设计原则（红线）

1. **哨兵不是价格**：`GetModelRatio` 未命中时返回的 `37.5`（=$75/1M）是「请配置我」的保护哨兵，**不得作为真实倍率驱动预扣估算**，否则预扣虚高 `7.5×+` 误拒。
2. **检测一致归一**：面向计费的模型名匹配（ratio / price / **billing_mode / billing_expr**）必须统一走 `FormatMatchingModelName`，否则后缀变体（`gpt-5.5[1m]`）在某一处 miss 就会掉进兜底。
3. **预扣是门槛不是收费**：预扣只为「确保余额能覆盖本次成本」，真实计费以结算为准；余额/令牌不足以覆盖最坏估算但能覆盖**输入下限**时应放行，而非硬拒。
4. **有界走负**：部分预扣后结算可短暂走负，靠下一请求 `userQuota<=0` 兜底，单账号至多一次，语义与受信任用户一致。

## 2. 根因（生产取证 micu-prod-do-us-1 / new-api-third）

预扣估算把 `GetModelRatio` 的未配置哨兵 `37.5` 当真实倍率。两条入口：

1. **tiered/价格模型的后缀变体走漏**：`setting/billing_setting/tiered_billing.go` 的 `GetBillingMode` / `GetBillingExpr` 是**裸 map 查找、无 `FormatMatchingModelName` 归一**。客户端发 `gpt-5.5[1m]` 等变体 → `GetBillingMode` 返回 `ratio` 默认 → `ModelPriceHelper` 走 ratio 路径 → `GetModelRatio` miss → `37.5` 哨兵。
2. **真正未配置倍率的可路由模型** + 用户开 `AcceptUnsetRatioModel` → 不报「价格未配置」，带 `37.5` 继续。

结算侧正确（tiered 表达式 / 真实倍率，`gpt-5.5` 实测 `model_ratio:0` 走 `p*5+c*30+cr*0.5` 表达式，单请求几分钱），但预扣按 `37.5` 虚高 → `controller/relay.go:187 PreConsumeBilling` → `service/billing_session.go` 硬门控 `userQuota < preConsume → 预扣费额度失败`。

**数值反推**（`QuotaPerUnit=500000`，`USDExchangeRate=1`）：案例 `¥84.639440 = 42,319,720 quota`，唯 `ratio≈37.5 × group≈1.6 → prompt≈449k` 契合；`opus-4-8(2.5)` 需 1058 万 tokens、`gpt-5.5` tiered 需 2000 万+ prompt，均不可能。当事人 `id 40755`（`¥80.6388`）精确命中。**影响面**：单个 ~3.4h 日志文件 **1362 次**「预扣费额度失败」。

## 3. 修复

### F1（坑点 #136）：billing 检测归一
`GetBillingMode` / `GetBillingExpr` 先精确查 map，miss 时用 `ratio_setting.FormatMatchingModelName(model)` 归一后再查一次（`billing_setting → ratio_setting` 无循环）。使 `gpt-5.5[1m]` 正确识别为 tiered，不再落 ratio 兜底。

### F2（坑点 #137）：未配置哨兵不驱动预扣
`relay/helper/price.go:ModelPriceHelper` 非价格分支，`GetModelRatio` 返回 `!success`（哨兵 37.5）且经 `AcceptUnsetRatioModel` 放行时，`QuotaToPreConsume` 改用**保守小额** `int(PreConsumedQuota × groupRatio)`，不用 37.5。`ModelRatio` 字段保留（结算/日志口径不变）。

### 优雅部分预扣（坑点 #137）
- `types/price_data.go` 增 `QuotaToPreConsumeMin`（仅输入的预扣下限）。三路径计算：ratio 路径 = `promptOnly × ratio`；价格路径 = full；tiered = 表达式 `C=0`；未配置/免费 = 保守值/0。
- `service/billing.go:PreConsumeBilling` 加 `minPreConsumedQuota` 形参；`controller/relay.go` 传 `priceData.QuotaToPreConsumeMin`；`relay/relay_task.go` task 传 `Quota` 作 min。
- `service/billing_session.go` 新增纯函数 `computePartialTarget(userQuota, tokenQuota, tokenUnlimited, full, min) (target, ok)`；`tryWallet` 非受信任分支改用它：
  - `余额 ≥ full` → 全额预扣；
  - `min ≤ 余额 < full` → **部分预扣** `min(full, 余额)`；
  - `余额 < min` → 拒（连输入都付不起）；
  - 令牌侧同构收敛（限额 key 场景）。
- 复用 `shouldTrust`（`ForcePreConsume=true` 不旁路），不回归坑点 #124 / #127。结算 `DecreaseUserQuota` 可有界走负。

## 4. 前后对比（案例2：后缀变体 / 未配置模型，prompt≈449k，group 1.6）

| 项 | 修复前 | 修复后 |
|---|---|---|
| tiered 检测 | 裸查 miss → ratio 路径 | 归一命中 → 正确 tiered 预扣 |
| 未配置倍率预扣 | 37.5 哨兵 → ¥84.64 | 保守小额 ~¥0.001 |
| 余额 ¥80.64 请求 | 硬拒「预扣费额度失败」 | 放行，结算回真（几分钱） |
| 临界（余额略低于估算） | 整单拒 | 余额 ≥ prompt 成本即部分预扣放行 |

## 5. 测试

- `setting/billing_setting/tiered_billing_test.go`：`GetBillingMode`/`GetBillingExpr` 后缀归一（`GetBillingMode` 覆盖 100%）。
- `service/billing_session_test.go:TestComputePartialTarget`：8 例覆盖 full/部分/输入下限拒/令牌收敛/无限额令牌/`min==full` 保留硬门控/零下限（`computePartialTarget` 覆盖 100%）。
- `relay/helper/price_test.go`：未配置→保守小额、价格路径 min=full、tiered min（C=0）< full 共 3 例。

**基线既有失败（非本次引入）**：`relay/helper/stream_scanner_test.go:TestStreamScannerHandler_StreamStatus_PreInitialized` 基线即 FAIL（坑点 #127 已登记），与本修复无关。

## 6. 前端

**无需改动 Classic/Default**：纯后端计费门控，前端定价展示由后端下发的 model 记录字段驱动，无 `FormatMatchingModelName` 镜像匹配，对前端透明。

## 7. 风险

- 部分预扣使单请求可短暂走负（有界，下一请求 `userQuota<=0` 兜底）；与受信任用户既有语义一致。
- 未配置模型保守小额预扣后，若结算按 37.5 实扣，低余额用户可能单次走负——**真实未配置模型仍建议管理员在「分组与模型定价设置」中配价**。
