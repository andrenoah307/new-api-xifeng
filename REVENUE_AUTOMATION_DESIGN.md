# 收入定时汇总与邮件通知

实现日期：2026-11-01。默认关闭；部署本代码不会主动启用、补算或发送邮件。
本次未修改生产配置，未发送实际邮件。以下描述第一版实际实现及限制。

## 2026-11-05 每次执行读取最新成本配置

每个新执行开始时读取最新系统计价单位、渠道成本倍率和高峰规则，保存为执行级
policy 快照，然后才查询或创建汇总任务。渠道配置变化不再要求重新启用自动化。
执行中的补算、重启和重试继续使用该执行快照，避免同一份报告混用不同单位或倍率。
所有渠道均未配置有效补全成本时，按无补全模式执行，未知成本仍保持未知。

旧版本已开始的执行沿用原配置中的冻结 policy；升级不会重算历史结果或自动重发
失败/不确定邮件。此前失败的执行仍需显式重试，下一次新执行自动采用最新配置。
快照保存在现有执行 data JSON 中，不迁移数据库；policy 上限128 KiB，计入执行
整体256 KiB预算。手工汇总的配置确认和指纹冲突保护不变。

## 2026-11-04 调度兼容性修复

执行历史清理查询必须同时设置 `LIMIT` 与 `OFFSET`；MySQL 不接受单独的
`OFFSET 127`，即使执行历史为空也会使首次调度失败。现按每批最多 128 条清理，
固定保留最新 127 条后插入本次记录，所有批次仍在同一租约事务中执行。
遇到 `unknown` 投递记录会回滚此前批次的删除，不推进 `next_at`。

SQLite、隔离 MySQL 8.2 / PostgreSQL 18 回归覆盖首次到期执行、重复调度复用、
257 条历史的滚动清理、未知投递保护及调度时间回滚。无需表结构迁移。
部署修复后，仍启用且保留到期 `next_at` 的配置会由调度器补执行；七天以上的
漏执行仍沿用原有失败保护。不要通过关闭再开启配置来修复，否则会重置下一次时间。

## 页面与权限

Default、Classic 的收入页均新增「定时任务与通知」独立页签，实时查询仍为默认页。
采用一套全站调度配置，包含预统计周期、执行时间、邮件周期、收件人、字段、主题和正文。
仅启用状态的系统超级管理员 role=100 可读写；后台执行也重新核查配置所有者权限。
保存使用 revision CAS，陈旧配置返回冲突；启用时复用渠道成本确认弹窗。
运行中不允许修改启用配置，可先关闭；重新启用从下一个执行时间开始。

界面提供执行历史、任务依赖 ID、冻结字段值、各收件人发送状态、失败重试和结束操作。
结果不确定的邮件重试必须确认可能重复发送。模板修改不自动补发历史通知。
重试/结束记录操作者ID；关闭时处于发送流程的记录保守转为unknown，仍可在关闭后明确结束。
已修改配置版本的旧执行只能结束，不能按新配置重新解释旧结果。

## 时间与调度

默认北京时间 04:00，可改为当天其他分钟；使用固定 UTC+8，不受服务器或浏览器时区影响。
所有统计区间为 `[start,end)`。自然周从周一开始，月、季度和年使用日历运算。

| 预统计周期 | 执行日期 | 窗口 |
| --- | --- | --- |
| 日 | 每日 | 昨日 00:00 至今日 00:00 |
| 周 | 周一 | 上周一至本周一 |
| 月 | 每月 1 日 | 上一自然月 |
| 季 | 1、4、7、10 月 1 日 | 上一自然季 |
| 年 | 1 月 1 日 | 上一自然年 |

同一天依次处理日、周、月、季、年，再处理邮件字段所需窗口。重复窗口合并。
邮件可按日、周、月、季、年发送，其字段可以各自引用不同窗口和渠道。
字段的相对日期以原定执行时间为锚点，不因排队或发送重试而移动。
自定义相对窗口支持前 1 至 366 个完整自然日，不支持任意公式或未结束周期。

协调器复用系统任务运行器的 15 秒调度节拍，在独立有界执行中推进，每次最多 60 秒；
不为每次轮询创建 system_tasks 行。主库单例租约防止多实例同时处理。
到期创建执行行并推进 next_at 在同一事务完成，scheduled_at 唯一索引防止重复日期。
重启恢复已持久化执行；若未处理触发点落后超过七天，记录明确失败并转至下一次执行，
不会静默对多年历史进行追赶。历史缺口需超级管理员显式创建汇总。

## 数据来源与口径

消费 = known_revenue；成本 = original_cost + fallback_cost；利润 = 消费 - 成本；
利润率 = 利润 / 消费。只统计已知成本子集，未知成本同时从消费与成本中排除。
消费为零时利润率为 `-`，负利润和真实零成本保留。金额使用 decimal，展示六位小数。
邮件单位沿用报表的 `quota / quota_per_unit`，不额外执行汇率换算或改变计费额度。
时间覆盖完整与成本覆盖率是两个独立概念，字段提供成本覆盖率占位符。

