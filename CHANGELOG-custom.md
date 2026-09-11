# 中转站定制变更日志

本文件只记录相对 Sub2API 上游的中转站定制。上游自身的完整历史以官方仓库为准。

## 版本格式

* `X.Y.Z-planning`：文档和决策版本，不含可运行功能。
* `X.Y.Z-rc.N`：候选发布，必须绑定测试报告和固定 image digest。
* `X.Y.Z`：已批准的小范围生产版本。

每个条目必须写明上游基线、代码变更、数据库变更、兼容性、安全影响、测试结果和恢复要求。

## [Unreleased]

### Planned

* M6：实现 CNY 钱包流水、Decimal 热路径、用户模型权限、API Key 摘要、Account credentials 加密和目标 Group 路由基础。
* M7：实现动态价格规则、不可变版本和 usage 价格快照。
* M8：实现安全的 Internal Recharge API。
* M3、M4、M5：按合规和技术门禁接入三个 Provider。
* M9、M10：精简 Admin 与 User Portal。
* M11 到 M14：完成客户端、安全、并发、部署和试运行验收。

### Human decisions required

* Codex Subscription 授权范围和收费边界。
* LGPL 与 README_CN 商业声明的处理方式。
* DeepSeek 公开逻辑模型名及实际 V4 映射。
* V1 模型清单和初始 CNY 价格。
* Admin 网络边界、RPO、RTO、备份目标和 Secret Manager。

### M2 completed

* 新增统一的后端功能关闭守卫。关闭、缺失或读取失败时，注册、优惠/邀请、支付与 Webhook、兑换/订阅、返利、可用渠道、公开监控和插件接口稳定返回 HTTP 404 与 `FEATURE_DISABLED`。
* 前端范围开关改为明确 opt-in，并为菜单、直接路由、公开支付回调页和订阅轮询增加相同口径的关闭控制。
* 新增 forward-only migration `239_v1_scope_defaults.sql`，将 19 个 V1 范围外开关设为 `false`；保留源码和 Admin 配置入口以支持受控恢复。
* 新增 `deploy/docker-compose.m2.yml`，本地镜像 `sub2api:m2-scope-closure` digest 为 `sha256:9c2092111a6a816b276210ae682b99335d415dce25024bf068e6d4fdd0282d27`。
* Backend `go test ./...` 全部通过；Frontend lint、typecheck、251 个测试文件/1838 个测试和 production build 通过。
* 本地升级库已记录 migration，19 个开关全部为 `false`；运行栈健康，注册/支付/Webhook 等黑盒请求不能绕过关闭状态。

### M2 recovery

应用代码可回退到 M0 固定镜像；migration 不回滚、不删除。若需恢复单项能力，由管理员在确认风险后显式开启对应开关，并重新执行该能力的安全与 API 验收。

### M0 completed

* 在 `codex/m0-baseline` 固定上游提交 `b1748c4ea99ce2120401a269142aa071e18a84da`。
* 新增 `deploy/docker-compose.m0.yml`，构建本地固定 tag `sub2api:m0-b1748c4`，镜像 digest 为 `sha256:74a563871167fbb5c4106b63d7668d247c6fda1470adcec629e6033cfbcb3725`。
* Windows Docker Desktop 上 PostgreSQL、Redis、Backend 和内嵌 Frontend 健康；只发布回环地址 `127.0.0.1:8080`。
* Backend unit/integration、Frontend lint/typecheck/251 个 Vitest 文件/production build，以及当前平台适用的 5 个 deploy 测试通过。
* 增加项目规划文档跟踪规则和 POSIX 部署文件 LF 属性；没有业务代码、schema 或 migration 变更。
* 创建正式 Fork `https://github.com/lanora-tree/sub2api`，配置为本地 `origin`，保留官方仓库为 `upstream`，并发布 `codex/m0-baseline` 分支。
* 管理员本人已在 UI 确认 `v2026.06.10` 合规声明；数据库记录已核验，自动化没有代签或绕过门禁。
* 创建仅含本地无价值数据的测试 Group、User、API Key 与 OpenAI Compatible mock Account；测试对象不记录真实 Provider 凭据。
* `/v1/models` 返回 `gpt-5.4`；非流式与 Streaming 请求均为 HTTP 200，首个 Streaming 数据帧约 `108.7 ms`，并收到 `[DONE]`。
* 两条请求均写入 `usage_logs`，每条 7 tokens、成本 `0.0000550000`；测试用户余额从 `100` 变为 `99.99989000`。
* 模拟账号在验收后设为 inactive，临时 mock 容器与脚本已移除；测试 Group、User 和 Key 保留供本地审计。
* `upstream/main` 当前为 `4726bdd08b6201d426a80529b79be123a4008d20`，领先固定基线 349 个提交；未在 M0 静默同步。

