# Codex 项目执行规则

## 1. 项目目标

本仓库基于 Sub2API 构建一个供课题组和熟人使用的小规模 AI API Gateway。V1 预计服务 10 到 50 人，提供独立用户、独立 API Key、人民币预充值余额、用量后扣费、调用记录，以及三个相互独立的上游账号池：

1. Codex Subscription Group
2. OpenAI Official API Group
3. DeepSeek API Group

当前规划基线是 Sub2API 官方主仓库提交 `b1748c4ea99ce2120401a269142aa071e18a84da`，提交时间为 2026 年 9 月 3 日。开始开发前必须重新确认 upstream 是否变化，并把差异记录到 `findings.md`。

## 2. 每次工作前必须执行

1. 完整阅读本文件。
2. 阅读 `task_plan.md`，确认唯一的当前 Milestone。
3. 阅读 `progress.md` 最近三条记录。
4. 阅读 `findings.md` 中未关闭的风险和待确认项。
5. 执行 `git status --short`，保护用户已有修改。
6. 确认当前分支，禁止直接在 `main` 上开发功能。
7. 写出本次范围、预计修改文件、验收命令。

## 3. 范围纪律

1. 一次只完成一个 Milestone 中的一个可验收任务。
2. 禁止擅自扩展 Provider、支付、商城、代理商、返利和公开注册需求。
3. 禁止一次性重构整个项目。
4. 优先顺序为配置复用、现有模块复用、前端裁剪、独立模块、小范围核心改动。
5. 已有 Gateway、OAuth、Scheduler、Streaming 逻辑应保持上游兼容。
6. 所有无法从代码或测试确认的结论必须标记“待验证”“需要测试”或“需要人工确认”。
7. 任何超出当前 Milestone 的必要工作，先写入 `task_plan.md` 的后续项，不在当前提交顺带实现。

## 4. 需要事先说明的改动

以下改动开始前，Codex 必须在工作记录中说明原因、影响面、回滚方式和测试范围：

1. 修改 `backend/internal/handler/gateway_handler.go` 或 Gateway 路由。
2. 修改 `backend/internal/service/openai_account_scheduler.go`、Sticky Session、并发槽位或重试逻辑。
3. 修改 OpenAI OAuth Token 刷新和账号凭据结构。
4. 修改 `backend/ent/schema` 或 `backend/migrations`。
5. 修改余额扣款、退款、调账、价格解析或 usage 结算。
6. 修改 Nginx、Caddy、Compose 网络和转发请求头。
7. 修改 API Key 认证缓存或密钥迁移路径。

## 5. 数据库规则

1. 迁移文件只增不改。已发布迁移不得重写、重命名或删除。
2. 新迁移沿用 `backend/migrations` 的编号和校验机制，先确认 upstream 最新编号，避免冲突。
3. 每个迁移必须可重复检查结果，并包含 schema 集成测试。
4. 金额列使用 PostgreSQL `NUMERIC`。Go 业务计算使用 `shopspring/decimal`，禁止在新增账务代码中使用 `float32` 或 `float64`。
5. 金额 API 使用十进制字符串，禁止依赖 JavaScript Number 保存账务精度。
6. 余额更新、交易流水、usage 结算和幂等记录必须位于同一数据库事务。
7. 唯一约束是幂等的最终防线，应用层预检查不能替代数据库约束。
8. 数据迁移必须给出迁移前检查、迁移后对账和回滚方案。
9. 禁止把 Redis 作为余额、价格历史、充值订单或账单的唯一数据源。
10. `usage_billing_dedup` 必须保存 fingerprint 与原结算结果引用；价格有效区间和 V1 exact route 唯一性必须由数据库约束保证。
11. 已被用户权限引用的 Route 不得原地修改模型、Provider、target Group、upstream model 或 endpoint；必须新建 Route 并重新授权。

## 6. Gateway 与 Provider 规则

1. V1 只开放三个目标 Group。
2. 所有客户模型路由必须为后台可审计的显式路由。
3. V1 Composite Group 必须开启 `explicit_routes_only`。
4. 路由必须锁定 `target_group_id`，仅按 `target_platform` 选池不满足隔离要求。
5. Codex 模型只能进入 Codex Subscription Group。
6. OpenAI 官方模型只能进入 OpenAI Official API Group。
7. DeepSeek 模型只能进入 DeepSeek API Group。
8. 三个目标 Group 的 `fallback_group_id` 和 `fallback_group_id_on_invalid_request` 必须为空。
9. 任一 Provider 停用时，其余 Provider 必须继续工作。
10. 未匹配、停用、无价格或无权限的模型必须 fail closed，禁止猜测 Provider。
11. 修改 Provider 后必须验证非流式、Streaming、usage、错误映射和健康恢复。
12. 修改 Codex 路由后必须验证 Responses API、Tool Calls、Reasoning、长请求和 Sticky Session。

## 7. 计费与钱包规则