每次新执行开始时服务器冻结当前渠道成本倍率、高峰规则、计价单位和算法版本。
已有相容汇总直接复用，不扫描消费日志；新增查询入口允许限定必须匹配的冻结 policy，
避免从其他已覆盖但不同倍率的结果中取数。渠道名、密钥和原始请求不进入邮件。
跨来源、跨倍率、跨单位的结果不能相加，旧整段金额不能按时间比例拆分。

没有完整覆盖时，复用现有汇总执行器创建该完整窗口的全站汇总，串行补算。
已有完整日结果可直接组成周/月/季/年查询，无需每天重新扫描全年日志。
第一版缺口补算以整个所需窗口为单位，并非精确最小缺口规划；已经完整的窗口不重算。
沿用原单小时执行、30 秒窗口超时和 ClickHouse 读取/线程/内存上限，不修改业务转发。
手工汇总运行时自动任务等待；创建使用来源＋冻结 policy 的稳定 UUID，事务记录自动归属。
倍率配置改变后，当前执行继续按其快照补算，下一次新执行采用变化后的配置。
暂停/失败的依赖不会自动循环创建新任务，需要在汇总任务页处理后重试。

04:00 不是源日志已完整到齐的证明。迟到日志、导入或来源变化不会自动修正已发送邮件；
仍需显式重算。发送前重新验证各窗口完整覆盖，再冻结字段、范围、正文和收件人状态。

## 模板与占位符

默认模板沿用工单邮件的信息表和简洁外壳。超级管理员可以编辑主题和 HTML 正文。
普通变量在正文中转义，只有服务器构建的 summary_table 作为安全 HTML 插入。
主题不允许换行；未知、未闭合占位符在保存/预览时拒绝。
预览为示例数据，不扫描日志、不发起汇总；浏览器 iframe 无脚本和同源授权，CSP 禁止外部资源。
测试邮件只发给当前操作者绑定邮箱，经过显式点击及接口限流，不使用正式收件人列表。

| 占位符 | 内容 |
| --- | --- |
| `{{system_name}}` | 系统名 |
| `{{scheduled_at}}` | 计划执行时间，UTC+8 |
| `{{generated_at}}` | 结果生成时间，UTC+8 |
| `{{run_id}}` | 执行记录 ID |
| `{{summary_table}}` | 字段、结果、实际时间窗口、成本覆盖率表格 |
| `{{fields.yesterday_profit.value}}` | 对应字段的金额或百分比 |
| `{{fields.yesterday_profit.start}}` | 实际开始时间 |
| `{{fields.yesterday_profit.end}}` | 实际结束时间，不包含该时刻 |
| `{{fields.yesterday_profit.unit}}` | 冻结的报表计价单位 |
| `{{fields.yesterday_profit.coverage}}` | 已知成本记录数 / 消费记录数 |

字段标识支持中文等 Unicode 字母开头，后续允许字母、数字和下划线，最长 48 个字符，不允许重复、空白、点或模板括号；最多 20 个字段。例如 `昨日利润` 可通过 `{{fields.昨日利润.value}}` 引用。每个字段选择指标、窗口和渠道 ID（0 为全站）。
收件人最多 10 个，逐人发送，互不暴露其他收件人。主题上限 200 个前端字符/800 字节校验，
正文 32 KiB，渲染消息 128 KiB，执行快照合计 256 KiB。

## 邮件可靠性

财务邮件采用新的有超时 SMTP 发送函数，复用现有 MIME 报文与认证配置；工单发送行为不变。
连接 10 秒、整体 30 秒上限，支持现有 TLS/STARTTLS 设置。自动发送同一时刻只推进一个收件人。

| 状态 | 处理 |
| --- | --- |
| pending | 待发送 |
| sending | 已落库发送意图，开始 SMTP；进程中断后转 unknown |
| accepted | 收到 DATA 成功响应；QUIT 断连不再当失败 |
| retryable | DATA 前可确定未提交的失败，按 1/5/30 分钟最多重试三次 |
| rejected | 收件人/认证/配置拒绝，或重试次数耗尽 |
| unknown | DATA 后断连、确认丢失或发送中进程退出，禁止自动重发 |

发送和数据库不能组成一个事务，因此不能承诺 exactly-once。未知状态必须由超级管理员
核对后明确重试或结束；已接受的收件人在同一执行重试中不会再次发送。
重试读取冻结正文，不重新计算统计窗口。租约丢失后的旧执行不能覆盖数据库状态。
关闭配置不会撤回 SMTP 已接收的邮件，也不会取消其他手工汇总。

## 存储上限与清理

