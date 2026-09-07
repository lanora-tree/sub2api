# 中转站运行与维护手册

## 1. 文档定位

本手册是中转站的目标运行手册，同时提供 M0 上游基线启动入口。M0 本地基线正在验收，定制业务代码和数据库变更尚未实施。标记为“定制版”的操作必须在对应 Milestone 完成并通过测试后使用。

当前规划版本为 `0.1.1-planning`（2026-09-06）。2026-09-07 已完成固定提交的镜像、全栈健康和大部分 M0 测试，剩余人工门禁见 `progress.md`。

中转站面向约 10 到 50 名课题组成员和熟人用户，在 Sub2API 上提供统一的 AI API Gateway。V1 为每位用户提供独立 API Key、模型权限、CNY 预付余额、实际用量账单和调用元数据，后端接入 Codex Subscription、OpenAI Official API 与 DeepSeek Official API 三个相互隔离的 Provider。

设计与交接入口：

| 文档 | 用途 |
| --- | --- |
| [findings.md](./findings.md) | Readiness、源码审计结论和风险 |
| [task_plan.md](./task_plan.md) | 里程碑、依赖和验收条件 |
| [architecture.md](./architecture.md) | 组件边界、数据流和故障策略 |
| [database_design.md](./database_design.md) | 表、约束、事务和迁移顺序 |
| [api_spec.md](./api_spec.md) | Gateway、Portal、Admin 和 Internal API 契约 |
| [billing_design.md](./billing_design.md) | CNY、价格版本、结算和对账 |
| [provider_design.md](./provider_design.md) | 三个 Provider、Group、路由和调度 |
| [security.md](./security.md) | Secret、HMAC、权限、日志和网络安全 |
| [deployment.md](./deployment.md) | 开发与生产拓扑、Nginx、备份和恢复 |
| [testing.md](./testing.md) | 自动化、客户端、异常与验收矩阵 |
| [progress.md](./progress.md) | 实际进展、测试结果和阻塞 |
| [CHANGELOG-custom.md](./CHANGELOG-custom.md) | 定制版本变更历史 |

## 2. 基线与支持范围

* 审计基线：Sub2API `b1748c4ea99ce2120401a269142aa071e18a84da`
* 上游最新 tag：`v0.2.0`，只作为审计时参考
* Backend：Go 1.27
* Frontend：Node.js、pnpm、Vue 3、TypeScript、Vite
* Runtime：PostgreSQL 18、Redis 8、Sub2API
* V1 客户端：Codex CLI、Cursor、OpenAI Compatible SDK
* V1 Provider：Codex Subscription、OpenAI Official API、DeepSeek Official API
* 记账币种：CNY

生产环境必须固定源码 commit、构建产物 checksum 和容器 image digest。禁止直接依赖 `latest`。

## 3. M0 开发机启动

### 3.1 前置条件

1. Docker Desktop 或 Linux Docker 20.10 以上。
2. Docker Compose v2。
3. Go 1.27。
4. Node.js 和与 lockfile 兼容的 pnpm。
5. 可访问 Go、npm 和容器镜像依赖源。
6. 仅使用测试 Secret、测试账号和无生产价值的数据。

### 3.2 获取并固定源码

```bash
git clone https://github.com/Wei-Shaw/sub2api.git
cd sub2api
git switch main
git checkout b1748c4ea99ce2120401a269142aa071e18a84da
git rev-parse HEAD
git status --short
```

若团队已经创建 Fork，应把 Fork 设为 `origin`，官方仓库设为 `upstream`。所有功能开发从短生命周期分支开始。

### 3.3 配置 Compose

```bash
cd deploy
cp .env.example .env
chmod 600 .env
mkdir -p data postgres_data redis_data
```

在 `.env` 中填写占位项对应的随机测试值：

```dotenv
POSTGRES_PASSWORD=<random-postgres-password>
JWT_SECRET=<random-32-byte-or-longer-secret>
TOTP_ENCRYPTION_KEY=<random-32-byte-or-longer-key>
ADMIN_EMAIL=<test-admin-email>
ADMIN_PASSWORD=<random-admin-password>
SERVER_PORT=8080
TZ=Asia/Shanghai
```

不要把 `.env`、数据库转储、OAuth token 或 API Key 提交到 Git。

### 3.4 启动与健康检查

