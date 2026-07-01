# 23 · 透传请求体与可观测字段补写

> 适用范围：OpenAI Chat Completions / OpenAI Responses API / Gemini 兼容路径开启「请求头透传模板」与「透传请求体」后，使用日志 `other.reasoning_effort` 的捕获口径。
> 关联坑点：#134（透传分支跳过 convert，导致 convert 内写回 `info` 的可观测字段丢失）。

## 1. 现象

渠道类型为 OpenAI，开启「请求头透传模板」并开启「透传请求体」（`pass_through_body_enabled`）后，`/v1/chat/completions` 与 `/v1/responses` 请求可正常转发，但使用日志无法获取思考等级：`other.reasoning_effort` 缺失。

| 条件 | 表现 | 影响 |
|---|---|---|
| OpenAI 渠道 | 请求头透传模板开启 | 进入透传处理分支 |
| 透传请求体 | `pass_through_body_enabled=true` | 请求体不再经过 OpenAI request convert |
| 请求携带思考等级 | body `reasoning_effort=high` / `reasoning.effort=high` 或模型带 `gpt-5-high` 后缀 | 请求上游可用，但使用日志缺少 `reasoning_effort` |

## 2. 根因链

本质问题不是日志层漏写，而是透传分支跳过 convert 后，所有依赖 convert 写回 `info` 的可观测字段都不会被填充；`reasoning_effort` 是首例暴露出来的字段。

| 环节 | 文件/位置 | 结果 |
|---|---|---|
| OpenAI Chat 透传分支 | `relay/compatible_handler.go` 使用 `requestBody = common.ReaderOnly(storage)` | 整段跳过 `adaptor.ConvertOpenAIRequest` |
| OpenAI Chat 赋值点 | `relay/channel/openai/adaptor.go:338`，在 `ConvertOpenAIRequest` 内且仅在 `o*` / `gpt-5` 分支 | `info.ReasoningEffort` 的赋值点不执行 |
| OpenAI Responses 透传分支 | `relay/responses_handler.go` 的 `/v1/responses` 透传分支 | 跳过 `adaptor.ConvertOpenAIResponsesRequest` |
| OpenAI Responses 赋值点 | `relay/channel/openai/adaptor.go:597-599`，优先级为模型后缀 > `request.reasoning.effort` | `info.ReasoningEffort` 的赋值点不执行 |
| RelayInfo 状态 | `info.ReasoningEffort` | 恒为空字符串 |
| 日志生成 | `service/log_info_generate.go:47` 的 `if relayInfo.ReasoningEffort != ""` | 条件不成立，`other` 不写 `reasoning_effort` |

## 3. 相似问题：Gemini 透传路径

`relay/gemini_handler.go` 的透传分支与 OpenAI 兼容路径同构：透传请求体时跳过 `CovertOpenAI2Gemini`，而 Gemini 的思考等级写回点在 `relay/channel/gemini/relay-gemini.go:195`（`info.ReasoningEffort = level`）。

| 路径 | 被跳过的 convert | convert 内写回字段 | 透传后结果 |
|---|---|---|---|
| OpenAI Chat 兼容 | `ConvertOpenAIRequest` | `info.ReasoningEffort` | 使用日志缺少 `reasoning_effort` |
| OpenAI Responses | `ConvertOpenAIResponsesRequest` | `info.ReasoningEffort` | 使用日志缺少 `reasoning_effort` |
| Gemini | `CovertOpenAI2Gemini` | `info.ReasoningEffort = level` | 使用日志同样缺少 `reasoning_effort` |

## 4. 修复口径

修复限定为**仅透传分支补写**，不改非透传路径，不改 convert 本身，避免影响既有请求转换、计费与日志口径。