1. 用户钱包的记账币种固定为 CNY，除非产品负责人书面变更该决策。
2. 价格来源必须是数据库中已生效的不可变价格版本。
3. 缺少有效价格时拒绝请求，禁止使用硬编码兜底价收费。
4. 每条 usage 保存 `pricing_rule_id`、`pricing_version_id` 和完整价格快照。
5. 每个已结算 usage 只能产生一条 `usage` 类型钱包流水。
6. 充值、退款、调账和 usage 必须记录 `balance_before` 与 `balance_after`。
7. 相同外部订单重复请求只能返回原结果，禁止重复入账。
8. Streaming 开始后允许完成。最终费用可使余额轻微为负，下一次新请求必须被拒绝。
9. 修改计费逻辑必须增加边界值、舍入、价格版本和幂等测试。
10. 修改钱包逻辑必须增加并发测试和账本对账测试。
11. PostgreSQL 持续失败时，最终结算必须先写入持久化、带 checksum 且已 fsync 的恢复日志；重启后保持 Billing degraded，幂等重放和对账完成前不得恢复新请求。

## 8. 安全规则

1. 禁止把真实 API Key、OAuth Token、Cookie、密码、HMAC Secret 或生产域名凭据写入 Git、文档、测试快照和日志。
2. `.env.example` 只保留变量名与无效占位符。
3. 用户 API Key 只在创建成功响应中展示一次。数据库仅保存 HMAC 摘要、前缀、末四位和版本。
4. 上游账号凭据必须使用独立的 AES 256 GCM 密钥加密，禁止复用 JWT 或 TOTP 密钥。
5. 普通用户接口和 DTO 禁止返回上游账号、上游密钥、OAuth Token、内部成本和系统 Secret。
6. 日志默认只存元数据，Prompt 与模型回答默认关闭。
7. 审计记录必须脱敏 Authorization、Cookie、签名和凭据字段。
8. Internal Recharge API 必须同时具备 HMAC、时间窗、Nonce、防重放、IP Allowlist、限流、幂等和审计。
9. Redis 和 PostgreSQL 禁止发布到公网端口。
10. 生产流量必须经过 HTTPS 反向代理。
11. 管理员敏感操作应启用 TOTP 与 step up 验证。
12. 发现 Secret 泄漏时立即停止后续工作，记录影响范围并启动轮换。
13. Internal Recharge 统一使用 `X-Recharge-Key-Id`、`X-Recharge-Timestamp`、`X-Recharge-Nonce` 和 `X-Recharge-Signature`；source 只能从 Key ID 的服务端配置推导，验签通过后才认领 nonce。
14. API Key pepper 只能保留 active 与 previous 有界双读；previous 命中时渐进重算，截止时撤销未迁移 Key。

## 9. 前端规则

1. 用户端只展示余额、消费、自己的 Key、自己的日志、可用模型、Base URL、Codex 和 Cursor 指南。
2. 页面隐藏必须同时有服务端授权或功能开关，前端隐藏不构成安全边界。
3. V1 关闭公开注册、支付、套餐、兑换码、优惠码、返利、第三方登录、Model Plaza、公开渠道和插件入口。
4. 不删除上游页面文件，优先使用功能开关、路由守卫和导航过滤。
5. 金额输入输出使用十进制字符串，展示层统一标注人民币。

## 10. 测试与验收

1. 每次修改执行直接相关单元测试。
2. Milestone 完成前执行后端全量测试、前端 lint、typecheck、关键 Vitest 和构建。
3. 数据库改动执行迁移集成测试与真实 PostgreSQL 测试。
4. Redis 相关改动验证重启、短暂不可用、TTL 和并发竞争。
5. Streaming 测试必须验证首包、逐块转发、结束标记、客户端断开和结算。
6. Nginx 改动必须验证 `proxy_buffering off`、SSE 不压缩、长超时、WebSocket 和 Session 请求头。
7. 客户端里程碑必须使用真实 Codex CLI、Cursor 和 OpenAI SDK 验收。
8. 测试失败时不得标记任务完成。
9. 禁止删除、跳过或弱化测试来制造通过结果。
10. 外部 Provider 测试只使用专用测试账号和最小额度，禁止在日志中输出凭据。
11. M3 到 M5 的正式验收必须等待 M6、M7；此前只允许不收费的 mock 或专用低额度技术探测。
12. M13 必须验证基础备份、连续 WAL/PITR 和持久化结算恢复日志，默认目标 RPO 不高于 5 分钟。

## 11. Git 与 upstream 同步

1. 保持 `upstream` 指向 `https://github.com/Wei-Shaw/sub2api.git`，`origin` 指向项目 Fork。
2. `main` 只跟踪可发布状态，`develop` 用于集成，功能分支使用 `feature/m数字_简述`。
3. 每个 Milestone 使用独立分支和小提交。
4. 禁止全项目格式化、无关重命名和依赖批量升级。
5. 自定义逻辑集中在清晰命名的文件和迁移中，减少修改上游热点文件。
6. 同步 upstream 前先创建数据库备份和 Git tag，在专用 `sync/upstream_日期` 分支合并。
7. 合并后重新执行协议、计费、路由、安全和客户端回归。
8. 每次发布更新 `CHANGELOG-custom.md`，列出相对 upstream 的修改与兼容影响。

## 12. 工作结束要求

1. 更新 `progress.md`，记录日期、Milestone、任务、文件、命令、测试、发现、风险和下一步。
2. 重要新事实或冲突写入 `findings.md`。
3. 数据模型或接口变化同步更新对应设计文档。
4. 执行 `git diff --check` 和 `git status --short`。
5. 给出本次完成项、未完成项、测试证据和回滚点。
6. 只有满足 `task_plan.md` 的完成条件后才能更新 Milestone 状态。
