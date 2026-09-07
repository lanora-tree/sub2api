# API 规格

## 1. 约定

V1 保留 Sub2API 现有 Gateway 协议和面板 API 风格。新增接口位于 `/api/v1` 下。Gateway 协议错误继续使用对应 OpenAI Compatible 错误结构，面板和 Internal API 使用统一业务错误结构。

### 1.1 Base URL

| 用途 | Base URL 示例 |
| --- | --- |
| Gateway | `https://api.example.invalid` |
| 面板 API | `https://api.example.invalid/api/v1` |
| Internal Recharge | `https://api.example.invalid/api/v1/internal` |

示例域名无效，不得把生产域名或 Secret 写入仓库。

### 1.2 内容类型和时间

* JSON 请求使用 `application/json; charset=utf-8`。
* Streaming 使用 `text/event-stream` 或现有 WebSocket 协议。
* 时间使用 RFC 3339 UTC 字符串。
* 金额使用十进制字符串，例如 `"12.34000000"`。
* Token 数量使用整数。
* ID 在后端为 BIGINT。前端接口建议以十进制字符串返回，避免 JavaScript 安全整数边界。

### 1.3 面板响应

成功：

```json
{
  "success": true,
  "data": {}
}
```

失败：

```json
{
  "success": false,
  "error": {
    "code": "MODEL_NOT_ALLOWED",
    "message": "当前用户无权使用该模型",
    "request_id": "req_example"
  }
}
```

实现时应先确认 Sub2API `backend/internal/handler/response` 的既有 envelope，并保持兼容。上面的结构是新增接口目标，若既有 envelope 不同则标记需要人工确认。

## 2. 权限模型

| 权限 | 认证 | 作用域 |
| --- | --- | --- |
| Gateway User | 用户 API Key | 当前 Key、当前 User、绑定的 access Group |
| Portal User | JWT Access Token | 当前 User |
| Admin | Admin JWT，可选 TOTP step up | 全站管理资源 |
| Internal Recharger | HMAC Key ID、签名、来源 IP | 仅充值写入与订单结果 |

所有 Portal User 查询从 JWT Subject 获取 User ID。请求体或 query 中出现的 `user_id` 不得改变作用域。

## 3. Gateway API

现有路由定义位于 `backend/internal/server/routes/gateway.go`。V1 最低支持：

### 3.1 Models

```http
GET /v1/models
Authorization: Bearer sk_example
```

只返回以下交集：

1. V1 显式 route 已启用。
2. User 拥有 `user_model_permissions`。
3. 目标 Group 已启用。瞬时账号健康不参与模型目录生成，避免客户端缓存的模型列表因 Cooldown 抖动。
4. 存在当前有效 CNY 价格版本。

响应遵循 OpenAI Models list。不得泄漏 `target_group_id`、账号数量、账号 ID 或内部成本。

### 3.2 Responses

```http
POST /v1/responses
Authorization: Bearer sk_example
Content-Type: application/json

{
  "model": "codex-logical-model",
  "input": "Return OK",
  "stream": true
}
```

适用于 Codex Subscription 和经验证的 OpenAI Official 模型。继续支持现有 Responses 子路径以及仓库已有 root alias。具体兼容矩阵见 `testing.md`。

### 3.3 Chat Completions

```http
POST /v1/chat/completions
Authorization: Bearer sk_example
Content-Type: application/json

{
  "model": "deepseek-v4-flash",
  "messages": [
    {"role": "user", "content": "Return OK"}
  ],
  "stream": true,
  "stream_options": {"include_usage": true}
}
```

适用于 OpenAI Official 与 DeepSeek。是否向 Cursor 开放 Responses 由客户端实测决定。

### 3.4 Gateway 预检顺序

1. 解析并摘要 API Key。
2. 检查 Key 状态、过期、IP ACL。
3. 检查 User 状态。
4. 检查 access Group。
5. 精确解析 route。
6. 检查 User model permission。
7. 检查价格版本。
8. 检查 `balance > 0`。
9. 检查 RPM 与并发。
10. 调用 target Group Scheduler。

只有 usage、模型列表和受控健康类路径可以按现有规则跳过余额检查。其他模型生成请求禁止绕过。

### 3.5 Gateway 错误

保持 OpenAI Compatible 结构：

```json
{
  "error": {
    "message": "No active price is configured for this model",
    "type": "invalid_request_error",
    "code": "PRICE_NOT_CONFIGURED",
    "param": "model"
  }
}
```

