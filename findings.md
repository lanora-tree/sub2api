# Project Readiness Summary

## 当前判断

Sub2API 有条件适合作为本项目底座。它对 Codex、OpenAI Compatible API、多账号调度、Streaming、usage、并发、管理后台和 Compose 的覆盖程度很高，可以显著减少网关协议层工作。V1 仍不能直接进入多人或收费试运行，安全、账务、路由隔离和合规问题需要先关闭。

## 0.1.1-planning 评审修订

2026-09-06 完成一次跨文档复核并修正以下规划缺口：Internal Recharge header 漂移、source spoof 边界、nonce 验证顺序、M3/M5 依赖缺失、usage dedup 结果结构、价格区间与 V1 route 唯一约束、API Key pepper 轮换、Route 权限语义漂移、模型列表健康抖动、Billing degraded 重启恢复以及每日备份 RPO 过大。修订只改变设计文档，不代表相应代码、migration、部署或测试已经完成。

## M0 运行时发现

2026-09-12 已在 Windows Docker Desktop 上完成 M0。固定提交的生产镜像、PostgreSQL、Redis、Backend、内嵌 Frontend、后端 unit/integration、前端全量测试与适用平台的部署测试均通过。运行时结论如下：

1. 上游管理端在任何管理写操作前强制管理员本人确认 `v2026.06.10` 合规声明。管理员已在 UI 完成确认，数据库记录版本与用户；自动化没有代签或绕过门禁。
2. 正式 Fork `https://github.com/lanora-tree/sub2api` 已创建；本地 `origin` 与 `upstream` 已按项目规则配置，原 Fork 阻塞关闭。
3. `upstream/main` 于 2026-09-12 开始 M6 前重新抓取，仍为 `4726bdd08b6201d426a80529b79be123a4008d20`，已领先审计提交 349 个提交。M6 继续从已验收 M2 基线开发；上游同步必须在专用分支审阅，不能静默改变审计基线。
4. Windows CRLF 检出会使部分 POSIX 精确行匹配部署测试误报。实际配置语义未失败；临时 LF 副本全部通过。项目已补充 `.gitattributes`，保证后续检出一致。
5. M0 使用本地临时 OpenAI Compatible mock 验证 `gpt-5.4` 模型列表、非流式与 Streaming。两条请求均落入 `usage_logs` 并按上游原始 USD 语义扣费；模拟账号随后设为 inactive，临时容器和脚本已移除。

## M2 范围收口结论

2026-09-12 已完成 M2。数据库 migration 将 19 个范围外开关统一写为 `false`；前端菜单、直接路由与后台轮询改为明确 opt-in；后端对注册、优惠/邀请、支付与 Webhook、兑换/订阅、返利、可用渠道、公开监控和插件接口增加统一 fail-closed 守卫。全量 Backend、Frontend、production build 与本地升级库/HTTP 黑盒验收通过。

1. 后端而非前端是最终关闭边界，稳定响应为 HTTP 404、`FEATURE_DISABLED` 和 feature metadata。
2. Payment、Promo Code、Channel Monitor 等历史 fail-open/opt-out 口径已在 V1 范围内改为 opt-in；设置缺失或读取失败不会重新暴露入口。
3. 支付配置的 Admin GET/PUT 有意保留，以便管理员受控恢复；订单、套餐、Provider、公开查询和 Webhook 仍受支付总开关保护。
4. `239_v1_scope_defaults.sql` 基于当前上游最大 migration 编号 238 分配。上游同步仍须先检查文件名和 checksum 冲突。
5. M2 不删除功能源码，也不修改 Admin 创建 User、密码登录或 Gateway 主链路；M0 的运行证据与 M2 全量回归共同覆盖保留能力。

## M6 已确认决策

2026-09-12 产品负责人确认 CNY 为全站唯一权威记账币种。当前本地数据库只含 M0 无价值测试余额，因此 M6.1 可按数值 1:1 写入 CNY opening adjustment，不推导或使用汇率；未来若导入真实 USD 存量，必须另行审批汇率、基准时间与逐用户对账结果。

