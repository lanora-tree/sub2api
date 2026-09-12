# ADMIN_PAYMENT_INTEGRATION_API

> 单文件中英双语文档 / Single-file bilingual documentation (Chinese + English)

---

## 中文

> V1 安全提示：M2 已关闭旧支付、兑换码和外部回调能力。本文件中的服务间充值流程在 M8 安全 Internal Recharge API 完成前不得用于真实资金；M6.2 的钱包写接口仅供受审计的管理员人工操作。

### 目标
本文档用于对接外部支付系统（如 `sub2apipay`）与 Sub2API 的 Admin API，覆盖：
- 支付成功后充值
- 用户查询
- 人工余额修正
- 前端购买页参数透传

### 基础地址
- 生产：`https://<your-domain>`
- Beta：`http://<your-server-ip>:8084`

### 认证
推荐使用：
- `x-api-key: admin-<64hex>`
- `Content-Type: application/json`
- 幂等接口额外传：`Idempotency-Key`

说明：管理员 JWT 也可访问 admin 路由，但服务间调用建议使用 Admin API Key。

### 1) 一步完成创建并兑换
`POST /api/v1/admin/redeem-codes/create-and-redeem`

用途：原子完成“创建兑换码 + 兑换到指定用户”。

请求头：
- `x-api-key`
- `Idempotency-Key`

请求体示例：
```json
{
  "code": "s2p_cm1234567890",
  "type": "balance",
  "value": 100.0,
  "user_id": 123,
  "notes": "sub2apipay order: cm1234567890"
}
```

幂等语义：
- 同 `code` 且 `used_by` 一致：`200`
- 同 `code` 但 `used_by` 不一致：`409`
- 缺少 `Idempotency-Key`：`400`（`IDEMPOTENCY_KEY_REQUIRED`）

curl 示例：
```bash
curl -X POST "${BASE}/api/v1/admin/redeem-codes/create-and-redeem" \
  -H "x-api-key: ${KEY}" \
  -H "Idempotency-Key: pay-cm1234567890-success" \
  -H "Content-Type: application/json" \
  -d '{
    "code":"s2p_cm1234567890",
    "type":"balance",
    "value":100.00,
    "user_id":123,
    "notes":"sub2apipay order: cm1234567890"
  }'
```

### 2) 查询用户（可选前置校验）
`GET /api/v1/admin/users/:id`

```bash
curl -s "${BASE}/api/v1/admin/users/123" \
  -H "x-api-key: ${KEY}"
```

### 3) 管理员钱包操作（M6.2）
`POST /api/v1/admin/users/:id/wallet/transactions`

用途：人工充值或正负调账。`amount` 必须是字符串；`recharge` 为正数且最多两位小数，`adjustment` 为非零有符号数且最多八位小数。`note` 必填，负调账不能使余额小于零。不再支持无流水的 `set`。

请求体示例（扣减调账）：
```json
{
  "amount": "-100.00",
  "operation": "adjustment",
  "note": "support ticket CM-123"
}
```

```bash
curl -X POST "${BASE}/api/v1/admin/users/123/wallet/transactions" \
  -H "Authorization: Bearer ${ADMIN_JWT}" \
  -H "Idempotency-Key: balance-subtract-cm1234567890" \
  -H "Content-Type: application/json" \
  -d '{
    "amount":"-100.00",
    "operation":"adjustment",
    "note":"support ticket CM-123"
  }'
```

启用 step-up 时，写操作必须使用近期完成 TOTP 二次验证的管理员 JWT；Admin API Key 会被拒绝。旧 `/balance` 路径仅保留为相同严格契约的兼容别名。

余额查询：`GET /api/v1/admin/users/:id/wallet`。一次性全额 usage 退款：`POST /api/v1/admin/users/:id/wallet/refunds`，请求体为 `{"original_transaction_id":44,"note":"approved ticket CM-124"}`，同样要求 `Idempotency-Key`。普通用户只能查询 `GET /api/v1/user/wallet`，服务端忽略任何外部 user ID。

### 4) 购买页 / 自定义页面 URL Query 透传（iframe / 新窗口一致）
当 Sub2API 打开 `purchase_subscription_url` 或用户侧自定义页面 iframe URL 时，会统一追加：
- `user_id`
- `token`
- `theme`（`light` / `dark`）
- `lang`（例如 `zh` / `en`，用于向嵌入页传递当前界面语言）
- `ui_mode`（固定 `embedded`）

示例：
```text
https://pay.example.com/pay?user_id=123&token=<jwt>&theme=light&lang=zh&ui_mode=embedded
```

### 5) 失败处理建议
- 支付成功与充值成功分状态落库
- 回调验签成功后立即标记“支付成功”
- 支付成功但充值失败的订单允许后续重试
- 重试保持相同 `code`，并使用新的 `Idempotency-Key`

