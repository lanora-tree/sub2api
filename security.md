# 安全设计

## 1. 安全目标

V1 面向可信小群体，仍按公网服务安全边界设计。核心目标是保护用户数据、上游凭据、钱包余额、价格和充值入口，防止越权、重放、Secret 泄漏、误路由和账务篡改。

## 2. Threat Model

| 威胁主体 | 主要能力 | 重点风险 |
| --- | --- | --- |
| 未认证公网访问者 | 扫描、爆破、构造请求、DDoS | 登录爆破、公开支付或注册入口、资源耗尽 |
| 普通 User | 合法 JWT 与 API Key | 横向越权、读取他人 usage、探测账号池、超额并发 |
| API Key 泄漏者 | 调用 Gateway | 盗刷余额、Prompt 泄漏、滥用 Provider |
| 外部收费站被攻破 | 持有 Internal Key | 批量虚假充值、重放、订单篡改 |
| Admin 账号被攻破 | 管理权限 | 导出凭据、调价、调账、禁用审计 |
| 数据库备份泄漏 | 读取持久化数据 | 用户 Key、上游 Token、密码散列、账务和个人信息泄漏 |
| 供应链攻击者 | 污染镜像或依赖 | 代码执行、Secret 窃取、账单篡改 |
| 错误配置 | 暴露端口或启用 fallback | 数据泄漏、跨成本池误路由、意外费用 |

## 3. 当前源码安全发现

### 3.1 可复用能力

1. 密码使用 `password_hash`，具体算法需在 M0 安全基线确认。
2. Auth 高风险入口有 Redis 限流且多处 fail closed，见 `backend/internal/server/routes/auth.go`。
3. API Key 支持状态、过期与 IP ACL，见 `backend/ent/schema/api_key.go`。
4. Admin 和 User routes 有独立 JWT middleware。
5. `audit_logs` 为 append only 管理审计，并包含敏感字段脱敏，见 `backend/migrations/180_audit_logs.sql` 与 `backend/internal/service/audit_log.go`。
6. URL allowlist、trusted proxies、TOTP、step up、WebAuthn 等能力已经存在。
7. `deploy/EDGE_SECURITY.md` 提供 Nginx、Caddy、SSE 和 trusted proxy 基线。

### 3.2 必须修复的风险

1. `api_keys.key` 保存完整明文并通过 `KeyEQ` 查询，列表 DTO 也含完整 Key。
2. `accounts.credentials` 直接写 JSONB。`001_init.sql` 注释写“加密存储”，实际 `account_repo.go` 直接 `SetCredentials`。
3. `security_secrets.value` 为数据库明文，不适合保存 HMAC 或上游凭据。
4. 金额热路径使用 float64，存在精度和审计风险。
5. Payment webhook routes 始终注册，需要确认 `payment_enabled=false` 时是否在 handler 层全部拒绝。
6. Composite 内置模型检测和 Group fallback 可能引起非预期路由。

## 4. Secret 分类

| Secret | 存储 | 可读主体 | 轮换 |
| --- | --- | --- | --- |
| 用户 API Key 原文 | 仅创建响应和用户安全存储 | User | 可单 Key 撤销和新建 |
| API Key HMAC pepper | 生产 Secret 文件或 Secret Manager | Backend | 版本化双读、认证时渐进重算，逾期 Key 强制轮换 |
| OpenAI 与 DeepSeek API Key | AES 256 GCM 密文 | Backend Provider runtime | 每账号独立轮换 |
| OAuth access 与 refresh Token | AES 256 GCM 密文 | OAuth refresh 与 Provider runtime | 自动刷新，泄漏时撤销 |
| Account encryption key | Secret Manager 或 root only 文件 | Backend | key version 渐进重加密 |
| Billing recovery encryption key | Secret Manager 或 root only 文件 | 结算恢复组件 | key version 渐进重加密，独立于 Account key |
| JWT Secret | 固定安全配置 | Backend | 会使会话失效，计划轮换 |
| TOTP encryption key | 固定安全配置 | Backend | 独立于其他密钥 |
| PostgreSQL password | `.env` root only 或 Secret Manager | Backend 与运维 | 定期轮换 |
| Redis password | `.env` root only | Backend 与运维 | 定期轮换 |
| Internal Recharge HMAC | Secret Manager 或 root only 文件 | Backend 与外部收费站 | Key ID 双密钥窗口 |
| TLS private key | ACME 或 root only Nginx path | Nginx | 自动续期 |

禁止复用 JWT、TOTP、Account encryption、Billing recovery encryption、API Key pepper 和 Recharge HMAC 密钥。

## 5. 用户 API Key

### 5.1 生成

1. 使用 cryptographic random 生成至少 32 bytes。
2. 使用可识别前缀，例如 `sk_gw_`，前缀不代表权限。
3. 完整 Secret 只在创建成功响应中出现一次。
4. 响应设置 `Cache-Control: no-store` 和 `Pragma: no-cache`。
5. 前端不写 localStorage，不写 analytics，不在错误追踪中捕获 Secret。

### 5.2 存储与认证

数据库保存：