| HTTP | Code | 条件 | 可重试 |
| --- | --- | --- | --- |
| 401 | `INVALID_API_KEY` | Key 缺失或摘要不匹配 | 否 |
| 403 | `API_KEY_DISABLED` | Key 被禁用 | 否 |
| 403 | `USER_DISABLED` | User 被禁用 | 否 |
| 403 | `MODEL_NOT_ALLOWED` | 无模型权限 | 否 |
| 403 | `INSUFFICIENT_BALANCE` | 新请求开始前余额小于等于零 | 充值后可重试 |
| 404 | `MODEL_ROUTE_NOT_FOUND` | 未命中显式 route | 否 |
| 503 | `BILLING_DEGRADED` | 结算系统处于安全降级 | 是，遵守 `Retry-After` |
| 429 | `RATE_LIMITED` | 用户或 Key 限流 | 是，遵守 Retry After |
| 503 | `PROVIDER_UNAVAILABLE` | 目标 Group 无可用账号 | 是 |
| 504 | `UPSTREAM_TIMEOUT` | 上游超时 | 视幂等性决定 |

`PROVIDER_UNAVAILABLE` 响应不得暗示或触发其他 Provider 自动接管。

`BILLING_DEGRADED` 响应必须带 `Retry-After`，且服务重启后只要仍有 degraded sentinel 或未归档结算恢复记录就继续返回该错误。

## 4. User API

现有 User routes 位于 `backend/internal/server/routes/user.go`。V1 复用并裁剪为以下能力。

### 4.1 Profile 与钱包摘要

```http
GET /api/v1/user/profile
Authorization: Bearer jwt_access_token
```

返回当前用户基本信息。余额字段改为：

```json
{
  "id": "42",
  "email": "member@example.invalid",
  "role": "user",
  "status": "active",
  "wallet": {
    "currency": "CNY",
    "balance": "96.35781234"
  }
}
```

新增：

```http
GET /api/v1/user/wallet/transactions?page=1&page_size=20&type=usage
```

返回当前用户流水，禁止接受其他 User ID。

### 4.2 API Key

保留：

```http
GET    /api/v1/keys
POST   /api/v1/keys
PUT    /api/v1/keys/{id}
DELETE /api/v1/keys/{id}
```

创建请求：

```json
{
  "name": "Laptop Codex",
  "expires_at": "2027-09-05T00:00:00Z",
  "ip_whitelist": []
}
```

创建成功仅返回一次完整 Secret：

```json
{
  "success": true,
  "data": {
    "id": "108",
    "name": "Laptop Codex",
    "secret": "sk_generated_once",
    "masked_key": "sk_abcd********wxyz",
    "status": "active",
    "created_at": "2026-09-05T08:00:00Z"
  }
}
```

列表和详情响应没有 `secret` 和 `key` 字段，只返回 `masked_key`。服务端必须验证 Key 属于当前用户。

### 4.3 Usage 与 Dashboard

复用：

```http
GET /api/v1/usage
GET /api/v1/usage/{id}
GET /api/v1/usage/stats
GET /api/v1/usage/dashboard/stats
GET /api/v1/usage/dashboard/trend
GET /api/v1/usage/dashboard/models
```

V1 返回字段：

* request ID
* API Key 名称与 masked key
* Provider 显示名
* 逻辑模型
* input、output、cache Token
* CNY 费用字符串
* latency
* status code 与脱敏错误类型
* created at

不返回 account ID、上游 request body、Prompt、Answer、上游 Key 或内部成本。

### 4.4 可用模型与接入配置

新增或精简：

```http
GET /api/v1/user/models
GET /api/v1/user/client-config
```

`client-config` 只返回公开 Base URL、模型名和无 Secret 的 Codex、Cursor 配置片段。

## 5. Admin API

现有 Admin routes 位于 `backend/internal/server/routes/admin.go`。继续复用用户、账号、Group、Usage 和 Settings CRUD。

### 5.1 用户钱包操作

保留现有 `POST /api/v1/admin/users/{id}/balance` 的兼容入口，内部统一进入 Wallet Service。新增更明确的资源 API：

```http
POST /api/v1/admin/users/{id}/wallet/transactions
Authorization: Bearer admin_jwt
Idempotency-Key: admin-operation-unique-id
```

请求：

```json
{
  "type": "recharge",
  "amount": "100.00",
  "note": "课题组 9 月额度"
}
```

`amount` 对 recharge、refund 为正。adjustment 可以为正或负。管理员负向调账不得使余额小于零。

响应：

