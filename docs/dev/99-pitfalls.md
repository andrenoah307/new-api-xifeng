# 踩坑预案汇总

---

### #76 工单交叉校验竞态：pre-check + create 双重校验

| 模块 | 触发条件 | 严重度 |
|------|----------|--------|
| controller/ticket.go | 退款/发票创建时 | P2 |

**问题**：用户多 tab 操作时，tab A 执行 pre-check 返回无冲突，同时 tab B 提交了发票/退款工单。如果创建接口不做二次校验，tab A 的提交将绕过冲突检查。

**解法**：`CreateRefundTicket` 和 `CreateInvoiceTicket` controller 内部必须在提交时再查一次 `GetUserInvoiceSummaries` / `GetUserActiveRefundSummaries`。有冲突但请求无 `acknowledged` 标记时返回 400。前端的 pre-check 只是 UX 优化，后端才是安全屏障。

**相关代码**：`controller/ticket.go:CreateRefundTicket`、`controller/ticket.go:CreateInvoiceTicket`

---

### #77 GetAdminRefundDetail 返回结构变更

| 模块 | 触发条件 | 严重度 |
|------|----------|--------|
| controller/ticket.go, api.ts, TicketAdmin/index.jsx | 管理员查看退款工单详情时 | P1 |

**问题**：`GetTicketRefund` API 原来返回 `{ refund: {...} }`，现在返回 `{ refund: {...}, user_invoices: [...] }`。Default 前端 `getAdminRefundDetail` 的消费方式从 `.data?.data?.refund` 改为 `.data?.data`（返回完整对象），Classic 前端 `loadRefundDetail` 需要额外提取 `data.user_invoices`。两套前端必须同步更新，否则 `refundData` 结构不匹配导致退款详情无法渲染。

**解法**：API 返回结构变更必须同时更新所有消费端。Default 前端的 `ticket-admin-detail.tsx` 改为 `refundData.refund` 访问退款记录，Classic 前端的 `TicketAdmin/index.jsx` 新增 `userInvoices` state 提取。

**相关代码**：`controller/ticket.go:GetTicketRefund`、`api.ts:getAdminRefundDetail`、`TicketAdmin/index.jsx:loadRefundDetail`

---

### #78 退款方式精简：表单选项与显示标签必须分离

| 模块 | 触发条件 | 严重度 |
|------|----------|--------|
| model/ticket_refund.go, constants.ts, ticketUtils.js | 精简收款方式下拉选项时 | P2 |

**问题**：移除微信/其他收款方式后，如果同时删除后端的 `refundPayeeTypeText` 和前端的显示映射，历史退款记录中 `payee_type=wechat` 或 `payee_type=other` 的工单将无法正确显示收款方式名称。

**解法**：分离"表单选项"（用户可选的值）和"显示标签"（管理员/用户查看的值）。后端 `IsValidRefundPayeeType` 仅允许 `alipay`/`bank`；`refundPayeeTypeText` 保留所有历史类型的中文映射。前端用独立的 `PAYEE_TYPE_OPTIONS`（表单用）和 `PAYEE_TYPE_LABELS`（显示用），Classic 用 `REFUND_PAYEE_TYPE_OPTIONS`（下拉框）和完整 `REFUND_PAYEE_TYPES`（显示）。

**相关代码**：`model/ticket_refund.go:IsValidRefundPayeeType`、`model/ticket_refund.go:refundPayeeTypeText`、`constants.ts:PAYEE_TYPE_OPTIONS/PAYEE_TYPE_LABELS`、`ticketUtils.js:REFUND_PAYEE_TYPE_OPTIONS`

---

### #79 Alert 组件 warning 变体：对话框内提醒统一规范

| 模块 | 触发条件 | 严重度 |
|------|----------|--------|
| alert.tsx, create-ticket-dialog.tsx, create-invoice-ticket-dialog.tsx | 对话框内展示冲突提醒时 | P3 |

**问题**：shadcn Alert 仅有 `default` 和 `destructive` 两种变体。冲突提醒不是错误（destructive 语义），也不是纯信息（default 无视觉突出），需要中间态的 warning 样式。使用 `destructive` 会让用户误以为出错而放弃操作。

**解法**：CVA 新增 `warning` 变体（amber 配色 + dark mode 适配），对话框内的冲突提醒统一改用 `variant="warning"`。InvoiceHistoryAlert 因需控制宽度（max-w-2xl），不使用 Alert 组件而用自定义 div + 相同 amber 配色。

**相关代码**：`alert.tsx:alertVariants`、`create-ticket-dialog.tsx`、`create-invoice-ticket-dialog.tsx`、`refund-detail.tsx:InvoiceHistoryAlert`

---

### #92 下载 403：window.open / 邮件链接无法携带自定义请求头

| 模块 | 触发条件 | 严重度 |
|------|----------|--------|
| middleware/auth.go, router/api-router.go, export-tasks-sheet.tsx | 用户点击下载按钮或邮件中下载链接时 | P1 |

**问题**：`authHelper` 两阶段认证——Stage 1 验 cookie session，Stage 2 验 `New-Api-User` 请求头（CSRF 防护）。`window.open()` 和邮件客户端点击链接只发送 cookie，不发自定义头，Stage 2 返回 403/401。

**解法**：
- **站内下载**：改用 `fetch()` + blob 下载，通过 `getCommonHeaders()` 携带 `New-Api-User` 头。三处都改：Default 用户端、Classic 用户端、Default 管理端。
- **邮件链接**：下载路由从 `UserAuth()` 改为 `SessionAuth()`（仅验 cookie，跳过 CSRF 头检查）。`SessionAuth` 仍设置 `c.Set("id", ...)` 和 `c.Set("role", ...)`，rate limiter 和 ownership 校验正常工作。仅用于幂等只读 GET 资源，与工单附件下载同模式。

**相关代码**：`middleware/auth.go:SessionAuth`、`router/api-router.go:458`、`export-tasks-sheet.tsx:handleDownload`、`ExportTasksModal.jsx:onClick`、`admin-export-tab.tsx:onClick`

---

### #93 前端构建缓存：Go embed 导致代码修改不生效

| 模块 | 触发条件 | 严重度 |
|------|----------|--------|
| main.go, web/default/dist | 修改前端代码后直接 go build 部署 | P1 |

**问题**：`main.go` 的 `//go:embed web/default/dist` 在编译时将前端静态文件嵌入 Go 二进制。如果只修改了前端源码但没有重新 `bun run build`，`dist/` 目录仍是旧版本，Go 二进制打包旧文件。表现为"代码已提交但线上行为不变"。

**解法**：前端代码修改后部署流程必须：`cd web/default && bun run build` → `go build`。可通过检查 `dist/` 文件时间戳 vs 源码最后修改时间排查。CI 中确保 build 步骤在 go build 之前。

---

### #94 Classic 自动分组字符串指标：独立于数值指标的完整链路