```bash
docker compose --env-file .env -f docker-compose.local.yml -f docker-compose.m0.yml config
docker compose --env-file .env -f docker-compose.local.yml -f docker-compose.m0.yml up -d
docker compose --env-file .env -f docker-compose.local.yml -f docker-compose.m0.yml ps
curl -fsS http://127.0.0.1:8080/health
```

依赖检查：

```bash
docker compose --env-file .env -f docker-compose.local.yml -f docker-compose.m0.yml exec postgres pg_isready
docker compose --env-file .env -f docker-compose.local.yml -f docker-compose.m0.yml exec redis redis-cli ping
docker compose --env-file .env -f docker-compose.local.yml -f docker-compose.m0.yml logs --tail=200 sub2api
```

首次启动会执行 forward only migration。任何 migration 失败都应停止后续操作，保存日志并从干净测试库复现。

首次进入 Admin 后，上游会要求当前管理员打开并阅读 `docs/legal/admin-compliance.zh.md` 或英文版本，再输入页面显示的完整确认短语。该确认会保存版本、管理员、时间、IP 和 User-Agent，必须由实际部署或运营负责人本人完成，禁止自动化代签或直接写数据库绕过。

### 3.5 基线测试

在仓库根目录执行：

```bash
make test-backend
make test-frontend
make build
```

把工具版本、测试耗时、通过数、失败数和失败日志位置写入 `progress.md`。不要在已有失败未归因时开始 M2。

## 4. 日常启动与停止

在 `deploy` 目录执行：

```bash
docker compose -f docker-compose.local.yml up -d
docker compose -f docker-compose.local.yml ps
docker compose -f docker-compose.local.yml logs -f sub2api
```

安全停止应用栈：

```bash
docker compose -f docker-compose.local.yml down
```

不要在生产运行 `down -v`，也不要直接删除 `data`、`postgres_data` 或 `redis_data`。

### 4.1 数据库迁移与版本核对

Sub2API 在应用启动时按文件名顺序执行 `backend/migrations/*.sql`，并在 `schema_migrations` 保存文件名和 checksum。定制 migration 必须遵守：

1. 先备份并记录当前 migration 版本。
2. 在正式 Fork 基于当时最大编号分配新编号，避免与新 upstream migration 冲突。
3. 只新增 forward migration，禁止修改已执行文件的内容或 checksum。
4. 在空库和上一生产版本的副本上分别执行 migration。
5. 对金额、Key 和 credentials 迁移执行行数、空值、唯一性和抽样校验。
6. 迁移失败时停止应用写入。涉及 schema 变化的恢复使用升级前数据库备份或经审核的补偿 migration。

只读检查示例：

```bash
docker compose -f compose.yaml exec -T postgres \
  psql -U sub2api -d sub2api \
  -c 'SELECT filename, applied_at FROM schema_migrations ORDER BY filename;'
```

具体表顺序和数据回填方案见 `database_design.md`。生产执行前应把实际命令、耗时、校验结果和恢复决策点加入变更单。

## 5. 生产发布检查

发布前逐项确认：

* M12 已完成，Release Candidate 的测试报告已批准。
* Codex Subscription 使用方式和商业边界已有书面合规结论。
* 许可证处理、隐私说明、服务条款和计费规则已批准。
* 镜像 tag 与 digest 已固定，SBOM 和漏洞扫描结果可追溯。
* PostgreSQL、Redis 与应用数据备份均已完成，恢复演练在近期通过。
* JWT、TOTP、Account credentials、API Key pepper 和 Internal Recharge HMAC 使用不同 Secret。
* Secret 存在受控 Secret Manager 或 root only 文件中，没有写入镜像和日志。
* Nginx 只开放 80、443，SSH 仅允许管理网或固定 IP。
* PostgreSQL 与 Redis 没有公网端口。
* Admin 路径只允许 VPN 或批准的固定 IP，并启用 MFA。
* 注册、支付、返利、兑换码、第三方登录和无关入口保持关闭。
* 三个 Provider 的 Group、显式 route、价格版本、用户权限和 fallback 状态经过双人复核。
* 监控、告警、日志保留、RPO 与 RTO 已配置。

## 6. 定制版初始配置顺序

以下步骤适用于 M2 到 M10 已实施的版本。

### 6.1 创建用户