### M0 recovery

停止并移除本地 Compose 容器时必须保留数据卷；回退 tracked 变更只需删除 M0 override 和规划文档变更。不得在未备份时删除 `deploy/postgres_data`、`deploy/redis_data` 或 `deploy/data`。

## [0.1.1-planning] - 2026-09-06

### Changed

* 统一 Internal Recharge 请求头为 `X-Recharge-Key-Id`、`X-Recharge-Timestamp`、`X-Recharge-Nonce` 和 `X-Recharge-Signature`。
* `source` 改为由服务端按 Key ID 推导，并定义稳定的业务 fingerprint；HMAC 验签通过后才认领 nonce。
* M3、M5 的正式验收增加 M6、M7 依赖，主执行路径明确为 `M2 -> M6 -> M7 -> M3/M4/M5`。
* `BILLING_DEGRADED` 改为 HTTP 503，并要求返回 `Retry-After`。
* PostgreSQL 持续失败时，最终结算 envelope 必须先写入持久化恢复日志并 fsync；重启保持 degraded，重放和对账后由 Admin step up 解除。
* `/v1/models` 不再因瞬时账号 Cooldown 改变，只反映授权、启用配置和有效价格。
* API Key pepper 轮换改为 active、previous 有界双读，认证时渐进重算，逾期旧 Key 强制撤销。
* 生产恢复从单独每日 `pg_dump` 提升为物理基础备份、连续 WAL 归档、PITR 与每日逻辑备份，默认 RPO 不高于 5 分钟。

### Database design

* `usage_billing_dedup` 增加 fingerprint、usage、流水、金额、币种和结算时间引用。
* 价格版本增加 PostgreSQL 时间区间排他约束。
* V1 启用的 exact route 增加部分唯一索引。
* 已被权限引用的 Route 授权语义字段改为不可原地修改。
* Account credentials 使用插入前生成的独立 AAD UUID，避免新账号 ID 尚未生成时出现明文窗口。

### Verification

本版本只修订规划文档，没有修改 Sub2API 业务代码或数据库。已执行压缩包完整性、Markdown 文件清单、关键术语、Internal Recharge headers、里程碑依赖和测试数量的一致性检查；运行时验证仍由 M0 及后续 Milestone 完成。

### Recovery

该版本仅为文档修订。保留 `0.1.0-planning` 原包即可回退；不得把回退文档解释为回退任何生产数据或实现。

## [0.1.0-planning] - 2026-09-05

### Upstream baseline

| 项目 | 值 |
| --- | --- |
| Repository | `https://github.com/Wei-Shaw/sub2api.git` |
| Branch | `main` |
| Commit | `b1748c4ea99ce2120401a269142aa071e18a84da` |
| Commit time | `2026-09-03T15:40:51+08:00` |
| Latest tag observed | `v0.2.0` |

### Added

新增 14 份规划与交接文档：

* `AGENTS.md`
* `task_plan.md`
* `architecture.md`
* `database_design.md`
* `api_spec.md`
* `billing_design.md`
* `provider_design.md`
* `security.md`
* `deployment.md`
* `testing.md`
* `findings.md`
* `progress.md`
* `README-runbook.md`
* `CHANGELOG-custom.md`

### Change reason

本版本用于把产品范围、源码事实、技术决策、风险、测试门禁和运维步骤固化为可交给 Codex 逐 Milestone 执行的基线。单独维护定制变更日志，可以在同步 upstream 时区分官方变更与本项目改动，并明确数据库和兼容性影响。