M6.1 代码审查确认：极小 usage 舍入到 `0.00000000` 时仍需写审计流水，因此 usage 允许零或负数；recharge/refund 只允许正数，普通 adjustment 必须非零，migration 专用 opening adjustment 可以为零。`users.balance` 与流水由同一事务、用户行锁和幂等 advisory lock 保护。

Windows Docker Desktop 4.68.0 的 AF_UNIX 套接字重解析点故障已于 2026-09-13 恢复：完整重启后，首次启动仍读取旧 `Docker\\run\\dockerInference`，在 Docker 进程退出后把精确 `Docker\\run` 目录移动为 `run.stale-after-reboot-20260912-221448`，再次启动即恢复 Linux Engine。未执行 factory reset，镜像、容器、卷和 `docker_data.vhdx` 未删除或移动；运行目录备份暂保留用于诊断。

M6.1 最终验证结果：真实 PostgreSQL 18 上的 migration、opening、不可变 trigger、并发更新、并发幂等重放、失败回滚和对账测试全部通过；本地升级库 2 个用户对应 2 条 opening 流水，差异用户数为 0。Linux Go 1.27 执行 `go test ./... -count=1` 全部通过，说明先前 5 个 Windows 失败属于平台抖动而非当前改动。

## 可以直接复用

1. Gin Gateway routes 与协议处理。
2. OpenAI OAuth、Codex Responses、Token refresh。
3. OpenAI 与通用 Scheduler、Sticky Session、previous response affinity。
4. Group、Account、优先级、并发、Cooldown 和健康状态基础。
5. PostgreSQL、Redis、Ent、SQL migration 和 Wire 依赖注入。
6. usage 解析、余额预检、账务 dedup 与事务扣款基础。
7. Admin、User、API Key、Usage、Settings 等页面与 API 框架。
8. Audit log、TOTP、step up、URL allowlist、trusted proxy 和 rate limit 基础。
9. Docker Compose、Healthcheck 和边缘安全文档。

## 需要修改

1. API Key 从明文改为 HMAC 摘要，只在创建时展示一次。
2. Account credentials 从 JSONB 明文改为独立密钥加密。
3. 金额热路径从 float64 改为 Decimal，明确 CNY 语义。
4. 把 usage log、余额和钱包流水纳入同一结算事务。
5. 扩展 Composite Route，增加 `target_group_id` 和 only explicit 模式。
6. 关闭 Group fallback，禁止跨成本池自动切换。
7. 前后端共同关闭公开注册、支付、套餐、优惠码、返利、第三方登录和无关入口。
8. User DTO 与页面做严格字段裁剪。

## 需要新增

1. `wallet_transactions`。
2. `model_pricing_rules` 与 `model_pricing_versions`。
3. `external_recharge_orders` 与 Internal Recharge API。
4. `user_model_permissions`。
5. 价格快照、钱包对账、Billing degraded 和相关告警。
6. V1 精简 Admin 与 User 页面。
7. Codex、Cursor、OpenAI SDK 的版本化验收矩阵。

## 最大的五个技术和项目风险

1. Codex Subscription 账号池可能违反上游账号共享、转售或 Usage Limit 规则，可能导致账号封禁和项目无法按当前模式运行。
2. 用户 API Key 与上游账号凭据当前可在数据库中以明文读取，数据库或备份泄漏会直接暴露可用 Secret。
3. float64、缺少完整钱包总账、价格版本和原子 usage log 会造成账单精度、追溯与对账风险。
4. Codex Subscription 与 OpenAI Official API 同属 `openai` 平台，当前 Composite Route 不能锁定目标 Group，存在误选高成本账号池的风险。
5. Codex、Cursor 和 DeepSeek 协议变化快。DeepSeek 已停用需求指定的两个旧模型名，客户端与模型兼容必须依赖真实版本测试。

## M0 结论