1. Admin 登录后台。
2. 创建 User，设置初始状态、并发限制和备注。
3. 不通过公开注册创建账号。
4. 为用户分配允许的逻辑模型。
5. 充值前核对用户 ID 与金额。

### 6.2 创建 Provider 账号池

按顺序创建三个具体 Group：

1. `Codex Subscription Pool`
2. `OpenAI Official Pool`
3. `DeepSeek Official Pool`

每个 Group：

* `fallback_group_id` 保持空值。
* 只添加同一成本池和凭据类型的 Account。
* 设置明确 priority、并发和 cooldown 策略。
* Secret 在服务端加密，列表接口只显示掩码和指纹。
* 保存后执行独立健康检查和一条无敏感内容的测试请求。

### 6.3 创建 Gateway Group 与显式路由

创建一个用户可访问的 `V1 Gateway` Composite Group：

* `composite_explicit_routes_only=true`
* 所有规则使用 exact match
* 每条规则同时指定 `target_group_id`、上游模型和 endpoint
* 不配置 Group fallback

建议路由结构见 `provider_design.md`。Codex Subscription 与 OpenAI Official 同属 OpenAI platform 时，必须依靠目标 Group 约束隔离。

### 6.4 配置模型权限

1. 先建立已批准的逻辑模型清单。
2. 再为 User 写入 `user_model_permissions`。
3. 权限检查发生在账号调度和上游调用之前。
4. 未配置模型返回稳定的 `model_not_allowed`，不得落入自动检测。

### 6.5 配置价格

1. 为 Provider、目标 Group、逻辑模型、协议和生效时间创建价格规则。
2. 保存后生成不可变 `model_pricing_versions`。
3. 使用 Decimal 字符串录入 CNY 单价。
4. 先在预览页面校验一组 Token 样例，再启用版本。
5. 检查所有开放模型恰好命中一个有效价格。
6. 无价格请求保持 fail closed。

### 6.6 创建用户 API Key

1. User 或 Admin 发起创建。
2. 明文 Key 只在成功响应中出现一次。
3. 要求操作者立即保存到客户端的安全凭据存储。
4. 后续列表只显示前缀、后四位、指纹、创建时间和状态。
5. 怀疑泄漏时禁用旧 Key，创建新 Key，验证流量迁移后删除旧 Key。

## 7. 充值操作

### 7.1 Admin 手工充值

1. 核对 User ID、显示名和当前余额。
2. 输入 CNY 金额、业务原因和外部凭证号。
3. 二次确认后提交。
4. 核对 `wallet_transactions`、余额和 audit log 的业务 ID 一致。
5. 退款和调账使用新的反向或调整流水，不更新历史流水金额。

金额通过字符串传输，例如 `"100.00"`。禁止使用 JSON number 或浮点运算。

### 7.2 Internal Recharge API

目标 endpoint：

```text
POST /api/v1/internal/wallet/recharges
```

调用方必须发送 Key ID、Unix timestamp、随机 nonce、请求签名和 JSON body。签名原文：

```text
METHOD + "\n" + PATH + "\n" + TIMESTAMP + "\n" + NONCE + "\n" + SHA256(RAW_BODY)
```

值示例只使用占位符：

```http
POST /api/v1/internal/wallet/recharges HTTP/1.1
Host: <gateway-internal-host>
Content-Type: application/json
X-Recharge-Key-Id: <key-id>
X-Recharge-Timestamp: <unix-seconds>
X-Recharge-Nonce: <128-bit-random-value>
X-Recharge-Signature: <hex-hmac-sha256>

{
  "external_order_id": "<stable-order-id>",
  "user_id": 123,
  "amount": "100.00",
  "currency": "CNY",
  "description": "<business-description>"
}
```

`source` 由服务端从 `X-Recharge-Key-Id` 的受控配置推导，不在 body 中接受。合法重试使用新的 timestamp 与 nonce，并保持同一订单业务字段；服务端应返回原结果。同一订单若金额、用户或币种不同，应返回 `409 EXTERNAL_ORDER_CONFLICT`。详细契约见 `api_spec.md`。

## 8. Gateway 客户端接入

客户端 Base URL 使用：

```text
https://<gateway-domain>/v1
```

认证头：

```http
Authorization: Bearer <user-api-key>
```

模型名只能使用后台已发布并授权给当前用户的逻辑模型。配置示例不得内嵌真实 Key，不应把 Key 放入命令历史、截图、Issue 或聊天记录。