### Architecture decisions

* 采用单体 Sub2API Backend、Vue Portal、PostgreSQL、Redis 和 Nginx 的 V1 拓扑。
* 保留上游 Gateway 与 Scheduler，减少协议层和并发调度重写。
* 创建一个用户入口 Composite Group 和三个独立 Provider 目标 Group。
* Composite Route 规划增加 `target_group_id` 和 exact only 模式。
* 禁止跨成本池 Group fallback。
* 使用 CNY 作为唯一权威记账币种。
* 保留 `users.balance` 为权威余额，新增不可变统一钱包流水。
* 为模型价格新增规则表和不可变版本表，每条 usage 保存快照。
* 已开始的 Streaming 请求完成后按实际 usage 结算，可能形成短暂负余额。

### Security decisions

* User API Key 规划改为 HMAC 摘要，明文只展示一次。
* Provider credentials 规划使用独立密钥域的 authenticated encryption。
* Internal Recharge 使用 HMAC SHA256、Key ID、时间窗、Nonce、Redis 防重放、IP Allowlist、限流和 PostgreSQL 幂等约束。
* V1 日志只保留 metadata，禁止保存 Prompt 和完整 Answer。
* 生产只公开 80、443，管理入口限制到 VPN 或批准 IP，数据库和 Redis 仅在内部网络。

### Database changes

本版本未执行数据库变更。已规划以下后续 migration：

* `wallet_transactions`
* `model_pricing_rules`
* `model_pricing_versions`
* `external_recharge_orders`
* `user_model_permissions`
* `api_keys` 摘要、前缀、后四位和指纹字段迁移
* `accounts.credentials` 独立加密 envelope 迁移
* `usage_logs` 价格版本、单位价格、费用和业务事件快照
* `groups.composite_explicit_routes_only`
* `composite_model_routes.target_group_id`

Migration 编号需在正式 Fork 创建分支时，基于当时上游最新 migration 冲突检查后分配。

### Compatibility findings

* 现有 Gateway routes 已覆盖 Codex、Responses、Chat Completions、Embeddings 和相关兼容路径。
* Codex CLI、Cursor 和 SDK 的真实兼容性尚未执行动态验收。
* DeepSeek 已停用需求中出现的两个旧上游模型名。旧名只可作为待确认的 Gateway 逻辑别名。
* Codex Subscription 和 OpenAI Official API 在现有实现中都属于 OpenAI platform，需要目标 Group 级路由隔离。

### Risks recorded

* Codex Subscription 多用户共享和收费模式存在上游条款风险。
* User API Key 当前为明文存储和查询。
* Account credentials 当前直接持久化 JSONB，未见实际加密调用。
* float64、价格 fallback 和缺少统一钱包流水影响精度和追溯。
* 扣费成功后 usage log 仍可能 best effort 写入失败。
* 前端隐藏功能无法替代服务端 route guard。

### Source changes

* Sub2API 业务代码：无。
* Ent schema：无。
* SQL migration：无。
* Frontend：无。
* Compose 或生产环境：无。

### Verification

已完成：

* 静态源码审计与源码路径交叉核对。
* 上游 commit、tag 和 working tree 状态确认。
* 交付文件、标题、关键术语、敏感值模式和内部一致性检查。

未完成：

* Go tests，当前运行器未安装 Go。
* Frontend tests 与 build，依赖下载受限。
* Compose startup，当前运行器未安装 Docker。
* Provider、客户端、migration、负载、故障注入和恢复测试。

### Recovery

本版本只有新增文档，不涉及运行系统或数据。删除该文档包即可撤回规划稿。上游源码目录保持 clean。

## 后续版本条目模板

```markdown
## [X.Y.Z] - YYYY-MM-DD

### Upstream baseline

### Change reason

### Files involved

### Added

### Changed

### Removed or disabled

### Security

### Database changes

### API compatibility

### Client compatibility

### Configuration changes

### Tests

### Known issues

### Upgrade

### Recovery
```