* `key_hash = HMAC_SHA256(pepper_version, full_key)`
* `key_prefix`
* `key_last4`
* `key_hash_version`

认证时从 Authorization 取 Key，在内存计算 HMAC，通过唯一索引查询摘要。签名比较使用 constant time。完整 Key 不进入结构化 logger、Redis value 或 trace attribute。

Pepper 轮换不能脱离 Key 原文离线重算。轮换窗口内后端只加载 active 与 previous 两个版本：先计算 active 摘要查询，未命中再计算 previous 摘要；previous 命中后在同一受控更新中改写为 active 摘要和版本。轮换截止时仍未登录的旧版本 Key 必须撤销并要求用户创建新 Key，随后移除 previous pepper。不得无限期保留旧 pepper。

### 5.3 迁移

旧明文 Key 加摘要后必须轮换。历史数据库备份仍含原文，需缩短保留期并加密保存。迁移完成后扫描数据库、导出、日志和对象存储，确认无可用明文 Key。

## 6. 上游账号凭据

使用独立 AES 256 GCM envelope：

1. 每条凭据随机 Nonce。
2. AAD 包含每账号独立 `credentials_aad_id`、platform 和 key version。
3. 密文和 key version 写数据库，主密钥在数据库外。
4. 解密只发生在 Provider 调用与 Token refresh 的最小范围。
5. 内存日志和 panic recovery 禁止 dump credentials map。
6. Admin DTO 只返回 masked value 和 configured flag。
7. 备份只包含密文，主密钥单独保存。

M6.5 使用独立的 versioned keyring 实现 AES-256-GCM、keyed fingerprint 和有限 active/previous 轮换；Account 凭据密钥域不复用 TOTP Key。

## 7. 身份、Admin 与权限

1. 关闭公开注册和所有第三方 signup。
2. Admin 通过现有 Admin API 创建 User。
3. User 角色只保留 `admin` 与 `user`。
4. Admin 强制使用长密码和 TOTP，敏感操作启用 step up。
5. Admin session 建议 8 小时过期，step up 建议 10 分钟。
6. 管理面可以在 Nginx 增加 VPN 或管理 IP Allowlist，是否启用需要人工确认。
7. 禁止普通 User 调用账号、Group、价格、钱包写入和系统设置 API。
8. 用户资源 repository 查询必须同时带当前 User ID，避免先按 ID 查询再在 handler 判断。
9. 所有 404 与 403 避免泄漏其他用户资源是否存在。

## 8. Internal Recharge API

### 8.1 多层控制

1. Nginx 来源 IP Allowlist。
2. 独立 Host 或严格 path 限制。
3. HMAC Key ID 与 HMAC SHA256。
4. 原始 body SHA256，签名覆盖 method、path、timestamp、nonce 和 body hash。
5. Timestamp 五分钟时间窗。
6. 128 bit 以上随机 Nonce。
7. Redis `SET NX EX` 防短期重放，Redis 失败时 fail closed。
8. PostgreSQL `(source, external_order_id)` 唯一约束防长期重复充值。
9. 每 source 与 IP 限流。
10. 最大 body 16 KiB，禁止压缩请求体。
11. applied、replayed、conflict 和失败均写脱敏审计。

每个 Internal Key ID 在服务端配置中绑定唯一 source。请求体不接受 source，数据库和审计中的 source 只能从已认证 Key ID 推导，防止调用方冒充其他收费站。

### 8.2 密钥轮换

配置至少支持 current 与 previous 两个 Key ID。发送方切换到新 Key 后观察一个最大重试周期，再停用旧 Key。停用和启用操作写审计，Secret 原文不进入数据库设置页面。

### 8.3 防重放细节

Nonce Redis key 使用 `recharge:nonce:{key_id}:{sha256(nonce)}`，TTL 至少 10 分钟，只存 hash。处理顺序为来源 IP、大小、Key ID、时间窗、Nonce 格式、HMAC 验签，然后使用 Redis `SET NX EX` 原子认领 nonce。签名未通过的请求不得占用 nonce；签名通过后 Redis 不可用则 fail closed。

同订单合法重试应使用新 Nonce 与新 timestamp。数据库返回 replayed 结果。

## 9. Nginx 与 HTTPS

生产要求：

1. 80 只做 ACME 与 301 到 HTTPS。
2. 443 使用 TLS 1.2 或更高版本，优先 TLS 1.3。
3. HSTS 在确认所有子域 HTTPS 后启用。
4. Backend 只监听回环或 Compose 内网。
5. 覆写 `X-Forwarded-For`，只信任明确代理地址。
6. `proxy_buffering off` 与 `proxy_request_buffering off`。
7. SSE 不压缩。
8. `proxy_read_timeout` 和 `proxy_send_timeout` 支持长 Codex 请求。
9. WebSocket 正确处理 Upgrade 与 Connection。
10. 限制 header、body、连接数和 auth rate。
11. 保留 Session 相关请求头，禁止在 proxy 层统一删除未知 `X` header。

Nginx 示例见 `deployment.md`，上线前对照 `deploy/EDGE_SECURITY.md`。

## 10. PostgreSQL 与 Redis