上线前需要分别冻结并记录 Codex CLI、Cursor 和 SDK 版本。升级客户端后重新执行 `testing.md` 中对应的兼容矩阵。

## 9. 监控与告警

至少监控：

| 信号 | 告警建议 |
| --- | --- |
| `/health` | 连续失败立即告警 |
| 请求成功率 | 按 Provider、route、模型观察 |
| 首包与总延迟 | 区分 Streaming 和非 Streaming |
| 401、403 | 观察用户 Key 与上游凭据异常 |
| 429 | 区分用户限流、账号限流和上游限流 |
| 5xx、timeout | 按 Account 与 Provider 聚合 |
| Account 状态 | Cooldown、Exhausted、Disabled 变化告警 |
| Billing degraded | 立即告警并停止新请求 |
| 结算恢复日志 | 未归档记录、checksum 失败、持久卷写入或 fsync 失败立即告警 |
| 钱包对账差异 | 任意非零差异立即告警 |
| 价格缺失或多重命中 | 立即告警并 fail closed |
| External recharge 拒绝 | 签名、时间窗、nonce、IP 和冲突分别计数 |
| PostgreSQL、Redis | 可用性、连接池、磁盘、复制或持久化异常 |
| 备份 | 任务失败、文件过小、校验失败或超出 RPO |

日志默认只记录 metadata。发现 Prompt、Answer、Authorization、Cookie、OAuth token、API Key、数据库密码或 HMAC Secret 时，立即停止相关日志路径并按泄漏事件处理。

## 10. 备份

生产恢复采用基础备份加连续 WAL 归档与 PITR；每日逻辑备份只用于可移植导出和额外校验，不能单独承担账务系统恢复。所有备份应写入部署目录之外的受控目标并加密保存。以下命令仅是逻辑备份示例，执行前先创建并核对目标：

```bash
mkdir -p <approved-backup-directory>
docker compose -f compose.yaml exec -T postgres \
  pg_dump -U sub2api -d sub2api -Fc \
  > <approved-backup-directory>/sub2api-$(date -u +%Y%m%dT%H%M%SZ).dump

tar -C data -czf \
  <approved-backup-directory>/sub2api-data-$(date -u +%Y%m%dT%H%M%SZ).tar.gz .
```

每份备份记录：

* 应用 commit、image digest 和 migration 版本。
* PostgreSQL 与 Redis 版本。
* 文件大小、SHA256、创建时间和加密状态。
* Secret 版本标识，不记录 Secret 值。
* 恢复演练日期和结果。

WAL 归档必须持续监控，并定期恢复到指定时间点。M13 默认目标为 RPO 不高于 5 分钟、RTO 不高于 4 小时；未达到经批准的 RPO 前不得开启真实充值或收费流量。

Redis 数据可重建的范围需要在 M13 冻结。Nonce、防重放和并发状态丢失的影响必须单独评估。

## 11. 恢复演练

恢复只能在隔离环境先验证：

1. 固定与备份匹配的应用 image 和 PostgreSQL 版本。
2. 禁止外部流量和所有写入。
3. 从物理基础备份恢复 PostgreSQL，并重放 WAL 到批准的目标时间；逻辑 dump 只用于单独的可移植恢复演练。
4. 恢复应用配置、结算恢复日志持久卷和所需 Secret 版本。
5. 启动 PostgreSQL 与 Redis；应用发现 degraded sentinel 或未归档恢复记录时必须保持 Gateway fail closed。
6. 校验 migration、恢复时间线、用户数、余额汇总、钱包流水汇总、价格版本和 usage。
7. 如存在未归档结算记录，按 fingerprint 幂等重放；checksum 异常时停止并人工处理。
8. 完成账本与 usage 对账后，由 Admin step up 解除 Billing degraded。
9. 执行健康检查、只读 Portal 检查和无真实扣费的测试请求。
10. 记录实际 RPO、RTO、数据截止点和所有差异。

逻辑备份恢复的示意命令如下；PITR 命令必须根据 M13 选定的 PostgreSQL 或云服务方案单独固化：

```bash
docker compose -f compose.yaml exec -T postgres \
  pg_restore -U sub2api -d sub2api --clean --if-exists \
  < <approved-backup-file>
```