M0 已完成。宿主机未安装 Go 1.27，但固定的 Go builder 容器已成功执行 unit 与 integration 测试，因此工具链不再是阻塞。正式 Fork、固定镜像、全栈、管理员与普通用户登录、测试对象、模型列表、Gateway、Streaming 首包、用量落库和资源采样均有实际证据。M2 可以在该基线上开始。

## Codex 开发前需要人工确认

1. 是否拥有 Codex Subscription 多用户使用所需的上游授权，是否允许收费或成本分摊。
2. 如何解释和处理 Sub2API LGPL 3.0 与 README_CN“无商业授权”声明的关系。
3. CNY 已确认为唯一权威记账币种；未来若出现真实 USD 存量，其迁移汇率、时点和逐用户清单仍需人工批准。
4. DeepSeek 对外继续使用 `deepseek-chat`、`deepseek-reasoner` 逻辑别名，还是直接发布 V4 模型名。
5. OpenAI Official 和 Codex V1 具体开放模型清单与初始价格。
6. Admin 是否只允许 VPN 或固定 IP 访问。
7. 可接受 RPO、RTO、备份对象存储和 Secret Manager。

## 1. 审计基线

* 官方仓库：`https://github.com/Wei-Shaw/sub2api.git`
* Commit：`b1748c4ea99ce2120401a269142aa071e18a84da`
* Commit 时间：2026 年 9 月 3 日
* 当前最新 release tag：`v0.2.0`
* Backend Go 版本：`go 1.27.0`
* Frontend：Vue 3、TypeScript、Vite、pnpm
* Upstream Compose：PostgreSQL 18、Redis 8、Sub2API latest image

本报告以该 commit 的实际代码为主。官方外部文档只用于确认已经发生的 Provider 与条款变化。

## 2. 目录与启动

Backend 入口为 `backend/cmd/server/main.go`。启动流程支持 setup wizard 和 Docker auto setup，加载 config、logger、Wire app、插件与审计组件后启动 Server，并处理 graceful shutdown。

主要目录：

| 目录 | 职责 |
| --- | --- |
| `backend/internal/server/routes` | Gateway、Auth、User、Admin、Payment routes |
| `backend/internal/handler` | HTTP 与协议 handler |
| `backend/internal/service` | Gateway、Provider、Scheduler、Billing、Auth、Admin 业务 |
| `backend/internal/repository` | Ent、SQL、Redis 和加密基础设施 |
| `backend/ent/schema` | Ent schema |
| `backend/migrations` | Forward only SQL migrations |
| `frontend/src` | Vue User 与 Admin UI |
| `deploy` | Compose、Caddy、安装和安全文档 |

Makefile 提供 backend、frontend、test 和 build 聚合入口。Backend test 依赖 Go，部分 integration test 依赖 Docker。

## 3. Gateway 与协议

`backend/internal/server/routes/gateway.go` 已注册：

* `/v1/messages`
* `/v1/messages/count_tokens`
* `/v1/models`
* `/v1/usage`
* `/v1/live`
* `/v1/responses` 及子路径
* `/v1/chat/completions`
* `/v1/embeddings`
* Codex backend paths
* Gemini v1beta paths
* 多个 root compatibility aliases

结论：Responses 与 Chat Completions 的路由基础齐全。真实 Codex CLI 和 Cursor 仍需版本化实测，代码覆盖不能替代客户端验收。

## 4. OpenAI、OAuth 与 Codex

OpenAI 相关能力分散在 `backend/internal/service/openai_*`。现有实现包含 OAuth、refresh、quota、Responses、Chat Completions、WebSocket、Tool Calls、Reasoning、模型 mapping、错误转换和大量回归测试。

OpenAI Scheduler 位于 `openai_account_scheduler.go`，考虑 Group、模型能力、账号状态、priority、load、concurrency、previous response 和 session sticky。`openai_sticky_compat.go` 兼容新旧 sticky hash。`session_id.go` 解析多种会话请求头。

技术适配度高。主要阻塞来自上游条款和多用户共享场景授权。

参考：