| 模块 | 触发条件 | 严重度 |
|------|----------|--------|
| web/classic/src/pages/AutoGroup/index.jsx | 添加 current_group 等字符串类型条件指标时 | P2 |

**问题**：Classic 前端自动分组条件编辑器仅支持数值类型指标——操作符固定 6 个（>=, >, <=, <, ==, !=），值输入固定 InputNumber。`current_group` 是字符串类型指标，需要不同的操作符（仅 ==, !=）和值输入（分组选择器 Select）。

**解法**：新增 `STRING_METRICS` Set 和 `STRING_OPS` 数组。条件行渲染根据 `STRING_METRICS.has(metric)` 切换：操作符用 `STRING_OPS` / `OP_OPTIONS`；值输入用分组 Select / InputNumber。指标切换时通过 `handleMetricChange` 重置 op/value/value_str，防止数值条件的残留值污染字符串条件。`formatConditionsSummary` 对字符串指标展示 `value_str` 而非 `value`。

**相关代码**：`web/classic/src/pages/AutoGroup/index.jsx:STRING_METRICS`、`handleMetricChange`、条件行渲染 (line 822+)

---

### #95 自动分组保存报"无效的参数"：前端预序列化 conditions 与后端类型不匹配

| 模块 | 触发条件 | 严重度 |
|------|----------|--------|
| web/classic/src/pages/AutoGroup/index.jsx, controller/auto_group.go | Classic 前端创建/更新/启禁用规则时 | P1 |

**问题**：Classic 前端 `submitRule` 用 `JSON.stringify(ruleForm.conditions)` 将条件数组预序列化为字符串发送，但后端 `autoGroupRuleRequest.Conditions` 类型为 `[]model.AutoGroupCondition`（Go slice）。Go JSON decoder 遇到字符串 `"[{...}]"` 无法解码为 slice → `DecodeJson` 返回 error → controller 返回 "无效的请求参数"。`toggleRuleEnabled` 同理。

**解法**：前端不做 `JSON.stringify`，直接发送 `conditions: ruleForm.conditions`（JSON 数组）。后端 controller 负责验证后用 `common.Marshal(req.Conditions)` 重新序列化为字符串存入 DB。`toggleRuleEnabled` 用 `safeParseJSON(record.conditions)` 将 API 返回的字符串解析回数组再发送。

**相关代码**：`controller/auto_group.go:15-23`（请求结构体）、`AutoGroup/index.jsx:submitRule`、`toggleRuleEnabled`

---

### #96 Classic 运营设置父组件缺少初始 key → 子组件 useEffect 过滤后 inputsRow 丢失 key → compareObjects 跳过

| 模块 | 触发条件 | 严重度 |
|------|----------|--------|
| web/classic/src/components/settings/OperationSetting.jsx, SettingsMonitoring.jsx | 保存任何在父组件初始 state 中缺失的设置项时 | P1 |

**问题**：Classic 父组件 `OperationSetting.jsx` 的初始 `inputs` state 缺少 `AutomaticDisableWhitelist`。子组件 `SettingsMonitoring.jsx` 的 `useEffect([props.options])` 会触发两次——第一次用初始父 state（无此 key），将子组件 `inputs` 和 `inputsRow` 都重置为不含此 key 的对象。第二次（API 数据到达后）虽然 `props.options` 有此 key，但 `Object.keys(inputs).includes(key)` 检查的是第一次重置后的 `inputs`（已无此 key）→ 过滤掉 → `inputsRow` 仍无此 key。用户编辑后 `inputs` 有此 key 但 `inputsRow` 没有 → `compareObjects` 要求两个对象都 `hasOwnProperty` → 跳过 → "没有修改什么"。

**解法**：父组件初始 state 必须包含所有子组件需要的 key。新增设置项时，同时更新父组件初始 state 和子组件初始 state。排查：比对子组件表单字段与父组件 `inputs` 初始值。

**相关代码**：`OperationSetting.jsx` 初始 state、`SettingsMonitoring.jsx:useEffect`、`utils.jsx:compareObjects`

---

### #97 Default 前端 baselineRef 与 defaultValues 不同步：查询刷新后 dirty check 失效

| 模块 | 触发条件 | 严重度 |
|------|----------|--------|
| web/default/src/features/system-settings/integrations/monitoring-settings-section.tsx | 保存设置后重新加载数据时 | P2 |

**问题**：`baselineRef` 仅在 `useRef` 初始化和 `onSubmit` 成功后更新，不随 `defaultValues` prop 变化同步。保存后 `useUpdateOption` 触发 query invalidation → 服务器返回标准化值（如小写转换）→ `useResetForm` 重置表单为服务器值 → 但 `baselineRef` 仍为用户提交的原始值 → 下次 dirty check 比较错位。

**解法**：添加 `useEffect(() => { baselineRef.current = normalizeDefaults(defaultValues) }, [defaultValues])` 使 baseline 随服务器数据同步。

---

### #98 CSV 导出缺少 Content 详情列：manage/topup 记录导出后全为空值

| 模块 | 触发条件 | 严重度 |
|------|----------|--------|
| service/log_export_worker.go, controller/log.go | 导出包含 manage/topup 类型日志时 | P2 |

**问题**：`Log.Content` 是 manage/topup/system 类型记录的唯一有效信息字段（如"管理员为用户 xxx 修改额度"、"充值 100 元"），但 CSV 导出仅包含 token name、model name、quota 等 relay 相关列，manage/topup 记录的这些字段全为零值，导出后该行几乎全为空。

**解法**：三处 CSV 写入统一追加"详情"列，映射 `log.Content`：
1. 离线导出 worker（`service/log_export_worker.go:generateExport`）
2. 管理员在线导出（`controller/log.go:ExportAllLogsCsv`）
3. 用户在线导出（`controller/log.go:ExportUserLogsCsv`）

**相关代码**：`service/log_export_worker.go:222-258`、`controller/log.go:212-232`、`controller/log.go:259-276`

---

### #99 导出文件命名无意义：export-{id}.csv.gz 无法识别来源

| 模块 | 触发条件 | 严重度 |
|------|----------|--------|
| controller/log_export.go, 4 处前端下载 handler | 用户下载导出文件时 | P3 |

**问题**：导出文件下载名为 `export-{taskId}.csv.gz`（如 `export-42.csv.gz`），下载到本地后无法区分不同导出任务的来源用户和时间。多个文件混在一起时更难辨别。

**解法**：
- **后端**：`serveExportFile` 通过 `buildDownloadFilename(task)` 构建 Content-Disposition 文件名为 `{username}-{YYYYMMDD-HHmmss}.csv.gz`。`sanitizeFilename` 过滤文件系统非法字符，截断 50 字符。无用户名时回退到 `user-{userId}`。
- **前端**：4 处 blob 下载（Default admin-export-tab、Default export-tasks-sheet、Classic AdminExportPanel、Classic ExportTasksModal）的 `a.download` 同步使用 task 对象的 `username` + `created_time` 构建相同格式文件名。