### 6) `doc_url` 配置建议
- 查看链接：`https://github.com/Wei-Shaw/sub2api/blob/main/docs/ADMIN_PAYMENT_INTEGRATION_API.md`
- 下载链接：`https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/docs/ADMIN_PAYMENT_INTEGRATION_API.md`

---

## English

> V1 security note: M2 disables the legacy payment, redeem-code, and external callback surfaces. Do not use the server-to-server funding flow in this document for real funds until the M8 Internal Recharge API is complete. The M6.2 wallet write endpoints are for audited manual administrator actions only.

### Purpose
This document describes the minimal Sub2API Admin API surface for external payment integrations (for example, `sub2apipay`), including:
- Recharge after payment success
- User lookup
- Manual balance correction
- Purchase page query parameter forwarding

### Base URL
- Production: `https://<your-domain>`
- Beta: `http://<your-server-ip>:8084`

### Authentication
Recommended headers:
- `x-api-key: admin-<64hex>`
- `Content-Type: application/json`
- `Idempotency-Key` for idempotent endpoints

Note: Admin JWT can also access admin routes, but Admin API Key is recommended for server-to-server integration.

### 1) Create and Redeem in one step
`POST /api/v1/admin/redeem-codes/create-and-redeem`

Use case: atomically create a redeem code and redeem it to a target user.

Headers:
- `x-api-key`
- `Idempotency-Key`

Request body:
```json
{
  "code": "s2p_cm1234567890",
  "type": "balance",
  "value": 100.0,
  "user_id": 123,
  "notes": "sub2apipay order: cm1234567890"
}
```

Idempotency behavior:
- Same `code` and same `used_by`: `200`
- Same `code` but different `used_by`: `409`
- Missing `Idempotency-Key`: `400` (`IDEMPOTENCY_KEY_REQUIRED`)

curl example:
```bash
curl -X POST "${BASE}/api/v1/admin/redeem-codes/create-and-redeem" \
  -H "x-api-key: ${KEY}" \
  -H "Idempotency-Key: pay-cm1234567890-success" \
  -H "Content-Type: application/json" \
  -d '{
    "code":"s2p_cm1234567890",
    "type":"balance",
    "value":100.00,
    "user_id":123,
    "notes":"sub2apipay order: cm1234567890"
  }'
```

### 2) Query User (optional pre-check)
`GET /api/v1/admin/users/:id`

```bash
curl -s "${BASE}/api/v1/admin/users/123" \
  -H "x-api-key: ${KEY}"
```

### 3) Administrator wallet operations (M6.2)
`POST /api/v1/admin/users/:id/wallet/transactions`

Use case: manual recharge or signed adjustment. `amount` must be a string. A `recharge` is positive with at most two fractional digits; an `adjustment` is non-zero, signed, and supports up to eight fractional digits. `note` is required, and a negative adjustment cannot make the balance negative. Unledgered `set` is no longer supported.

Request body example (negative adjustment):
```json
{
  "amount": "-100.00",
  "operation": "adjustment",
  "note": "support ticket CM-123"
}
```

```bash
curl -X POST "${BASE}/api/v1/admin/users/123/wallet/transactions" \
  -H "Authorization: Bearer ${ADMIN_JWT}" \
  -H "Idempotency-Key: balance-subtract-cm1234567890" \
  -H "Content-Type: application/json" \
  -d '{
    "amount":"-100.00",
    "operation":"adjustment",
    "note":"support ticket CM-123"
  }'
```

When step-up is enabled, writes require an administrator JWT with a recent TOTP grant; an Admin API Key is rejected. The old `/balance` path remains only as a compatibility alias with the same strict contract.

Balance query: `GET /api/v1/admin/users/:id/wallet`. One-time full usage refund: `POST /api/v1/admin/users/:id/wallet/refunds` with `{"original_transaction_id":44,"note":"approved ticket CM-124"}` and an `Idempotency-Key`. A normal user can only query `GET /api/v1/user/wallet`; the server ignores any caller-supplied user ID.

### 4) Purchase / Custom Page URL query forwarding (iframe and new tab)
When Sub2API opens `purchase_subscription_url` or a user-facing custom page iframe URL, it appends:
- `user_id`
- `token`
- `theme` (`light` / `dark`)
- `lang` (for example `zh` / `en`, used to pass the current UI language to the embedded page)
- `ui_mode` (fixed: `embedded`)

Example:
```text
https://pay.example.com/pay?user_id=123&token=<jwt>&theme=light&lang=zh&ui_mode=embedded
```

### 5) Failure handling recommendations
- Persist payment success and recharge success as separate states
- Mark payment as successful immediately after verified callback
- Allow retry for orders with payment success but recharge failure
- Keep the same `code` for retry, and use a new `Idempotency-Key`

### 6) Recommended `doc_url`
- View URL: `https://github.com/Wei-Shaw/sub2api/blob/main/docs/ADMIN_PAYMENT_INTEGRATION_API.md`
- Download URL: `https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/docs/ADMIN_PAYMENT_INTEGRATION_API.md`