* [OpenAI Codex Authentication](https://developers.openai.com/codex/auth)
* [OpenAI Services Agreement](https://openai.com/policies/services-agreement/)
* [OpenAI Terms of Use](https://openai.com/policies/row-terms-of-use/)

## 5. API Key

`backend/ent/schema/api_key.go` 的 `key` 字段为 required unique string。`backend/internal/repository/api_key_repo.go` 使用 `apikey.KeyEQ(key)` 直接查询。`backend/internal/handler/dto/types.go` 的 APIKey DTO 公开 `json:"key"`。

结论：需求中的 hash only 与一次展示当前没有实现。M6 前不能把该 Key 模型视为生产安全。

API Key 的 status、expires、IP whitelist、IP blacklist、quota 和 rate limit 已存在，可以继续复用。

## 6. Account credentials

`backend/ent/schema/account.go` 把 credentials 定义为 JSONB。初始 migration 注释称其为加密存储，但 `backend/internal/repository/account_repo.go` 创建和更新时直接调用 `SetCredentials(normalizeJSONMap(...))`，没有调用 Secret Encryptor。

现有 `backend/internal/repository/aes_encryptor.go` 实现 AES 256 GCM，但构造函数使用 TOTP Encryption Key，主要服务 TOTP 和部分已有加密组件。Account credentials 需要独立密钥域和迁移。

结论：源码注释与实际持久化行为冲突，属于高危安全发现。

## 7. User、权限与前端

Auth routes 包含注册、登录、密码重置、LinuxDo、GitHub、Google、WeChat、OIDC 和 DingTalk。`registration_enabled` 查询失败时安全默认关闭。Payment、affiliate、Model Plaza、Available Channels 等已有功能开关。

`backend_mode_enabled` 会限制为仅 Admin 登录，无法用于本项目，因为普通 User 仍需 Portal 和 Gateway。

User routes 已具备 profile、keys、groups、usage、dashboard、redeem、subscriptions 和 monitors。V1 需要前端裁剪与后端 route policy。只隐藏 Sidebar 无法阻止直接 API 请求。

用户模型级授权目前不足。现有 `user_allowed_groups` 适合 Group 访问，不能在一个 Composite Group 内对每个逻辑模型授权。

## 8. Group、Composite 与路由

Group Schema 包含 platform、subscription type、rate multiplier、model pricing JSON、model routing、models list、fallback Group 和多种限制。Account Group 关联支持 priority。

Composite Route 当前有 public model、match type、target platform、upstream model、endpoint、priority、enabled。迁移 227 已允许 target platform 为 DeepSeek。

关键缺口：

1. 无 target Group。
2. 默认仍会使用内置模型检测。
3. OpenAI OAuth 与 API Key 账号都属于 platform openai。

因此只配置 Composite target platform 无法严格满足“Codex 到 Subscription Group，OpenAI official 到 Official API Group”。扩展 `target_group_id` 和 only explicit 模式是最小的可靠方案。

## 9. Scheduler、Sticky、并发与限流

现有 Scheduler 已很成熟，重写收益低且冲突风险高。Account 中有 status、schedulable、rate limited、reset、overload 和 temporary unschedulable 等字段。UI 可以映射 Active、Cooldown、Exhausted、Disabled。

`backend/internal/service/concurrency_service.go` 与 Redis repositories 管理用户和账号并发槽位。Auth 和 Panel 有 rate limiter。OpenAI WS 也有入口 lease 和清理。

Sticky 的 request header 与客户端实际行为仍待 M3、M11 验证。Nginx 需要保持 header，并关闭 SSE buffering 与 gzip。

## 10. Billing 与余额

现有 Billing 支持多种成本分量和多层价格来源。Group `model_pricing` 优先于 Channel 与内置价。Channel pricing 在 SQL migrations 中有独立表和动态管理接口。

问题：

1. Service 与 DTO 大量使用 float64。
2. `billing_service.go` 有硬编码 fallback map。
3. 价格修改缺少统一不可变版本历史。
4. usage log 无完整单价快照。
5. `users.balance` 虽映射 PostgreSQL decimal，Go 侧仍是 float。
6. 基线 Admin balance history 借用 redeem code 与 affiliate ledger，缺少统一钱包流水；M6.1/M6.2 已新增 CNY 不可变流水并把受控 Admin 写入口切入 Wallet Service。

`usage_billing_repo.Apply` 的事务和 dedup 很有价值。它先认领 `(request_id, api_key_id)`，再更新余额和 quota。余额不足时当前实现仍允许扣成负数，符合已开始 Streaming 的结算需求。

`gateway_usage_billing.go` 在结算后 best effort 写 usage log，并有同步 fallback。仍可能出现扣款完成、usage log 最终失败的状态。V1 应把 usage、钱包流水和余额合入同一事务。

## 11. DeepSeek

已有能力：

1. `PlatformDeepseek`。
2. OpenAI Compatible Chat Completions。
3. reasoning content 普通与 Streaming 单元测试。
4. `/user/balance` 余额探测和双币种解析。
5. V4 Pro 与 Flash 默认模型列表。
6. Composite target platform 支持。

需求冲突：DeepSeek 官方已经宣布 `deepseek-chat` 和 `deepseek-reasoner` 于 2026 年 7 月 24 日完全退役。当前日期为 2026 年 9 月 5 日，旧名不能当作上游实际模型继续设计。

官方来源：

* [DeepSeek Change Log](https://api-docs.deepseek.com/updates/)
* [DeepSeek V4 Preview Release](https://api-docs.deepseek.com/news/news260424/)
* [DeepSeek Thinking Mode](https://api-docs.deepseek.com/guides/thinking_mode/)

可以保留旧名作为 Gateway 逻辑别名，实际映射和 thinking profile 需要人工确认和真实 API 测试。

## 12. 日志与隐私

`usage_logs` 主要记录元数据、Token、费用、模型、Group、Account、延迟与错误。未发现 usage schema 保存完整 Prompt 或 Answer 的字段。

Gateway 会在内存读取 request 和 upstream response body 以实现协议转换、错误处理和 usage 解析。Debug、Ops error 和内容审核路径可能处理请求内容，需要逐路径验证 logger redaction。

`audit_logs.request_body` 保存脱敏和截断后的管理请求体。其 redaction tests 较完整。V1 仍应禁用 Prompt capture，并确保 account credentials、签名和 API Key 新字段加入敏感键表。

## 13. 支付、注册和商业功能

Payment routes 始终注册用户、public verify、webhook 和 Admin endpoints，见 `backend/internal/server/routes/payment.go`。Payment config 默认 disabled，但需要验证每个 handler 在 disabled 状态都 fail closed。V1 建议添加统一 route group guard，避免漏检。

Registration、promo、invitation、affiliate、Model Plaza、Available Channels 和部分插件入口已有 DB settings。Frontend `featureFlags.ts` 可隐藏菜单。Payment flag 当前为 opt out 模式，设置未加载时默认可见，V1 应改为安全初始策略或在站点策略层强制关闭。

## 14. PostgreSQL、Redis 与迁移

SQL migrations 按文件名顺序执行，记录 filename 和 checksum。Migration 为 forward only。Ent Schema 与部分后期 SQL table 并存，Channel pricing 等功能主要由 SQL migration 管理。

Redis 用于缓存、Session、Sticky、限流、并发和调度状态。钱包、价格和订单不应依赖 Redis 作为唯一存储。

现有 Compose 正确地不发布 PostgreSQL 与 Redis 端口。App 默认发布 0.0.0.0:8080，生产应改为回环地址或仅 Docker 内网。

## 15. 测试体系

源码测试覆盖广，尤其是 OpenAI Responses、Chat Completions、Streaming、Scheduler、Billing、DeepSeek reasoning、migration 和安全 redaction。根 Makefile 的前端测试只运行关键清单，M12 应补充全量 Vitest。

当前环境证据：

* Node `v24.19.0` 与 pnpm `11.19.0` 可用。
* 宿主机 Go 命令不存在；固定的 Go 1.27 builder 镜像可用。
* Docker Engine `29.3.1` 可用，Compose 全栈健康。
* Backend unit/integration、Frontend lint/typecheck/251 个 Vitest 文件/production build，以及当前平台适用的 5 个 deploy 测试均已通过。

## 16. 许可证发现

根 `LICENSE` 是 GNU Lesser General Public License Version 3。README 与 README_CN 写明 LGPL 3.0 或更高版本。

README_CN 另有以下项目声明：服务条款风险、仅供技术学习研究、无商业授权。该声明与 LGPL 授权文本的关系需要法律专业人士确认。不能仅凭工程分析得出商业使用结论。

分发时至少需要保留版权和许可证文本，并根据 LGPL 具体使用、修改与链接方式履行对应义务。详细法律义务需要人工确认。

## 17. 需求与源码冲突清单

| 需求 | 源码事实 | 结论 |
| --- | --- | --- |
| API Key hash only | 明文 `api_keys.key` 与完整 DTO | 必须修改 |
| 上游 Secret 不泄漏 | Account credentials 明文 JSONB | 必须修改 |
| 金额避免 float | Service 大量 float64 | 必须修改 |
| 所有主要价格在 DB | 存在 Group、Channel DB 价格，也有硬编码 fallback | V1 resolver 必须隔离 fallback |
| 价格历史和快照 | 只有成本分量与倍率，缺完整版本和单价快照 | 必须新增 |
| Wallet 与 transactions | 当前只有 users balance，Admin 历史分散 | 新增统一流水，保留 users balance |
| 模型到具体 Group | Composite 只到 platform | 必须扩展 target Group |
| 不自动跨 Provider | Group 有 fallback，Composite 有内置检测 | V1 强制关闭 |
| DeepSeek 旧模型名 | 官方已退役，源码默认 V4 | 需要人工确认 alias |
| 关闭 Payment | flag 默认关闭，但 routes 持续注册 | 需要逐 handler 验证和统一 guard |
| User 模型权限 | Group 权限存在，Composite 内模型级权限不足 | 新增权限表 |
| Usage 与扣款原子 | 扣款事务化，usage log best effort | 需要合并事务 |

## 18. Technical Debt

1. Ent float field 映射 NUMERIC，类型安全不足。
2. Account credential 注释与实现不一致。
3. Ent schema 和后期裸 SQL table 混合，迁移设计需要双重检查。
4. Payment 和商业功能范围大，设置、route 与 UI 可能出现开关漂移。
5. OpenAI Gateway 热点文件体积和职责较大，直接修改易与 upstream 冲突。
6. 模型价格来源多层，V1 需要明确单一用户售价来源。
7. usage log best effort 带来账单和日志分离风险。
8. Compose 示例使用 latest，不适合可重复生产发布。

## 19. Risk Register

| ID | 风险 | 严重性 | 可能性 | 控制措施 | Gate |
| --- | --- | --- | --- | --- | --- |
| R1 | Subscription 上游规则变化或不允许共享 | Critical | High | 法律与上游授权，独立停用开关 | M3、M14 |
| R2 | OAuth Token 失效 | High | High | refresh、401 处理、同组账号切换、告警 | M3 |
| R3 | 账号限制或 Usage Limit 变化 | High | High | 配额探测、Cooldown、容量阈值 | M3、M14 |
| R4 | Codex 协议变化 | High | High | 固定客户端版本、Responses 回归 | M11 |
| R5 | Cursor 协议变化 | High | Medium | 真实客户端矩阵、脱敏抓取 endpoint | M11 |
| R6 | Sub2API upstream 重大变化 | High | Medium | 小改动、独立模块、sync 分支、全回归 | 每次升级 |
| R7 | float 或价格错误导致计费偏差 | Critical | High | Decimal、版本、快照、对账 | M6、M7 |
| R8 | 余额并发或重复扣费 | Critical | Medium | PostgreSQL 事务、unique、并发测试 | M6、M12 |
| R9 | 数据库故障后结算丢失 | Critical | Medium | 合并事务、持久化恢复日志与 fsync、重启保持 degraded、幂等重放、人工对账 | M12 |
| R10 | Secret 泄漏 | Critical | High | hash、encryption、redaction、rotation | M6、M12 |
| R11 | Provider API 价格变化 | High | High | Admin price version、生效时间、告警 | M7、运营 |
| R12 | DeepSeek 接口和模型变化 | High | High | V4 实测、显式 mapping、版本记录 | M5 |
| R13 | OpenAI API 变化 | High | Medium | 官方 SDK、Responses 与 Chat 回归 | M4、M11 |
| R14 | 服务器故障 | High | Medium | restart、health、备份、容量告警 | M13 |
| R15 | 备份无法恢复或 RPO 过大 | Critical | Medium | 基础备份、连续 WAL/PITR、checksum、加密、月度定点恢复演练 | M13 |
| R16 | target platform 误选成本池 | Critical | High | M6 完成 target Group 锁定、only explicit、唯一约束和无 fallback，M3 到 M5 分别验收 | M6、M3、M4、M5 |
| R17 | Payment route 关闭不完整 | High | Low | M2 已统一后端 guard 并通过直接 API/全量回归；后续升级持续回归 | M2、每次升级 |
| R18 | Redis 故障绕过防重放或限流 | High | Medium | 关键路径 fail closed、重启测试 | M8、M12 |
| R19 | Admin 账号接管 | Critical | Medium | TOTP、step up、IP 限制、审计 | M2、M12 |
| R20 | 许可证或 README 声明阻止商业计划 | Critical | Medium | 法律审核、保留声明、发布 gate | M14 |

## 20. 待验证事项

1. 当前 Codex CLI 的完整 Base URL 配置、header 和协议。
2. 当前 Cursor 的 endpoint、重试和模型探测。
3. OpenAI OAuth 账号在目标授权下的合法使用方式。
4. DeepSeek V4 thinking 和旧 alias 的最佳兼容方法。
5. Password hashing 具体算法与参数。
6. Account credential 加密后的 Scheduler 性能。
7. usage log 与 billing transaction 合并后的热点锁性能。
8. Nginx reload、SSE cancel 和 WebSocket 的实际表现。

## 21. 建议下一步

1. 进入 M6 前固化 CNY 权威币种与存量数据迁移决策。
2. M6 优先完成钱包不可变流水、Decimal 热路径、API Key 摘要和 Account credential 加密。
3. 同一 Milestone 完成用户模型权限、显式目标 Group 路由与唯一约束，禁止跨成本池 fallback。
4. 完成人工确认清单，尤其是 Subscription 合规与 DeepSeek 模型名。

## 22. M6.2 实施确认

1. 管理员充值和调账现已使用十进制字符串、钱包事务、管理员 operator、请求幂等键和非空备注；负调账不能使余额低于零。
2. usage 退款只允许一次性全额退款。HTTP 幂等负责请求重放，`reverses_transaction_id` 的部分唯一索引和数据库 trigger 独立校验同用户、负数 CNY usage 与完整反向金额。
3. 钱包写路由由 step-up 中间件保护；启用时 Admin API Key 被拒绝，必须使用近期完成 TOTP 验证的管理员会话。管理员和普通用户余额查询均返回八位定点字符串，普通用户 ID 只取自认证 subject。
4. 通用 Admin User 创建被限制为零初始余额，通用更新不能携带 balance，旧 float 型 `AdminService.UpdateUserBalance` 已移除，避免内部或后台 CRUD 绕过流水。公开注册目前由 M2 关闭；未来若恢复且配置非零默认余额，必须先补齐同事务开账。
5. 真实 PostgreSQL 18 已验证 migration 幂等、schema、trigger、不同请求键的并发退款唯一性和对账。本地升级库 migration checksum 为 64 位，2 个现有测试用户仍只有 2 条 opening 流水，对账差异为零。
6. Gateway usage 仍沿用既有结算路径，尚未生成 `wallet_transactions` usage 行；不能把 M6.2 解释为完整收费闭环。下一顺序项为用户模型权限，usage 原子结算和定时对账仍需在 M6 后续子任务完成。