**相关代码**：`controller/log_export.go:serveExportFile`、`admin-export-tab.tsx:421`、`export-tasks-sheet.tsx:57`、`AdminExportPanel.jsx:102`、`ExportTasksModal.jsx:109`

---

### #100 HiddenModels 精确匹配 vs 子串匹配——模型名是精确标识符

| 模块 | 触发条件 | 严重度 |
|------|----------|--------|
| controller/pricing.go, setting/operation_setting | 管理员配置隐藏模型后 | P2 |

**问题**：`AutomaticDisableKeywords` 使用 Aho-Corasick 子串匹配（大小写不敏感），如果 `HiddenModels` 照搬此逻辑，`gpt-4` 会误匹配 `gpt-4o`、`gpt-4-turbo` 等模型。模型名是精确标识符，不适合模糊匹配。

**解法**：
- **精确匹配**：`hiddenModelsSet = map[string]struct{}`，`IsModelHidden(name)` 直接 map 查找，O(1) 且大小写敏感。
- **Controller 层过滤**：`filterHiddenModels` 在 `controller/pricing.go` 的 `GetPricing()` 中执行，位于 `filterPricingByUsableGroups()` 之后。不修改缓存层——模型在 `model.GetPricing()` 缓存中仍存在，管理员通过其他接口仍可见。
- **即时生效**：`updateOptionMap` 中 `HiddenModelsFromString` 在设置保存时重建 map，下次 API 请求即生效，无需等待缓存刷新。
- **并发安全**：先构建新 slice/map 再赋值（与 `AutomaticDisableKeywords` 同模式），无需 mutex。

**新增设置项清单**（通用）：
1. `setting/operation_setting/` 变量 + 解析函数
2. `model/option.go` InitOptionMap + updateOptionMap case
3. Default 前端：`types.ts` 类型 + `operations/index.tsx` defaultValues + section-registry build + UI 组件
4. Classic 前端：`OperationSetting.jsx` 父 state + `SettingsGeneral.jsx` 子 state + UI

**相关代码**：`setting/operation_setting/operation_setting.go`、`model/option.go:219,675`、`controller/pricing.go:filterHiddenModels`

---

### #101 common.DeepCopy 反射拷贝已移除——浅拷贝 + targeted clone 替代