```json
{
  "success": true,
  "data": {
    "transaction_id": "9001",
    "user_id": "42",
    "type": "recharge",
    "amount": "100.00000000",
    "balance_before": "0.00000000",
    "balance_after": "100.00000000",
    "currency": "CNY",
    "created_at": "2026-09-05T08:00:00Z"
  }
}
```

同一 `Idempotency-Key` 和相同 body 返回原结果。Key 相同且 body 不同返回 409 `IDEMPOTENCY_CONFLICT`。

### 5.2 用户模型权限

```http
GET /api/v1/admin/users/{id}/model-permissions
PUT /api/v1/admin/users/{id}/model-permissions
```

PUT 使用完整替换并要求 `If-Match` 或 version，避免并发覆盖：

```json
{
  "version": 3,
  "route_ids": ["11", "12", "21"]
}
```

### 5.3 V1 Route

扩展现有 Composite route API：

```http
GET  /api/v1/admin/groups/{id}/composite-routes
POST /api/v1/admin/groups/{id}/composite-routes
PUT  /api/v1/admin/groups/{id}/composite-routes/{route_id}
```

V1 创建请求：

```json
{
  "public_model": "codex-logical-model",
  "match_type": "exact",
  "target_platform": "openai",
  "target_group_id": "101",
  "upstream_model": "gpt-current-codex-model",
  "endpoint": "responses",
  "priority": 10,
  "enabled": true,
  "notes": "V1 Codex route"
}
```

校验失败包括 target Group 不存在、平台不一致、目标也是 Composite、route 冲突和 V1 使用 prefix。

如果 Route 已被任何 `user_model_permissions` 引用，PUT 不允许修改 public model、target platform、target Group、upstream model 或 endpoint。管理员必须 POST 新 Route、显式迁移权限并停用旧 Route；避免既有授权静默指向另一模型或成本池。

### 5.4 模型价格

```http
GET  /api/v1/admin/pricing/rules
POST /api/v1/admin/pricing/rules
GET  /api/v1/admin/pricing/rules/{id}/versions
POST /api/v1/admin/pricing/rules/{id}/versions
POST /api/v1/admin/pricing/rules/{id}/enable
POST /api/v1/admin/pricing/rules/{id}/disable
```

创建价格版本：

```json
{
  "input_price": "1.50",
  "output_price": "6.00",
  "cache_input_price": "0.15",
  "cache_write_price": null,
  "usage_price": null,
  "effective_from": "2026-09-06T00:00:00Z",
  "note": "上游调价后的内部价格"
}
```

发布新版本关闭旧版本有效区间。已经生效的版本禁止原地编辑。

### 5.5 账号 Secret

账号列表只返回 masked credential 和 `credential_configured=true`。创建或轮换上游 Key 的响应不回显持久化 Secret。OpenAI OAuth 流程继续复用现有受控 Admin endpoints。

## 6. Internal Recharge API

### 6.1 Endpoint

```http
POST /api/v1/internal/wallet/recharges
Content-Type: application/json
X-Recharge-Key-Id: billing-site-v1
X-Recharge-Timestamp: 1788595200
X-Recharge-Nonce: unique-random-128-bit-value
X-Recharge-Signature: lowercase-hex-hmac-sha256
```

Body：

```json
{
  "external_order_id": "order_20260905_0001",
  "user_id": "42",
  "amount": "100.00",
  "currency": "CNY",
  "description": "September allocation"
}
```

`source` 不接受客户端指定，JSON decoder 对包括 `source` 在内的未知字段返回 400。服务端从已认证的 `X-Recharge-Key-Id` 配置中取得绑定的 source，并把该 source 写入订单、流水和审计。一个 Key ID 只能绑定一个 source；轮换 Key 时新旧 Key ID 可以在受控窗口内绑定同一 source。`description` 可选、最长 500 字符，只在首次成功入账时保存为脱敏 note，不参与订单 fingerprint。

### 6.2 签名

服务端使用收到的原始 body bytes 计算：

```text
body_hash = lowercase_hex(SHA256(raw_body))
canonical = method + "\n" + path + "\n" + timestamp + "\n" + nonce + "\n" + body_hash
signature = lowercase_hex(HMAC_SHA256(secret_for_key_id, canonical))
```

Path 固定使用 `/api/v1/internal/wallet/recharges`，不包含 scheme、host 和 query。比较签名时使用 constant time compare。

### 6.3 校验顺序