仅新增三张小表：revenue_automations（单行配置/租约）、revenue_automation_runs
（执行与每人发送状态）、revenue_automation_jobs（自动报表归属）。不新增逐请求数据表。

| 数据 | 上限与策略 |
| --- | --- |
| 原统计检查点 | 全局 10,000 行 / 128 MiB 原有逻辑预算，保持不变 |
| 自动执行快照 | 最多 128 行，每行 256 KiB；最旧终态滚动清理 |
| 自动生成报表 | 最多 500 个，每个 policy+rows 最多 256 KiB |
| 单行配置 | 配置+旧执行兼容 policy 最多 256 KiB |

自动报表达到 500 个时，仅清理最旧且超过一年加七天、执行器已结束并释放锁的自动报表，
同事务归还检查点预算、删除对应自动系统任务及归属记录。没有可安全回收的记录则停止新增，
不会突破上限。单条自动报表超限时拒绝发布该窗口，不通过拆更多记录绕过限制。
未解决的 unknown 执行不能自动淘汰，达到历史容量时停止创建新执行，保留核对证据。
控制记录不计入“10,000 个统计检查点”这一口径，界限不能混淆为所有数据库表共一万行。
手工报表沿用原生命周期，不擅自删除；原始 logs、用户额度、订单、计费和转发数据完全不动。
容量是逻辑记录/载荷限制，不承诺数据库文件、索引、WAL 或 binlog 立即回收空间。

使用 GORM 跨库类型；快照避免 MySQL TEXT 的 64 KiB 上限，使用 size 生成相应长文本类型。
租约事务显式产生字段变化，避免 MySQL 同秒同值更新 RowsAffected=0 被误判失锁。
SQLite 新建表可能改变日志来源 schema fingerprint，不强行修改历史来源标识来绕过验证。

## 接口与代码

- `GET/PUT /api/channel-finance/automation`：读取/保存配置及执行摘要。
- `POST /api/channel-finance/automation/mail/preview`：无副作用示例预览。
- `POST /api/channel-finance/automation/mail/test`：明确发送测试到操作者邮箱。
- `GET /api/channel-finance/automation/runs/:id`：冻结结果、收件人和依赖详情。
- `POST /api/channel-finance/automation/runs/:id/retry`：重试，未知发送需 confirm_duplicate_risk=true。
- `POST /api/channel-finance/automation/runs/:id/dismiss`：结束失败/未知执行。

主要实现：model/revenue_automation*.go、service/revenue_automation.go、
common/revenue_email.go、controller/revenue_automation.go 和两主题 automation 页面。
只在系统后台任务节拍增加独立协调器，不接入请求转发、用户结算、充值或监控缓存计算链路。

## 验证

日期测试覆盖闰月、跨年自然周、季度、年度、UTC+8 和相对日数边界；
模型测试覆盖默认关闭、revision/租约 CAS、降权、滚动上限及不淘汰未知发送。
服务集成测试验证历史汇总直接复用、缺失成本排除、负利润/利润率和成本覆盖率，
以及发送中断恢复为 unknown 而非重发。接口测试验证普通管理员和陈旧 root 会话均不可见。
本地假 SMTP 验证 DATA 确认丢失、收件人拒绝、DATA 已接受后 QUIT 断连。
隔离 MySQL/PostgreSQL 验证迁移、较大快照、同秒连续事务和租约排他。
两主题构建、定向lint和1440/390浏览器通过；model/service/controller/common全包及收入定向race通过。
Default全量typecheck保留既有103条诊断，收入模块无新增；model/service/controller vet通过，
common vet保留既有CustomEvent锁值复制和旧邮件测试IPv6地址诊断。未修无关代码。
不包含生产部署或正式邮件试发。

## Built-in Chinese Report Template

Both themes offer an explicit "Use Chinese report template" action. It replaces
only the editor's subject/body; saving and sending remain separate actions. Existing
saved templates are not migrated or overwritten. Sample preview is sandboxed and
uses clearly marked demonstration data, without sending mail.

The new `{{channel_reports}}` placeholder reuses each configured field's published
query result. Identical time-window/channel scopes are rendered once. Each section
shows known-cost consumption, snapshot plus supplemented cost, profit and margin,
with a signed, color-coded HTML bar chart (top 10 by absolute profit) and up to 100
channel detail rows, identified by channel ID. Totals include all eligible channels,
even when display limits apply. Empty/zero-consumption results and negative profits
remain explicit. Time boundaries are Beijing time, end-exclusive; amounts retain
the report's quota-per-unit basis rather than assuming a currency conversion.

The HTML needs no scripts, remote image service or image download. Large reports
stop appending further windows after approximately 40 KB and show an omission
notice; existing 128 KB email and 256 KB execution-snapshot guards still apply.
No new source-log scans, scheduled jobs, database migrations or mail-send side
effects are introduced by template application/preview.
