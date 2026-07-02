# 25 · `[1m]` 长上下文后缀定价归一

> 适用范围：客户端使用 `xxx[1m]` 长上下文档位后缀模型名时的定价解析。
> 关联坑点：#135。

## 1. 设计原则（红线）

1. **定价匹配先归一**：面向计费的模型名匹配必须先消除仅表示档位的后缀，避免同一基础模型因请求名变体落入兜底价。
2. **归一同价，KISS**：`xxx[1m]` 与 `xxx` 使用同一套倍率、价格、补全倍率和音频倍率；不为长上下文后缀维护第二套默认价格。
3. **一处归一，多处受益**：只在 `FormatMatchingModelName` 入口归一，所有经它的定价 getter 自动一致。
4. **不要前端镜像匹配**：前端只展示后端解析后的模型记录字段，不复制后端的模型名 → 倍率匹配逻辑。

## 2. 根因链

`claude-fable-5[1m]` 这类长上下文档位模型名没有独立配置倍率时，旧链路如下：

1. `FormatMatchingModelName` 不归一尾部 `[1m]`。
2. `GetModelRatio` 用原始 `claude-fable-5[1m]` 查 `ModelRatio` miss。
3. miss 后进入 `37.5` 兜底哨兵（`setting/ratio_setting/model_ratio.go:414`，等价 `$75/1M`，是基础倍率 `5` 的 `7.5×`）。
4. `ModelPriceHelper`（`relay/helper/price.go:96`）按兜底倍率预扣，导致额度虚高；未开启 `AcceptUnsetRatioModel` 时会因价格未配置直接拒绝。
5. 结算侧继续按未归一模型名取倍率，预扣与结算都可能偏离基础模型真实价格。

## 3. 修复策略

修复点放在 `FormatMatchingModelName` 首行：先调用 `stripContextWindowSuffix`，剥离尾部大小写不敏感的 `[1m]` 后再进入既有匹配逻辑。

- **覆盖预扣**：`ModelPriceHelper` 调用定价 getter 时自动复用归一结果，`xxx[1m]` 命中 `xxx` 的基础倍率/价格。
- **覆盖结算**：`service/text_quota.go` 使用 `OriginModelName` 进入倍率解析，同样经 `FormatMatchingModelName` 归一，结算倍率与预扣一致。
- **覆盖 getter**：`GetModelRatio`、`GetModelPrice`、`GetCompletionRatio`、`GetAudioRatio` 等所有经 `FormatMatchingModelName` 的定价读取都同步受益。
- **覆盖模型族**：规则只认通用尾部 `[1m]`，不绑定 `fable`；`opus`、`gemini`、`fable` 等同类后缀都按基础模型同价处理。

## 4. 前后对比

场景：`170k prompt + max_tokens 64000`，`groupRatio=1.0`，基础模型 `claude-fable-5` 已配置倍率 `5`，请求模型为 `claude-fable-5[1m]`。

| 项 | 修复前 | 修复后 |
|---|---|---|
| 解析倍率 | `37.5` 兜底 | `5`（归一 `claude-fable-5`） |
| 预扣额度 | `(170000+64000)×37.5=8,775,000`（约 `$17.55`）或直接拒绝 | `(170000+64000)×5=1,170,000`（约 `$2.34`） |
| 结算倍率 | `37.5` 虚高 | `5` 一致 |
| 修正 | — | 降 `7.5×`，按价格准确估计 |

## 5. 测试

- `setting/ratio_setting/model_ratio_test.go` 全绿。
- `stripContextWindowSuffix` 覆盖率 `100%`。
- 覆盖点包括：尾部 `[1m]` 剥离、大小写不敏感、整串等于 `[1m]` 不剥离、非结尾 `[1m]` 不剥离。

## 6. 前端说明

无需改动 Classic / Default。

`web/default/src/features/pricing/lib/price.ts` 的定价展示基于后端已解析的 `model` 记录字段渲染，不存在 `FormatMatchingModelName` 的模型名 → 倍率镜像匹配逻辑。因此后端归一对前端透明，前端只会看到后端给出的最终价格字段。

## 7. 影响面与边界

- **整串等于 `[1m]` 不剥离**：避免把模型名变成空串。
- **`[1m]` 非结尾不剥离**：只处理长上下文档位后缀，避免误伤中间含义。
- **显式登记 `xxx[1m]` 独立倍率会被旁路**：当前决策是归一同价，`xxx[1m]` 会回退使用 `xxx` 的基础价；如未来要支持独立价，需要先改变“同价”产品决策。
- **`service/quota.go:hasCustomModelRatio` 仍用原始名比较**：仅影响日志展示中的自定义倍率判断，无计费影响。

## 8. 生产取证

实例 `micu-prod-do-us-1/new-api-third`：

- `ModelRatio` 仅有 `"claude-fable-5":5`，没有 `[1m]` 变体。
- `SelfUseModeEnabled=false`。
- 近 7 天 `claude-fable-5[1m]` 成功日志 `0` 条。

该配置下，未归一的 `claude-fable-5[1m]` 无法复用基础模型倍率，容易落入 `37.5` 兜底哨兵或被“价格未配置”路径拒绝；归一后可直接复用 `claude-fable-5` 的已配置价格。