1. Nginx 来源 IP Allowlist。
2. 请求体大小上限 16 KiB。
3. Key ID 存在且处于轮换有效期。
4. Timestamp 与服务器时间相差不超过 300 秒。
5. Nonce 格式正确。
6. 使用原始 body bytes 验证 HMAC 签名，使用 constant time compare。
7. 签名通过后使用 Redis `SET NX EX` 认领 Nonce；重放或 Redis 故障时 fail closed。
8. Body schema、CNY、金额和 User。
9. 从 Key ID 推导 source，计算规范业务 fingerprint。
10. PostgreSQL 业务幂等事务。

业务 fingerprint 固定为以下 UTF-8 字符串的 SHA256，不使用原始 JSON 字段顺序：

```text
source + "\n" + external_order_id + "\n" + canonical_user_id + "\n" + canonical_amount_8dp + "\nCNY"
```

`canonical_user_id` 是无前导零的十进制 ID，`canonical_amount_8dp` 是校验范围后补齐到八位小数的正金额。说明文字不进入 fingerprint；同一订单的金额、用户、币种或绑定 source 变化均视为冲突。

### 6.4 成功与重放

首次应用返回 201：

```json
{
  "success": true,
  "data": {
    "status": "applied",
    "external_order_id": "order_20260905_0001",
    "transaction_id": "9002",
    "user_id": "42",
    "amount": "100.00000000",
    "balance_after": "196.35781234"
  }
}
```

相同 source、order ID 和业务 fingerprint 返回 200，`status` 为 `replayed`，其余结果来自已完成记录。

合法网络重试必须使用新的 Timestamp 和 Nonce，但保持相同业务字段。HMAC 覆盖新的 header 与原始 body，数据库仍按规范业务 fingerprint 返回原结果。

相同订单号且 User、金额或币种不同返回 409：

```json
{
  "success": false,
  "error": {
    "code": "EXTERNAL_ORDER_CONFLICT",
    "message": "订单号已存在且请求内容不一致",
    "request_id": "req_example"
  }
}
```

### 6.5 Internal 错误码

| HTTP | Code | 说明 |
| --- | --- | --- |
| 400 | `INVALID_AMOUNT` | 金额格式、精度或范围无效 |
| 400 | `INVALID_CURRENCY` | V1 只接受 CNY |
| 401 | `INVALID_INTERNAL_KEY` | Key ID 不存在或已停用 |
| 401 | `INVALID_SIGNATURE` | 签名不匹配 |
| 401 | `REQUEST_EXPIRED` | 超出时间窗 |
| 409 | `NONCE_REPLAYED` | Nonce 已使用 |
| 409 | `EXTERNAL_ORDER_CONFLICT` | 订单内容冲突 |
| 422 | `USER_NOT_FOUND` | User 不存在 |
| 422 | `USER_DISABLED` | User 被禁用，是否允许充值需人工确认，V1 默认拒绝 |
| 429 | `INTERNAL_RATE_LIMITED` | 来源限流 |
| 503 | `REPLAY_STORE_UNAVAILABLE` | Redis 防重放不可用 |
| 503 | `DATABASE_UNAVAILABLE` | 无法安全入账 |

## 7. 功能关闭接口行为

V1 关闭以下路由时，服务端统一返回 404，降低公开功能暴露：

* `/api/v1/auth/register`
* `/api/v1/payment/*`
* `/api/v1/payment/public/*`
* `/api/v1/payment/webhook/*`
* `/api/v1/redeem`
* OAuth signup start 与 callback
* Model Plaza 和公开渠道页面 API

管理员管理现有支付模块的路由也应在 V1 站点策略下隐藏或返回 404。登录、密码修改、TOTP、Admin 创建用户保持可用。

## 8. 限流建议

| 接口 | 初始限制 | 失败模式 |
| --- | --- | --- |
| Login | 延用现有每 IP 限制 | Redis 故障 fail closed |
| Gateway | User RPM 加 Key 限制 | Redis 故障按现有安全策略，需 M12 实测 |
| Admin wallet write | 每 Admin 每分钟 30 次 | fail closed |
| Internal recharge | 每 source 每分钟 30 次，burst 10 | fail closed |
| User usage query | 延用 panel heavy limiter | fail closed |

实际阈值在 M14 根据正常流量调整。

## 9. 审计要求

以下请求必须写 `audit_logs`：

1. Admin 用户创建、状态和权限变更。
2. API Key 创建、禁用、删除和轮换，只记录 masked key。
3. Admin 充值、退款和调账。
4. Route 与价格规则变更。
5. 上游账号创建、凭据轮换、启停和导出尝试。
6. Internal Recharge 的 applied、replayed、conflict 和鉴权失败。

审计记录不保存 Authorization、完整 Cookie、完整签名、Nonce 原文、Prompt、Answer 和 Secret。