| 变更点 | 口径 | 说明 |
|---|---|---|
| 新增纯函数 | `relay/common/reasoning_effort.go` | 只解析思考等级，不触碰请求体转换 |
| OpenAI Chat 透传补写 | `ResolveOpenAIReasoningEffortForPassthrough(upstreamModel, requestEffort)` | 优先级为模型后缀 `ParseOpenAIReasoningEffortFromModelSuffix` > body `reasoning_effort`，与 `ConvertOpenAIRequest` 一致 |
| OpenAI Responses 透传补写 | `ResolveResponsesReasoningEffortForPassthrough(upstreamModel, reasoning)` | nil 安全，内部复用 OpenAI 解析，优先级为模型后缀 > `request.reasoning.effort` |
| Gemini 透传补写 | `ResolveGeminiReasoningEffortForPassthrough(upstreamModel)` | 从 `TrimEffortSuffix` 支持的模型后缀解析等级 |
| Handler 接入 | `relay/compatible_handler.go` / `relay/responses_handler.go` / `relay/gemini_handler.go` | 仅在透传分支写回 `info.ReasoningEffort`；Responses 仅 `RelayModeResponses` 分支调用，compact 分支不处理 |

## 5. 前端展示

两套前端均已消费 `other.reasoning_effort`，且 i18n key `Reasoning Effort` 已存在；本次不新增翻译 key。

| 前端 | 文件/位置 | 展示口径 |
|---|---|---|
| Default | `web/default/src/features/logs/components/details-dialog.tsx:773` | `StatusBadge` 三色展示 |
| Classic | `web/classic/src/helpers/data/useUsageLogsData.jsx` | 由纯文本对齐为 Semi `Tag` 着色：`high=orange`、`medium=amber`、其余 `green` |
| i18n | Default 6 语 / Classic 8 语 | `Reasoning Effort` key 已存在，无新增 key |

## 6. 前后对比

生产只读实证（`micu-prod-do-us-1`，OpenAI 透传体渠道 122/188/189/224）如下；`/v1/chat/completions` 为上一轮修复部署后口径，`/v1/responses` 为本轮补齐的缺口，待新代码部署后复验。

| 路径 | 部署前 `other.reasoning_effort` | 部署后 `other.reasoning_effort` | 结论 |
|---|---|---|---|
| `/v1/chat/completions` | 0/13713 | 2528/8412（medium1500 / high838 / xhigh281 / low34 / none15，全合法） | 已验证生效 |
| `/v1/responses` | 0/63698 | 0/34619 | 本轮修复补齐，待部署复验 |

| 场景 | 修复前 `other.reasoning_effort` | 修复后 |
|---|---|---|
| OpenAI Chat + 透传 + `reasoning_effort=high` | 无 | `high` |
| OpenAI Responses + 透传 + `reasoning.effort=high` | 无 | `high` |
| OpenAI + 透传 + 模型 `gpt-5-high` 后缀 | 无 | `high`（后缀解析） |
| OpenAI 不透传 + `reasoning_effort` | 有 | 不变，无回归 |
| Gemini + 透传 + 模型 `-high` 后缀 | 无 | `high` |
| Claude 原生 `thinking(budget)` | 无 | 无，超出本次范围未纳入 |

## 7. 测试

`relay/common/reasoning_effort_test.go` 使用表驱动覆盖三个纯函数，函数级覆盖率均为 100%；Responses 追加 5 例覆盖 nil 安全、模型后缀优先与 `request.reasoning.effort` fallback。

| 函数 | 覆盖点 | 预期 |
|---|---|---|
| `ResolveOpenAIReasoningEffortForPassthrough` | 模型后缀优先、body fallback、空值 | 与 `ConvertOpenAIRequest` 优先级一致 |
| `ResolveResponsesReasoningEffortForPassthrough` | nil reasoning、模型后缀优先、`reasoning.effort` fallback、空值 | 与 `ConvertOpenAIResponsesRequest` 优先级一致 |
| `ResolveGeminiReasoningEffortForPassthrough` | `TrimEffortSuffix` 支持的后缀、无后缀 | 仅从模型后缀解析等级 |

## 8. 规范红线

任何透传分支只要跳过 convert，就必须显式补齐 convert 内写回 `info` 的可观测字段（`reasoning_effort` 等），否则使用日志会失真。新增透传路径必须把这一项纳入回归检查。