不要直接在生产数据库上练习 restore。Forward only migration 发生后，仅回滚应用镜像无法恢复旧 schema。

## 12. 升级流程

1. 阅读上游变更，确认 route、schema、migration、OAuth、模型和计费差异。
2. 在 Fork 建立升级分支，将 upstream commit 合入并解决冲突。
3. 更新 `CHANGELOG-custom.md` 中的 Upstream baseline。
4. 在隔离环境执行完整构建、测试和数据库迁移。
5. 生成升级前备份并完成 checksum。
6. 在 staging 使用脱敏数据执行三个 Provider 和客户端回归。
7. 固定新 image digest，制定变更窗口和恢复决策点。
8. 停止新请求或进入维护状态，等待在途 Streaming 请求结束。
9. 发布并验证 health、migration、价格解析、余额和关键请求。
10. 保留旧 image 与升级前备份，直到观察窗口结束。

## 13. 回滚原则

* 无数据库变更时，可以回滚到上一固定 image digest，并验证 health 和 smoke test。
* 已执行 forward only migration 时，按批准的恢复方案还原数据库和应用数据。
* 对充值、扣费和退款禁止通过直接改表回滚，应写补偿流水。
* 对 Secret 迁移禁止恢复已泄漏或已轮换的旧明文 Secret。
* 回滚后运行钱包对账，并确认重复请求的幂等状态仍然存在。

## 14. 常见故障处理

### 14.1 应用不健康

```bash
docker compose -f docker-compose.local.yml ps
docker compose -f docker-compose.local.yml logs --tail=300 sub2api
docker compose -f docker-compose.local.yml exec postgres pg_isready
docker compose -f docker-compose.local.yml exec redis redis-cli ping
```

重点检查 migration checksum、数据库认证、磁盘空间、Secret 缺失和端口绑定。

### 14.2 Streaming 中断或无首包

1. 检查 Nginx `proxy_buffering off`、读取超时和 gzip 设置。
2. 区分客户端断开、Gateway timeout、上游 timeout 和 Account cooldown。
3. 用同一客户端版本和 request ID 复现。
4. 核对是否已开始上游流量及是否产生实际 usage。
5. 不在日志中复制 Prompt 或完整 Answer。

### 14.3 用户余额异常

1. 暂停该用户的新请求和充值。
2. 按 `business_event_id` 核对 usage、`wallet_transactions` 和外部订单。
3. 运行只读对账，不直接修改历史流水。
4. 确认问题后用管理员 adjustment 写补偿交易。
5. 记录原因、审批人、关联事件和修复版本。

### 14.4 External recharge 重试异常

1. 核对调用方系统时间、Key ID 和允许 IP。
2. 确认签名使用原始 body bytes，字段顺序或空白变化会改变 body hash。
3. 每次新的业务请求使用新的 nonce，网络重试保留相同外部订单号。
4. 收到 409 时停止自动重试并人工核对原订单内容。
5. Redis 防重放不可用时入口保持 fail closed。

### 14.5 Provider 大面积 401 或 429

1. 按目标 Group 隔离受影响账号，禁止跨 Group fallback。
2. 检查 token 刷新、API Key 状态、quota 和 reset time。
3. 不通过提高重试次数掩盖认证错误。
4. 恢复前使用单一测试账号执行健康请求。

## 15. 安全事件快速响应

若怀疑 User API Key、Provider Secret、OAuth token 或 HMAC Secret 泄漏：

1. 立即禁用相关凭据和外部入口。
2. 保留脱敏日志、时间线、request ID 和审计证据。
3. 轮换受影响 Secret，保持各密钥域独立。
4. 检查数据库、备份、日志、CI artifact 和客户端配置的暴露范围。
5. 对异常用量和充值执行只读对账。
6. 完成影响评估后分批恢复，持续观察拒绝和异常流量。
7. 在 `progress.md` 和内部事件记录中登记修复与预防措施，禁止写入 Secret 值。

## 16. 日常交接最小清单

值班交接至少包含：

* 当前 commit、image digest 和 migration 版本。
* 三个 Provider 的健康状态及已禁用账号。
* Billing degraded、对账和价格告警状态。
* 用户余额或充值待处理事件。
* 最近一次成功备份与恢复演练时间。
* 正在处理的安全事件和到期密钥。
* 下一变更窗口、负责人和明确回滚点。