| 模块 | 触发条件 | 严重度 |
|------|----------|--------|
| relay/*.go (8 个 handler) | 每次 API 请求 | P0 |

**问题**：`common.DeepCopy` 使用 `jinzhu/copier` 的 `CopyWithOption(&dst, src, copier.Option{DeepCopy: true})`，通过反射递归拷贝 struct 全部字段。生产 pprof 显示此调用占 CPU 52.9%（19s/35.9s 采样）。实际只有 `Model`（string）和极少数 `SystemPromptOverride` 路径需要保护 `info.Request` 不被重试间共享污染。

**解法**：
- **浅拷贝**：`copied := *req; request := &copied`——Go struct value copy，所有值类型字段独立，slice/map/pointer 共享 backing data。栈分配，零反射，零 alloc。
- **Targeted clone**：仅在确认会 in-place mutation 共享 slice 元素的路径做 `make` + `copy`：
  - `applySystemPromptIfNeeded`（`chat_completions_via_responses.go`）：`SystemPromptOverride` 路径修改 `Messages[i].Content`，clone Messages slice
  - `compatible_handler.go` 第二个 system prompt block：同上模式
  - `gemini_handler.go`：`SystemPromptOverride` 路径修改 `SystemInstructions.Parts[i].Text`，clone SystemInstructions struct + Parts slice
- **安全分析**：`copier` 的 `DeepCopy: true` 对 `any`/`interface{}` 字段不做深拷贝（复制接口值而非底层对象）。`json.RawMessage` 是 `[]byte`，浅拷贝共享 backing array，但 handler 仅序列化（只读）不 mutate。浅拷贝在行为上与原 DeepCopy 等价。
- **依赖清理**：`common/copy.go` 已删除，`jinzhu/copier` 已从 `go.mod` 移除。

**新 handler 规范**：
1. 禁止引入 `jinzhu/copier` 或其他反射拷贝库
2. 使用 `copied := *req; request := &copied` 模式
3. 修改 slice 元素（非替换整个 slice）前，先 `make` + `copy` clone 该 slice
4. 替换 slice/pointer 字段本身（`request.X = newValue`）不需要 clone——浅拷贝的字段是独立的

**相关代码**：`relay/responses_handler.go:58`、`relay/compatible_handler.go:34`、`relay/claude_handler.go:34`、`relay/gemini_handler.go:63`、`relay/embedding_handler.go:28`、`relay/rerank_handler.go:28`、`relay/audio_handler.go:26`、`relay/image_handler.go:31`、`relay/chat_completions_via_responses.go:52`

---

### #102 JSON 序列化性能三连优化——go-json + ParseContent 缓存 + gjson 字段提取

| 模块 | 触发条件 | 严重度 |
|------|----------|--------|
| common/json.go, dto/claude.go, middleware/distributor.go | 每次 API 请求 | P0 |

**问题**：DeepCopy 消除后，`encoding/json` 成为新瓶颈（占剩余 CPU 75%+）。三个独立问题叠加：
1. 标准库 `encoding/json.Unmarshal` 先 `checkValid`（全量扫描验证）再解析——双重扫描，`checkValid` 单独占 17.44% flat CPU
2. `ClaudeMessage.ParseContent()` 无缓存，同一消息被 `GetTokenCountMeta` 和 relay adaptor 各调用一次，每次 `Any2Type` 做完整 marshal→unmarshal 往返（累计 63 GB 分配）
3. `middleware.getModelFromRequest` 全量 `json.Unmarshal` 解析整个 request body 只取 `model` 字段（23.59% cum CPU），controller 随后再次全量解析同一 body

**解法**：
1. **`goccy/go-json` 替换**：`common/json.go` 底层改为 `goccy/go-json`（纯 Go，无 CGO 要求）。`common/utils.go:Any2Type` 改用 `common.Marshal/Unmarshal` 而非直接 `encoding/json`，统一走 go-json 路径。`encoding/json` 仅保留类型引用（`json.RawMessage` 等）。
2. **ParseContent/ParseSystem 缓存**：`ClaudeMessage` 加 `parsedContent` + `contentParsed` 缓存字段，`ClaudeRequest` 加 `parsedSystem` + `systemParsed`——首次调用 `Any2Type`，后续直接返回缓存。仿 OpenAI `Message.parsedContent` 模式。未导出字段不影响 JSON 序列化。
3. **gjson 字段提取**：`middleware.getModelFromRequest` 对 `application/json` 请求用 `gjson.GetBytes(body, "model")` / `gjson.GetBytes(body, "group")` 直接提取，不触发全量解析。非 JSON content-type（form/multipart）回退原 `UnmarshalBodyReusable`。`GetBodyStorage` 仍被调用以初始化 body cache，controller 后续解析不受影响。

**兼容性要点**：
- `goccy/go-json` 的 `RawMessage`、`Number` 等类型是标准库的 type alias，完全兼容
- `goccy/go-json` 已在 `go.mod` 中（gin indirect dependency）
- `gjson` 已在 `go.mod` 中（direct dependency）
- 缓存字段为 unexported（小写），不参与 JSON marshal/unmarshal
- `ParseContent()` 返回值签名不变 `([]ClaudeMediaMessage, error)`

**相关代码**：`common/json.go`、`common/utils.go:Any2Type`、`dto/claude.go:ClaudeMessage.ParseContent`、`dto/claude.go:ClaudeRequest.ParseSystem`、`middleware/distributor.go:getModelFromRequest`

### #112 法律合规三 checkbox 分项确认：取代单 checkbox 模型

- 旧：登录/注册一个 checkbox 同时同意「用户协议+隐私政策」
- 新：三个独立 checkbox（用户协议 / 隐私政策 / 服务条款）+ 三浮窗 + 必须打开浮窗点"我已阅读"才能勾选
- Default 用 shadcn `Dialog`，Classic 用 Semi `Modal`（必须 `centered + bodyStyle.overflowY + getPopupContainer`，Rule 6）
- 三项链接同指 `/user-agreement` 端点（用户已确认不新增后端 `TermsOfService` 字段）
- 父组件用派生变量 `allLegalAgreed` / `allAgreedToTerms`，禁止把 `requiresLegalConsent && !agreedToLegal` 散落到各 handler

**相关代码**：`web/default/src/features/auth/components/legal-consent.tsx`、`web/classic/src/components/auth/{LoginForm,RegisterForm}.jsx`

### #113 localStorage 文档版本 hash 失效：避免历史 hash 堆积 + 隐私模式降级

- key 格式 `legal-read:{docKey}:{hash8}`，hash 来自后端 `/api/user-agreement` 响应字段（`common.Sha1(content)[:8]`）
- `markRead` 必须先 `clearStaleEntries` 扫描同 docKey 旧 hash 删除，否则 storage 会堆 N 份历史已读
- 隐私模式 / 配额满抛异常必须 try/catch 降级为内存 `Map`（单会话），否则 LegalConsent 渲染崩溃
- 空 hash（文档未配置）→ checkbox `disabled` + 走 `consentRequired=false` 放行

**相关代码**：`web/default/src/features/auth/lib/legal-consent-storage.ts`、`web/classic/src/helpers/legalConsentStorage.js`

### #114 后端响应字段仅追加：保证旧前端兼容

- `/api/user-agreement` `/api/privacy-policy` 响应追加 `hash` 字段必须**只追加**，旧前端仍能 `data` 字段读到原始内容
- 文档为空时 `hash=""`，前端必须按"无需确认"分支走

**相关代码**：`controller/misc.go:GetUserAgreement`、`controller/misc.go:GetPrivacyPolicy`

### #115 Classic LoginForm 存量 Modal Rule 6 旧债

- `Modal.error`（密码警告）/ 微信登录 Modal / 2FA Modal 三处都缺 `bodyStyle.overflowY` + `getPopupContainer`
- 本次合规改造仅对新增 Legal Modal 严格遵守，旧三处遗留登记需单独 PR 修复

**相关代码**：`web/classic/src/components/auth/LoginForm.jsx`

### #116 登录多通道单点守卫：26 处守卫的单变量收敛

- Default sign-in（9）+ Default sign-up（4）+ Classic LoginForm（9）+ Classic RegisterForm（8，**本次补齐 OAuth/Telegram/WeChat 守卫，原本仅靠按钮 disabled 防御**）
- 收敛到单一派生变量 `allLegalAgreed` / `allAgreedToTerms`，新增 provider 时仅需 `if (!allAgreedToTerms) { showLegalConsentError(); return; }`
- grep `agreedToLegal\|agreedToTerms\|allLegalAgreed\|allAgreedToTerms` 一次性确认全部覆盖

**相关代码**：`web/default/src/features/auth/sign-{in,up}/components/*.tsx`、`web/classic/src/components/auth/{LoginForm,RegisterForm}.jsx`

### #117 系统设置数组字段持久化：BlockedCountries 序列化与回填

- 后端结构体字段 `BlockedCountries []string` 通过 `config.GlobalConfig.Register("cn_disclaimer", ...)` 注册，前端保存路径 `controller/option.go:handleConfigUpdate` 需要 `reflect.Slice` 分支兼容空字符串/null（参考 #110 reflect.Map 修复）
- 前端 `defaultSiteSettings['cn_disclaimer.blocked_countries']` 默认值用 `'["CN"]'` 而非 `'[]'`，避免 dirty check 误报
- Default `CnDisclaimerSection` 提交时把 textarea 多行内容 trim + 大写 + 序列化为 JSON 数组字符串
- Classic `OtherSetting.jsx` 的 `getOptions` 读取后必须把 JSON 数组反序列化为换行文本回填 textarea，否则用户首次打开看到的是 `["CN"]` 文本

**相关代码**：`setting/system_setting/cn_disclaimer.go`、`web/default/src/features/system-settings/site/cn-disclaimer-section.tsx`、`web/classic/src/components/settings/OtherSetting.jsx`

### #118 全站拦截组件挂载位置：根布局 + 路由豁免

- Default `CnDisclaimerGate` 必须挂载在 `__root.tsx:RootComponent` 内部、`Outlet` 之后（让 Dialog portal 覆盖全部路由内容）
- Classic `CnDisclaimerGate` 必须在 `App.jsx` 的 `<Routes>` 同级、`<SetupCheck>` 内部挂载
- **必须豁免 `/setup` 路由**，否则首次安装向导被锁
- Default 必须早于 `<Toaster>`，否则 Dialog 内 toast 被遮挡
- Gate 内部用 `useLocation()` 取 pathname，前缀匹配豁免清单

**相关代码**：`web/default/src/routes/__root.tsx`、`web/classic/src/App.jsx`、`web/default/src/features/cn-disclaimer/cn-disclaimer-gate.tsx`、`web/classic/src/components/CnDisclaimerGate.jsx`

### #119 不可关闭 Modal/Dialog 完整封装：三路关闭路径都要禁

- shadcn Dialog 默认 ESC + 遮罩点击 + 右上 X 三个关闭路径
- 全站拦截必须三路全禁：`onEscapeKeyDown={e=>e.preventDefault()} + onPointerDownOutside={e=>e.preventDefault()} + className='[&>button.absolute]:hidden'`
- Classic Semi Modal 用 `maskClosable={false} + closable={false} + escClose={false}`
- Modal footer 自定义渲染 checkbox + 确认按钮，禁用默认 OK/Cancel（默认按钮无法禁用直到 checkbox 勾选）

**相关代码**：`web/default/src/features/cn-disclaimer/cn-disclaimer-gate.tsx`、`web/classic/src/components/CnDisclaimerGate.jsx`

### #120 status payload 一次性下发拦截决策：避免前端二次 IP 探测

- `cn_disclaimer_required` 由后端 `controller/misc.go:GetStatus` 基于 `requestip.GetClientCountry(c)` 一次性算好下发
- 前端不再发请求探测 IP（避免 race / CDN 头丢失 / Tor 用户多次切换 IP 抖动）
- hash 算法：`Sha1(title+"|"+content+"|"+blockedCountries.join(","))[:8]`
- hash 必须包含 title + content + blocked_countries 三项，**但不能包含 silence_minutes**（管理员调静默时长不应强迫用户重新确认）

**相关代码**：`controller/misc.go:GetStatus`、`controller/misc.go:GetCnDisclaimer`

### #121 已读静默时长（cooldown）实现：unix 秒时间戳 + 服务端实时读取 silence_minutes

- localStorage value 必须存**确认时间戳（unix 秒）**而非简单 `'1'` 标志
- `isStillSilent(hash, silenceMinutes)` = `(now - ts) < silenceMinutes * 60`
- `silenceMinutes <= 0` 时永远 false（每次都弹）
- **关键**：`silenceMinutes` 从后端 `/api/cn-disclaimer` 响应实时取，前端不缓存这个数字到 storage——避免管理员调短 silenceMinutes 后旧本地"静默到期时间"反而还有效
- `markAcknowledged` 前调 `clearStaleEntries` 扫描清掉同 prefix 旧 hash 残留，防历史 hash 堆积撑爆 storage
- 隐私模式 localStorage 异常需 try/catch + 内存 Map fallback，否则 Gate 渲染崩溃

**相关代码**：`web/default/src/features/cn-disclaimer/lib/storage.ts`、`web/classic/src/helpers/cnDisclaimerStorage.js`

### #122 计费 usage 零计费兜底：必须覆盖所有非 OpenAI handler

| 模块 | 触发条件 | 严重度 |
|------|----------|--------|
| relay/channel/claude, relay/channel/gemini, service/text_quota.go | 上游返回 `output_tokens=0` + 响应内容为空时 | P1 |

**问题**：#94/#711e3fdc/#76b4f721 已经把 OpenAI 路径（`OaiStreamHandler` / `OpenaiHandler`）的"上游 usage 不全 → 零计费 + 标记 `LocalCountTokens`"逻辑修好了，但 **Claude/Gemini 路径没同步**：

- Claude `HandleStreamFinalResponse`（`relay-claude.go:833`）在 `CompletionTokens == 0` 时只调 `service.ResponseText2Usage(responseText, ...)` 估算 completion；如果 `responseText == ""`（上游收到 `message_start.input_tokens` 后没有任何 `content_block_delta`，只回了 `message_delta.usage.output_tokens=0`），fallback estimator `rune_count×1.5 = 0` 仍把 completion 估算成 0，但 **PromptTokens 已经保留来自 message_start 的巨大值**（生产观测平均 88w，最大 293w）。
- `calculateTextQuotaSummary` 按 `TotalTokens = PromptTokens + CompletionTokens = PromptTokens > 0` 走完整计费路径，结果 `Quota ≈ PromptTokens × ModelRatio × GroupRatio`。即便 `admin_info.local_count_tokens=true`，依然实扣百万级 quota。
- Gemini `geminiStreamHandler` 同样：`info.ReceivedResponseCount > 0 && responseText == ""` 时仍走 `ResponseText2Usage` fallback，相同陷阱。
- Gemini 非流式 `GeminiChatHandler` 在 candidates 非空但实际无任何文本/工具调用时也按 prompt × ratio 计费。
- `calculateTextQuotaSummary` 末尾的"保底 1 quota"规则（`!ratio.IsZero() && Quota == 0 → Quota = 1`）会覆盖上游 handler 的零计费意图——即便 handler 已经把 usage 归零并标 `LocalCountTokens`，由于 ratio>0 仍会被强制保底成 1。

**生产数据**：vip_2_cc + gpt-5.5 近 7 天 1559 条样本均为此模式（100% `is_stream=1` + `claude:true` + `prompt > 0 && completion = 0` + `local_count_tokens=true`），累计错扣 quota ≈ 12 亿。

**解法**：
1. **Claude 流式**（`HandleStreamFinalResponse`）：在现有 fallback 之后追加"empty stream"兜底——`CompletionTokens == 0 && ResponseText.Len() == 0` → `PromptTokens/TotalTokens/PromptTokensDetails/ClaudeCacheCreation5m/1h` 全部归零 + `SetContextKey(ctx, ContextKeyLocalCountTokens, true)`。
2. **Claude 非流式**（`HandleClaudeResponseData`）：抽 `claudeResponseHasContent(resp *dto.ClaudeResponse)` helper，遍历 `Content[]`，检查 `tool_use`/`server_tool_use`/`GetText()`/`GetStringContent()`；返回 false 且 `OutputTokens == 0` → 整份 usage 归零 + 标 `LocalCountTokens`。
3. **Gemini 流式**（`geminiStreamHandler` line 1318-1324）：`ReceivedResponseCount > 0 && responseText.Len() > 0` 才走 `ResponseText2Usage` fallback；否则 `usage = &dto.Usage{} + LocalCountTokens`。
4. **Gemini 非流式**（`GeminiChatHandler`）：抽 `openaiResponseHasContent(resp *dto.OpenAITextResponse)` helper，检查 `Choices[].Message.ToolCalls`/`GetReasoningContent()`/`StringContent()`；`usage.CompletionTokens == 0 && !openaiResponseHasContent(resp)` → `usage = dto.Usage{} + LocalCountTokens`。
5. **通用兜底**（`service/text_quota.go:calculateTextQuotaSummary`）：保底 1 quota 规则增加 `LocalCountTokens` 守卫——`!ratio.IsZero() && Quota == 0 && !common.GetContextKeyBool(ctx, ContextKeyLocalCountTokens)` 才保底 1，避免上游 handler 的零计费意图被绕过。

**新 handler 规范**：
- 新增非 OpenAI 渠道（Bedrock/Vertex/OpenRouter Claude 路径等）的 stream/non-stream handler **必须**在 usage 解析路径末尾加 empty-output 兜底，不能只依赖 `ResponseText2Usage` 估算补 completion。
- `ResponseText2Usage` 仍是"流被中断 / 上游 usage 字段缺失"场景的正常 fallback；但调用方**必须**先判断"响应是否真的有产出"——`ResponseText.Len() == 0` 时直接走零计费分支，不要喂空字符串给估算器再问"为什么估算结果还是 0"。
- 任何在 handler 内部归零 usage 的路径都**必须**配套设置 `ContextKeyLocalCountTokens`，给 `calculateTextQuotaSummary` 的保底守卫提供信号。
- 历史的"保底 1 quota"规则只适用于 ratio>0 但实际 quota 极小（被 round 0）的合法路径，**不**适用于零计费意图。

**相关代码**：`relay/channel/claude/relay-claude.go:HandleStreamFinalResponse`（833-）、`relay/channel/claude/relay-claude.go:claudeResponseHasContent`、`relay/channel/claude/relay-claude.go:HandleClaudeResponseData`、`relay/channel/gemini/relay-gemini.go:geminiStreamHandler`、`relay/channel/gemini/relay-gemini.go:openaiResponseHasContent`、`relay/channel/gemini/relay-gemini.go:GeminiChatHandler`、`service/text_quota.go:calculateTextQuotaSummary`

**单测**：
- `service/text_quota_test.go`：`TestCalculateTextQuotaSummarySkipsMinimumQuotaWhenLocalCountTokens` / `TestCalculateTextQuotaSummaryKeepsMinimumQuotaWithoutLocalCountTokens` / `TestCalculateTextQuotaSummarySkipsMinimumWhenLocalCountTokensButTokensPresent`
- `relay/channel/claude/relay_claude_zero_charge_test.go`：`TestHandleStreamFinalResponseZeroChargesEmptyOutput` / `TestHandleStreamFinalResponseKeepsUsageWithResponseText` / `TestClaudeResponseHasContent*`
- `relay/channel/gemini/relay_gemini_zero_charge_test.go`：`TestOpenaiResponseHasContent*`

### #128 Lost Update：整行 / 整 hash 覆盖原子计数列（余额竞态）

**漏洞类**：TOCTOU / Lost Update 竞争条件（来自 `tmp/newapi-lost-update-poc`，PoC 估 CVSS 7.5-9.0）。

**原理**：计费扣费在 DB 与 Redis 两层都走**原子**操作——
- DB：`gorm.Expr("quota - ?")`（user）/ `used_quota + ?`（channel）/ `remain_quota - ?`（token）；
- Redis：`RedisHIncrBy(key, field, delta)`（user `Quota` / token `RemainQuota`）。

但任何**"读整行 → 改少数字段 → 整行/结构体写回"**（GORM `Save` 写全列；`Updates(struct)` 写全部非零字段）或**"整 hash 覆盖 Redis"**（`RedisHSetObj` 用结构体快照逐字段 `HSET`）的路径，都会把请求开始时读到的**旧计数值**覆盖回去，抹掉这期间并发扣费的原子增减。

**PoC 利用**：高并发 `PUT /api/user/self`（sidebar_modules / language 分支）反复触发 `User.Update`，旧 `quota` 快照覆盖 DB + 缓存，把余额"钉死"在旧值；同时用 API key 铸币 → **免费调用**。

**机制澄清**：PoC 文案写的是"乐观锁 version 冲突回滚"，但本仓库 **`users` 表没有 version 列**，真实机制是更朴素的 stale full-row overwrite。两层（DB + Redis）各是一个独立的 lost-update 向量。

**重要边界**：token 消费**同时扣 `user.quota`**；故修好 `user.quota` 后，channel `used_quota`（统计/展示）与 token `remain_quota`（子额度）的覆盖**不再产生"免费额度"**，属正确性 / 统计 / 缓存一致性加固，非新的免费铸币口子。

**修复（dev-20260512 本会话）**：

| 沉点 | 文件 | 修法 |
|------|------|------|
| `User.Update` | `model/user.go` | `.Omit(userProtectedColumns...)` + 结尾 `updateUserCache`→`invalidateUserCache` |
| `User.Edit` / `User.ClearBinding` | `model/user.go` | 结尾 `updateUserCache(*user)`→`invalidateUserCache(user.Id)`（DB 写本就走 map/单列白名单，仅缓存侧覆盖 `Quota`） |
| `Channel.Update` / `Channel.Save` / `SaveWithoutKey` | `model/channel.go` | `.Omit("used_quota","balance","balance_updated_time")`（`SaveWithoutKey` 由 `UpdateChannelStatus` 在自动封禁 / 压力冷却 / 中继报错热路径触发；`Save` 由 `GetSetting`/`GetOtherSettings` 损坏 JSON 自愈触发） |
| `GetTokenById`(读路径,无条件覆盖) / `Token.Update` / `SelectUpdate` | `model/token.go` | `cacheSetToken`→`cacheDeleteToken(token.Key)`（整 hash 覆盖 `RemainQuota`，对抗 `cacheDecrTokenQuota`） |

`userProtectedColumns`（`model/user.go`）= `quota, used_quota, request_count, aff_count, aff_quota, aff_history, enforcement_*（9 列）, risk_warning_pending_at`。

**安全（无需改、审计已证伪）**：`GetUserCache` / `GetTokenByKey` 的缓存回填是 **miss-only**（`shouldUpdateRedis(fromDB,err)`），且 `RedisHIncrBy`/`RedisHSetField` 带 `ttl>0` 守卫——key 缺失时为 no-op，不会产生残缺 hash，自愈安全。`TransferAffQuotaToQuota` 走 `FOR UPDATE` 行锁事务（重读在锁内），DB 侧安全。

**规范（红线）**：
1. **新增原子自增/自减计数列**（用户 / 渠道 / Token / 任何实体）必须**同步登记**到该实体的"禁写整行"白名单（`userProtectedColumns` 或 Channel/Token 的 Omit/Select），并在所有整行 `Save` / 结构体 `Updates` / 整 hash `RedisHSetObj` 路径排除。
2. 资料 / 绑定 / 设置类更新**只改自己那几列**，绝不整行回写余额 / 计数列；优先 `map[string]interface{}` 或 `Select(白名单)`。
3. 缓存更新优先 **失效（invalidate / DEL）而非整 hash 覆盖**——`RedisHSetObj` 会覆盖并发 `RedisHIncrBy`；失效后下次读从库（已正确）重建。读路径**不应**无条件整 hash 回写一个被 `HIncrBy` 管理的 hash。
4. "set 绝对值"语义改一个被并发自减的余额（如 token `remain_quota` 编辑），**加锁也救不了**（绝对覆盖本身丢失消费），必须 **compare-and-swap**（客户端回传基线 + `WHERE col=baseline`）或拆成原子增量。

**暂缓（S2，需前端 CAS，全栈改动）**：`controller/token.go:UpdateToken` 用客户端绝对值覆盖 `remain_quota`（经 `Token.Update` 的 `Select` 白名单写 DB）。正确解法 = 前端 Classic/Default 回传 `original_remain_quota` 基线 + 后端 `WHERE remain_quota=baseline` 的 CAS，冲突提示刷新；待单独排期。

**仅记录不修（二级一致性，非 lost-update）**：`TransferAffQuotaToQuota`/`Redeem`/`Topup 充值` 给 DB 加额度后**不回写/失效 Redis** → 缓存偏低、TTL 自愈、向用户少报余额（保守、不影响安全）。

**审计**：本会话用对抗式工作流全仓扫描（18 候选 → 11 确认 / 5 证伪），上表为去重后确认项。

**测试**：`model/lost_update_test.go`
- `TestUserUpdate_DoesNotClobberAtomicColumns`：原子改 `quota/used_quota/request_count` 后，旧快照 `Update` 不覆盖。
- `TestUserEdit_DoesNotClobberQuota`：`Edit` map 白名单不触碰 `quota`（回归守卫）。
- `TestChannelUpdate_DoesNotClobberUsedQuota` / `TestChannelSave_DoesNotClobberUsedQuota` / `TestChannelSaveWithoutKey_DoesNotClobberUsedQuota`：三条整行写路径均不覆盖 `used_quota`。
- `TestMain` 迁移列表补 `&Ability{}`（`Channel.Update` 末尾 `UpdateAbilities` 需 `abilities` 表）。
- 缓存侧（invalidate）端到端需 Redis 才能验，单测仅覆盖 DB 侧不被覆盖。

### #129 低余额工单/开票周限流：UTC+8 自然周 + 前后端双门

**需求**：用户余额 < 5（货币单位，`quota < 5*QuotaPerUnit`=2,500,000）时，当前 UTC+8 自然周内只能新建一次工单/开票申请（general/invoice/refund 三类合并计数），超出提示「提交过于频繁，如需帮助请先联系客服咨询」。仅 `role>=RoleAdminUser(10)` 豁免（客服 5 不豁免）。

**实现要点**（完整设计见 [`docs/dev/20-low-balance-ticket-weekly-limit.md`](20-low-balance-ticket-weekly-limit.md)）：

1. **自然周必须按固定 UTC+8 算，不能用服务器本地时区**。用 `time.FixedZone("CST", 8*3600)`（`common/week.go`），免 tzdata 依赖；周一对齐复用 `subscription.go` 的 `Monday=1..Sunday=7` 口径（`weekday==0→7`，回退 `-(weekday-1)` 天）。`reset_at` = 下周一 00:00。**坑**：直接 `time.Unix(now,0)` 取的是进程本地时区，部署在 UTC 机器上会算错周界——必须显式 `.In(cstZone)`。

2. **三类工单合并计数**：general/invoice/refund 同存 `tickets` 表（`type` 区分），单条 `COUNT(user_id=? AND created_time>=weekStart)` 即覆盖三类，无需按 type 分别查。

3. **阈值严格小于**：`quota >= 阈值` 不限——余额恰好等于 5 不触发限流。豁免/高余额时 `Remaining=-1`（无限标记）。

4. **前后端双门（坑点 #76 复用）**：
   - 后端硬门 `controller/ticket.go:enforceWeeklyTicketLimit`，注入 `CreateTicket`/`CreateInvoiceTicket`/`CreateRefundTicket` 三处（紧跟 `getTicketCurrentUser` 之后），被限返回 `ApiErrorI18n(MsgTicketWeeklyLimit)`。
   - 前端 pre-check `GET /api/ticket/limit-status`（`GetTicketLimitStatus`）→ 双主题打开创建表单时禁用提交 + 内联提示。**仅前端禁用不够**：多 tab / 直接调 API 必须靠后端硬门。
   - 门控在 controller 层（与既有 invoice/refund 冲突校验同层）。残留多 tab 并发窗口与现有冲突校验等价；需强一致可加 per-user 顾问锁（本批未做）。

5. **计数无 soft-delete 绕过**：用户不能自删工单（仅删附件），故非删计数即可。

6. **i18n 三处**：后端 `i18n/keys.go`+`{en,zh-CN,zh-TW}.yaml`（`ticket.weekly_limit_exceeded`）；前端两套 `i18n/locales/*.json`（英文源串为 key，补齐全部 locale 避免缺键）。

**相关代码**：`common/week.go`、`model/ticket_limit.go`、`controller/ticket.go`、`router/api-router.go:GET /api/ticket/limit-status`、`web/{default,classic}` tickets 创建组件。

**测试**：`common/week_test.go`（周界，100%）、`model/ticket_limit_test.go`（豁免/阈值/首末次/合并/跨周，90–100%）。锚点 2024-01-01 00:00 UTC+8 = 周一 = unix 1704038400。

### #130 超大 logs 表稀疏类型查询：(type,created_at) 在线索引 + 保持排序

**现象**：管理员按非「消费」类型筛日志慢，「消费」快（完整方案见 [`docs/dev/21-logs-nonconsume-query-optimization.md`](21-logs-nonconsume-query-optimization.md)）。

**根因**：`logs` 无以 `type` 打头的索引（`idx_created_at_type` 的 type 是第二列）。消费占全表 ~99%，PK 倒序 `ORDER BY id DESC LIMIT n` 几下凑满 → 快；非消费**稀疏**，要扫过海量消费行才凑够一页（SELECT 慢），管理员全量 `COUNT` 也要扫遍窗内逐行验 type（COUNT 慢）。

**修法（不重建表）**：
1. **新增复合索引 `idx_logs_type_created_at (type, created_at)`**（type 打头）→ `WHERE type=X AND created_at∈窗` 直接 seek 该类型再范围扫，**只碰匹配行**，COUNT/SELECT 均精确且快。
2. **不要改全局 `ORDER BY`**：`GetAllLogs` / `GetUserLogs` 继续保持 `ORDER BY logs.id desc`。`(type,created_at)` 索引单独即可解决非消费慢查询：COUNT 走 type 前缀范围扫描，SELECT 取稀疏行后只对少量行做廉价 filesort。COUNT 保持精确（加索引后已快，不反转 #69）。
3. **排序回归教训**：把全局排序改成 `created_at desc, id desc` 会在用户日志路径（`user_id` 过滤、无 `(user_id,created_at)` 索引、重度用户千万行）触发灾难性 filesort，也会在管理员全类型路径退化为 1.37 亿行全表扫 + filesort。

**坑**：给查询加二级排序列前，必须确认每条调用路径都有可命中的 `(前缀等值列…, 排序列)` 索引，否则 filesort。

**索引必须在线加、且严守上线顺序（坑）**：
- **不能走 GORM AutoMigrate 直接建**——它发的是阻塞 `CREATE INDEX`，超大表会卡库。
- MySQL：`ALTER TABLE logs ADD INDEX idx_logs_type_created_at (type, created_at), ALGORITHM=INPLACE, LOCK=NONE;`（InnoDB online DDL，不重建表、并发读写）。
- PostgreSQL：`CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_logs_type_created_at ON logs (type, created_at);`（**必须事务外**）。
- **D2 上线顺序**：DBA **先**在线建索引（名必须 = `idx_logs_type_created_at`，与 gorm tag 一致），**再**部署带 tag 的代码 → AutoMigrate 见已存在即 no-op。**反序（先部署）= 启动时阻塞建索引卡线上**。日志库以 `LOG_SQL_DSN` 为准。

**收敛**：`SumUsedQuota`(`/api/log/stat`) 恒 `type=Consume`、本就在快路径，**无需改**；keyset 游标 / 服务端时间窗护栏在加索引后**已非必要**（索引使非消费含 OFFSET 也快且保留页码跳转），本批不做 → **前端零改动**。

**相关代码**：`model/log.go`（Log 索引 tag、`GetAllLogs`/`GetUserLogs` 保持 `ORDER BY logs.id desc`）、`model/main.go:migrateLOGDB`。

### #131 超大 logs 表查询器多过滤列：过滤列必须与 created_at 复合

**现象**：超大 `logs` 表的查询器常叠加实体过滤列（如 `group` / `username` / `token_name`）和时间窗；如果实体列只有单列索引、没有与 `created_at` 组成复合索引，优化器可能先按实体列扫整组/整人，再逐行判断时间窗，行数大时会退化成慢查询。

**修法**：对高频实体过滤列建立 `(过滤列, created_at)` 复合索引，或统一过滤列到已存在的复合索引路径。例如用户 self stat 不再按 `username` 查，而是按 `user_id` 查，命中 `(user_id,created_at)`；分组查询命中 `(group,created_at)`。

**坑**：给查询加二级排序列（如 `created_at desc, id desc`）前，必须确认该调用路径有可命中的 `(前缀等值列…, 排序列)` 索引；没有索引支撑时，二级排序会把原本可早停的路径拖成 filesort。

### #132 使用日志默认结束时间：当天 23:59:59，勿用 now+1h

**现象**：使用日志页默认结束时间历史上取 `当前时刻 + 1 小时`（为兜住“当前小时”数据），语义不直观，跨天临界易把明天纳入。

**修法**：默认结束时间改为「当天 23:59:59」（本地时区），配合后端 `created_at <= endTimestamp`（含右端）精确覆盖当天、排除次日。
- Classic：`web/classic/src/helpers/utils.jsx` 新增对称 helper `getTodayEndTimestamp()`（= 今日 0 点 + 86399s），`useUsageLogsData.jsx` 两处默认值改用它并移除多余 `now`。
- Default：`web/default/src/features/usage-logs/lib/utils.ts` 新增 `getEndOfToday()`（`setHours(23,59,59,999)`），`getDefaultTimeRange()` 调用之；经 `timestampToSeconds` 向下取整 → 同为今日 0 点 + 86399s，两端一致。

**坑（边界）**：后端过滤是 `<=`（含右端）。取「当天 23:59:59」正确——含当天全部秒、排除次日 0 点；若取「次日 00:00:00」则 `<=` 会多纳入恰好次日零点那 1 秒的记录，除非后端改 `<` 否则不要用。

**范围**：本次仅改「使用日志页」。dashboard / 绘图(mj) / 任务日志仍有同样 `+3600` 模式（`useDashboardData.js` / `useMjLogsData.js` / `useTaskLogsData.js` / `web/default/src/lib/time.ts`），如需统一另行处理。完整说明见 [`docs/dev/24-logs-default-end-time.md`](24-logs-default-end-time.md)。

### #135 [1m] 长上下文后缀定价归一：避免落 37.5 兜底哨兵

**问题**：`xxx[1m]` 未配置时，`GetModelRatio` miss 后落 `37.5` 兜底哨兵，预扣/结算可能 `7.5×` 虚高，未开 `AcceptUnsetRatioModel` 时也会直接拒绝。
**根因**：`FormatMatchingModelName` 过去不归一尾部大小写不敏感 `[1m]`，导致 `claude-fable-5[1m]` 不能复用 `claude-fable-5` 的倍率/价格。
**修复**：在 `FormatMatchingModelName` 首行调用 `stripContextWindowSuffix` 剥离尾部 `[1m]`，一处归一覆盖 `ModelPriceHelper` 预扣、`service/text_quota.go` 结算及所有经它的定价 getter。
**生产取证**：`micu-prod-do-us-1/new-api-third` 仅配置 `ModelRatio["claude-fable-5"]=5`、无 `[1m]` 变体，`SelfUseModeEnabled=false`，近 7 天 `claude-fable-5[1m]` 成功日志 `0` 条。
**关联文档**：见 [`docs/dev/25-model-suffix-pricing-normalization.md`](25-model-suffix-pricing-normalization.md)。

### #136 tiered/价格计费检测需与 ratio 一致做模型名归一（否则后缀变体落 37.5 兜底）

**问题**：`setting/billing_setting/tiered_billing.go` 的 `GetBillingMode` / `GetBillingExpr` 是裸 map 查找、无归一。客户端发 `gpt-5.5[1m]` 等后缀变体时检测不到 tiered → `ModelPriceHelper` 走 ratio 路径 → `GetModelRatio` miss → 命中 `37.5` 兜底哨兵，预扣虚高 `7.5×+`。
**根因**：`GetModelRatio`/`GetModelPrice`/`GetCompletionRatio` 都经 `FormatMatchingModelName` 归一，唯 `GetBillingMode`/`GetBillingExpr` 没有，检测口径不一致。
**修复**：两函数先精确查、miss 再用 `ratio_setting.FormatMatchingModelName` 归一查一次（`billing_setting → ratio_setting` 无循环）。
**生产取证**：`micu-prod-do-us-1/new-api-third`，`gpt-5.5` 为 `tiered_expr`（结算 `p*5+c*30+cr*0.5`），其后缀变体在预扣落 37.5。
**关联文档**：见 [`docs/dev/26-preconsume-sentinel-and-partial.md`](26-preconsume-sentinel-and-partial.md)。

### #137 预扣去 37.5 哨兵 + 优雅部分预扣（避免临界拒绝与末位余额不可花费）

**问题**：预扣把 `GetModelRatio` 未配置哨兵 `37.5` 当真实倍率 → 估算虚高 `7.5×+`，且 `service/billing_session.go` 硬门控 `userQuota < preConsume` 整单拒「预扣费额度失败」；低余额用户末位余额永远花不掉。单个 ~3.4h 生产日志 **1362 次**该拒绝。
**根因**：① `!success` 哨兵值驱动预扣（叠加 #136）；② 硬门控无优雅降级，余额略低于最坏估算即整单拒。
**修复**：F2——`ModelPriceHelper` 未配置分支 `QuotaToPreConsume` 改保守小额 `int(PreConsumedQuota×groupRatio)`，不用 37.5，`ModelRatio`/结算不变。优雅部分预扣——`types.PriceData` 增 `QuotaToPreConsumeMin`（仅输入下限）；`computePartialTarget` 纯函数：`余额≥full`→全额、`min≤余额<full`→部分预扣 `min(full,余额)`、`余额<min`→拒，令牌侧同构；结算可有界走负，下一请求 `userQuota<=0` 兜底；复用 `shouldTrust` 不回归 #124/#127。
**生产取证**：案例 `¥84.639440=42,319,720 quota` 唯 `ratio≈37.5×group≈1.6→prompt≈449k` 契合（`QuotaPerUnit=500000`，`USDExchangeRate=1`），当事人 `id 40755`。
**关联文档**：见 [`docs/dev/26-preconsume-sentinel-and-partial.md`](26-preconsume-sentinel-and-partial.md)。