### 10.1 PostgreSQL

* 不发布公网端口。
* 独立数据库用户，只授予应用所需权限。
* 生产启用存储卷加密和备份加密。
* 禁止在查询日志记录参数中的 Secret。
* Admin 数据导出视为敏感操作，要求 step up 并写审计。
* Migration 账号与运行账号是否分离需要人工确认。

### 10.2 Redis

* 不发布公网端口。
* 设置密码，Compose 内网访问。
* 保存缓存、限流、并发与 Sticky，不保存唯一业务事实。
* 缓存值避免完整 API Key 和 Token。
* 认证、HMAC 防重放与关键限流在 Redis 不可用时 fail closed。
* Redis 重启后的并发槽位与 Sticky 行为必须测试。

## 11. 路由与成本安全

1. V1 Composite Group 开启 only explicit routes。
2. 每条 route 锁定 target Group。
3. target Group 平台必须与 target platform 一致。
4. 所有 Group fallback 为空。
5. 未配置价格、权限或 route 时请求开始前拒绝。
6. Provider 停用时禁止自动落入另一个 Provider。
7. Route 与价格修改要求 Admin step up、审计和版本控制。
8. 发布前自动检查每个开放 route 只有一个目标 Group 和一个有效价格。

## 12. 日志与隐私

默认记录：request ID、User ID、API Key ID、masked key、Provider、Group、Account ID 仅 Admin、逻辑模型、实际模型、Token、费用、延迟、状态和错误类型。

默认禁止：

* Prompt 与 Answer
* Authorization 与 Cookie
* API Key、OAuth Token、refresh Token
* Internal Signature 与 HMAC Secret
* 完整请求或响应 body
* 未脱敏上游错误 body

Debug body capture 如未来启用，必须满足：

1. 默认关闭。
2. 只允许 Admin step up 临时启用。
3. 最长 30 分钟自动过期。
4. 对 User、Key、模型做窄范围选择。
5. 加密存储并设置短保留期。
6. 仍执行 Secret redaction。
7. 每次启停写审计。

V1 建议暂不实现 Prompt capture。

## 13. 备份安全

1. PostgreSQL 物理基础备份、连续 WAL 和每日逻辑备份均使用强加密后上传异地位置。
2. Account encryption key、HMAC key 和 API Key pepper 与数据库备份分开保存。
3. `.env` 和配置备份加密，权限 600。
4. 恢复环境隔离网络，禁止恢复后自动连接真实 Provider。
5. 恢复演练使用替换后的测试 Secret。
6. 记录备份 checksum、时间、大小、加密 key ID 和恢复结果。
7. 删除过期备份前确认保留策略和合规要求。
8. 本地结算恢复日志位于独立持久卷，使用独立密钥加密并带 checksum；日志采集系统不得采集其正文，归档前不得删除。

## 14. 供应链与部署

1. 固定 Sub2API commit、release tag 和容器 digest。
2. 构建镜像保留 SBOM 和依赖扫描结果。
3. 禁止直接部署 `latest` 到生产。
4. 依赖升级单独提交并运行完整回归。
5. 容器启用 `no-new-privileges`，尽量使用非 root User 和只读文件系统。
6. `.env`、数据目录和备份目录不进入 Git。
7. 发布前执行 Secret scan。

## 15. 合规与许可证

Sub2API 根目录 `LICENSE` 是 GNU LGPL 3.0，README 标记为 LGPL 3.0 或更高版本。README_CN 同时包含“仅供学习研究”和“无商业授权”声明，两者的法律关系需要专业审核。保留原版权、LICENSE 和 NOTICE 类声明，分发修改版本前确认 LGPL 对源代码、修改、链接与安装信息的具体义务。

OpenAI 当前条款限制共享账号凭据、转售账号访问和规避 Usage Limits。Codex Subscription 多账号池是 V1 最大合规风险。技术可行不代表获得服务授权。M14 前必须有书面人工结论。

本文件只记录风险与工程控制，不提供法律意见。

## 16. 安全验收

1. 数据库、Redis、日志和备份搜索不到可用的用户 API Key 原文。
2. 上游凭据只有密文，主密钥在数据库外。
3. 两个 User 互相访问资源均失败。
4. 普通 User 无法读取 target Group、Account 或上游成本。
5. Payment、注册、OAuth signup 和 redeem 直接调用被服务端拒绝。
6. HMAC 错误、过期、Nonce 重放、订单冲突和非白名单 IP 均被拒绝。
7. 路由无跨 Group fallback。
8. Nginx SSL、SSE、WebSocket、trusted proxy 和端口扫描通过。
9. Admin 高风险操作有 step up 和完整审计。
10. Secret scan、依赖扫描和备份恢复测试通过。

## 17. 事件响应

发现 Secret 泄漏时：

1. 立即停用受影响 Key 或账号。
2. 暂停对应 Provider 或 Internal Recharge。
3. 保存脱敏审计证据。
4. 轮换 Secret，并清理缓存与会话。
5. 检查异常调用和账务影响。
6. 更新 `findings.md` 与 `progress.md`。
7. 完成复盘和回归测试后再恢复。
