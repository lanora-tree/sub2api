# 数据库设计

## 1. 设计结论

继续使用 PostgreSQL 与现有迁移框架。复用 `users.balance` 作为 CNY 权威余额，新增不可变钱包流水、版本化模型价格、外部充值订单和用户模型权限。扩展 Composite Route 以锁定目标 Group，扩展 usage log 保存价格快照。API Key 和上游凭据需要安全迁移。

V1 新增账务代码统一使用 `shopspring/decimal`。数据库金额使用 `NUMERIC(20,8)`，管理充值输入最多两位小数，Token 费用保留八位小数。

## 2. 现有数据模型审计

| 表或 Schema | 当前能力 | 主要缺口 | 源码路径 |
| --- | --- | --- | --- |
| `users` | 角色、状态、余额、并发、RPM、软删除 | `balance` 的代码类型为 float64，字段语义多处标为 USD | `backend/ent/schema/user.go` |
| `api_keys` | 多 Key、Group、状态、过期、IP ACL、额度 | 完整 Key 明文存储并直接等值查询，DTO 返回完整 Key | `backend/ent/schema/api_key.go`、`backend/internal/repository/api_key_repo.go`、`backend/internal/handler/dto/types.go` |
| `accounts` | 平台、类型、JSONB 凭据、优先级、并发、状态、Cooldown 信息 | `credentials` 当前直接写 JSONB，注释声称加密但 repository 未加密 | `backend/ent/schema/account.go`、`backend/internal/repository/account_repo.go` |
| `groups` | 平台、计费模式、倍率、模型价 JSON、模型路由、fallback | Composite 缺少 only explicit 模式，目标路由不能锁定 Group | `backend/ent/schema/group.go` |
| `composite_model_routes` | 逻辑模型、match、目标平台、实际模型、endpoint、priority、enabled | 无 `target_group_id` | `backend/ent/schema/composite_model_route.go` |
| `usage_logs` | Token、成本分项、模型、账号、Group、状态和延迟 | 无价格规则 ID、价格版本、完整单价快照和钱包流水引用 | `backend/ent/schema/usage_log.go` |
| `usage_billing_dedup` | `(request_id, api_key_id)` 唯一去重 | 只证明扣款认领，不形成用户可审计流水 | `backend/migrations/071_add_usage_billing_dedup.sql` |
| `idempotency_records` | 通用 scope 加 Key 幂等协调 | 可复用 API 协调，不能替代业务唯一订单 | `backend/migrations/057_add_idempotency_records.sql` |
| `redeem_codes` | 余额和订阅兑换历史 | 记录模型不适合完整钱包流水，代码类型仍为 float64 | `backend/ent/schema/redeem_code.go` |
| `audit_logs` | 管理操作审计和请求体脱敏 | 适合记录操作，不能承担余额总账 | `backend/migrations/180_audit_logs.sql` |
| `channel_model_pricing` | 动态渠道价、时段、倍率 | 面向渠道和上游成本，缺少不可变版本与用户卖价快照 | `backend/migrations/081_create_channels.sql` 及后续 channel migrations |
| `security_secrets` | JWT 等系统 Secret 持久化 | value 为明文 text，不适合保存外部 HMAC 或上游密钥 | `backend/ent/schema/security_secret.go` |

## 3. 复用策略

### 3.1 继续复用

1. `users`：用户身份、状态、并发和当前余额。
2. `groups`、`accounts`、`account_groups`：Provider 账号池与 Scheduler。
3. `composite_model_routes`：V1 用户模型路由注册表。
4. `usage_logs`：请求级元数据和账单记录。
5. `usage_billing_dedup`：usage 扣款幂等认领。
6. `idempotency_records`：管理员写接口和 Internal API 的执行协调。
7. `audit_logs`：管理员、Internal API 和安全事件审计。
8. `settings`：V1 功能开关与非 Secret 配置。

### 3.2 不新增 `wallets`

`users.balance` 已被 API Key 认证快照、余额预检、扣款、Dashboard 和 Admin API 使用。新增 `wallets.balance` 会形成两份当前余额，增加双写和 upstream 合并成本。

V1 钱包由以下两部分组成：

* `users.balance`：可快速读取的当前余额。
* `wallet_transactions`：不可变、可追溯、可对账的总账。

两者必须在同一事务中更新。

## 4. 新增表

### 4.1 `wallet_transactions`

| 字段 | 类型 | 约束与说明 |
| --- | --- | --- |
| `id` | BIGSERIAL | Primary Key |
| `user_id` | BIGINT | FK `users(id)`，禁止级联删除账本 |
| `type` | VARCHAR(24) | `recharge`、`usage`、`refund`、`adjustment` |
| `amount` | NUMERIC(20,8) | 带符号，充值与退款通常为正，usage 通常为负 |
| `currency` | CHAR(3) | V1 固定 `CNY` |
| `balance_before` | NUMERIC(20,8) | 事务内快照 |
| `balance_after` | NUMERIC(20,8) | 必须等于 before 加 amount |
| `reference_type` | VARCHAR(32) | `usage_log`、`external_order`、`admin_operation` 等 |
| `reference_id` | VARCHAR(128) | 稳定业务引用 |
| `idempotency_key` | VARCHAR(160) | 业务幂等键 |
| `request_fingerprint` | VARCHAR(64) | 规范化请求的 SHA-256；相同幂等键不同指纹拒绝 |
| `operator_id` | BIGINT NULL | Admin User ID，系统结算为空 |
| `source` | VARCHAR(64) | `gateway`、`admin` 或外部站点标识 |
| `note` | VARCHAR(500) | 禁止写 Secret |
| `metadata` | JSONB | 脱敏扩展信息 |
| `created_at` | TIMESTAMPTZ | 只增不改 |

约束与索引：

```sql
CHECK (currency = 'CNY')
CHECK (balance_after = balance_before + amount)
CHECK (type IN ('recharge','usage','refund','adjustment'))
CHECK (recharge/refund > 0, usage <= 0)
CHECK (required text fields are not blank)
CHECK (request_fingerprint is 64 lowercase hex characters)
UNIQUE (idempotency_key)
INDEX (user_id, created_at DESC, id DESC)
INDEX (reference_type, reference_id)
```

删除用户采用软删除。钱包流水不随用户删除。

M6.1 已由 `240_cny_wallet_transactions.sql` 实现本表、数据库不可变 trigger、CNY opening adjustment 和上述约束。当前本地库只有 M0 无价值测试余额，产品负责人确认按数值 1:1 开账；真实 USD 存量不得复用该捷径。应用层使用 `shopspring/decimal`，通过用户行锁和事务级幂等 advisory lock，将 `users.balance` 更新与流水插入置于同一 PostgreSQL 事务。Admin 与 Gateway 旧写入口的切换属于后续 M6 子任务。

### 4.2 `model_pricing_rules`

| 字段 | 类型 | 约束与说明 |
| --- | --- | --- |
| `id` | BIGSERIAL | Primary Key，稳定 `pricing_rule_id` |
| `provider` | VARCHAR(32) | `codex_subscription`、`openai_official`、`deepseek_official` |
| `target_group_id` | BIGINT | FK `groups(id)` |
| `logical_model` | VARCHAR(160) | 客户端看到的精确模型名 |
| `protocol` | VARCHAR(32) | `responses`、`chat_completions` 或 `any` |
| `billing_unit` | VARCHAR(32) | `per_1m_tokens`、`per_request` 等 |
| `currency` | CHAR(3) | V1 固定 `CNY` |
| `enabled` | BOOLEAN | 规则总开关 |
| `latest_published_version_id` | BIGINT NULL | 仅供 Admin 展示最近发布版本；运行时仍按请求开始时间查询有效区间 |
| `created_by` | BIGINT | Admin User ID |
| `created_at` | TIMESTAMPTZ | 创建时间 |
| `updated_at` | TIMESTAMPTZ | 配置更新时间 |

唯一约束：

```sql
UNIQUE (target_group_id, logical_model, protocol)
CHECK (currency = 'CNY')
```

### 4.3 `model_pricing_versions`

| 字段 | 类型 | 约束与说明 |
| --- | --- | --- |
| `id` | BIGSERIAL | Primary Key，稳定 `pricing_version_id` |
| `rule_id` | BIGINT | FK `model_pricing_rules(id)` |
| `version` | INTEGER | 从 1 递增 |
| `input_price` | NUMERIC(24,12) NULL | 每 billing unit 输入价 |
| `output_price` | NUMERIC(24,12) NULL | 输出价 |
| `cache_input_price` | NUMERIC(24,12) NULL | Cache read 价 |
| `cache_write_price` | NUMERIC(24,12) NULL | Cache write 价 |
| `usage_price` | NUMERIC(24,12) NULL | Provider 特定 usage 单价 |
| `extra_prices` | JSONB | 音频、搜索等后续扩展，V1 通常为空 |
| `effective_from` | TIMESTAMPTZ | 生效时间 |
| `effective_to` | TIMESTAMPTZ NULL | 发布新版本时关闭旧区间 |
| `created_by` | BIGINT | Admin User ID |
| `note` | VARCHAR(500) | 调价说明 |
| `created_at` | TIMESTAMPTZ | 创建时间 |

约束：

```sql
UNIQUE (rule_id, version)
CHECK (effective_to IS NULL OR effective_to > effective_from)
CHECK (input_price IS NULL OR input_price >= 0)
CHECK (output_price IS NULL OR output_price >= 0)
EXCLUDE USING gist (
  rule_id WITH =,
  tstzrange(effective_from, COALESCE(effective_to, 'infinity'::timestamptz), '[)') WITH &&
)
```

排他约束需要 `btree_gist` 扩展，并在 migration 集成测试中验证。版本行发布后不可更新单价或有效区间，纠错通过创建新版本完成。数据库 trigger 和运行账号权限共同阻止对已发布版本的更新与删除，不能只依赖 repository 规则。

### 4.4 `user_model_permissions`

| 字段 | 类型 | 约束与说明 |
| --- | --- | --- |
| `user_id` | BIGINT | FK `users(id)` |
| `route_id` | BIGINT | FK `composite_model_routes(id)` |
| `enabled` | BOOLEAN | 显式允许或暂停 |
| `created_by` | BIGINT | Admin User ID |
| `created_at` | TIMESTAMPTZ | 创建时间 |
| `updated_at` | TIMESTAMPTZ | 更新时间 |

Primary Key 为 `(user_id, route_id)`。增加 `(route_id, enabled)` 索引。新用户默认没有记录并被拒绝。已被权限引用的 Route，其 public model、Provider、target Group、upstream model 和 endpoint 视为不可变；改变这些授权语义时必须创建新 Route、重新授权并停用旧 Route，避免原权限静默指向另一成本池或模型。

### 4.5 `external_recharge_orders`

| 字段 | 类型 | 约束与说明 |
| --- | --- | --- |
| `id` | BIGSERIAL | Primary Key |
| `source` | VARCHAR(64) | 从已认证 Key ID 的服务端配置推导，禁止相信请求体 |
| `external_order_id` | VARCHAR(128) | 外部唯一订单号 |
| `user_id` | BIGINT | FK `users(id)` |
| `amount` | NUMERIC(20,8) | 必须大于零，管理输入最多两位小数 |
| `currency` | CHAR(3) | 固定 CNY |
| `request_fingerprint` | CHAR(64) | 规范业务字段 SHA256，算法见 `api_spec.md` |
| `nonce_hash` | CHAR(64) | 防重放审计，不存原始 Secret |
| `key_id` | VARCHAR(64) | HMAC 密钥版本标识 |
| `status` | VARCHAR(24) | `processing`、`completed`、`failed` |
| `wallet_transaction_id` | BIGINT NULL | 成功流水 FK |
| `failure_code` | VARCHAR(64) NULL | 稳定错误码 |
| `requested_at` | TIMESTAMPTZ | 外部时间戳 |
| `created_at` | TIMESTAMPTZ | 接收时间 |
| `completed_at` | TIMESTAMPTZ NULL | 完成时间 |

约束与索引：

```sql
UNIQUE (source, external_order_id)
UNIQUE (wallet_transaction_id)
CHECK (amount > 0)
CHECK (currency = 'CNY')
INDEX (user_id, created_at DESC)
INDEX (status, created_at)
```

Nonce 的五到十分钟快速窗口放在 Redis，Redis 故障时接口 fail closed。业务订单的长期幂等由 PostgreSQL 唯一约束保证。

## 5. 修改现有表

### 5.1 `groups`

新增：

```sql
composite_explicit_routes_only BOOLEAN NOT NULL DEFAULT FALSE
```

只有 V1 Gateway Composite Group 设为 true。其他 upstream Composite Group 保持原行为。

### 5.2 `composite_model_routes`

新增：

```sql
target_group_id BIGINT NULL REFERENCES groups(id) ON DELETE RESTRICT
```

为查询新增 `(group_id, enabled, public_model, endpoint)` 索引，并为 V1 exact route 增加部分唯一索引，使同一 access Group、逻辑模型和 endpoint 最多只有一条启用的 exact route。V1 Route 要求 `target_group_id` 非空；历史 upstream Route 允许为空以保持兼容。应用层和数据库 trigger 校验 source Group 为 Composite、target Group 为具体平台且平台一致。

示意约束：

```sql
CREATE UNIQUE INDEX uq_v1_enabled_exact_route
ON composite_model_routes (group_id, public_model, endpoint)
WHERE enabled = TRUE AND match_type = 'exact' AND target_group_id IS NOT NULL;
```

已存在 `user_model_permissions` 的 Route 禁止原地修改授权语义字段。启停、priority 和备注可以按审计策略更新；改变目标或模型必须新建 Route 并重新授权。

### 5.3 `api_keys`

目标字段：

```sql
key_hash CHAR(64)
key_prefix VARCHAR(16)
key_last4 CHAR(4)
key_hash_version SMALLINT NOT NULL DEFAULT 1
key VARCHAR(128) NULL
```

`key_hash` 为 `HMAC_SHA256(server_pepper, full_key)` 的十六进制摘要。Server pepper 独立保存在生产 Secret Manager 或 root only 环境文件，不进入数据库和备份。

迁移步骤：

1. 添加 nullable 新字段和唯一索引。
2. 新 Key 生成至少 256 bit 随机值，只写摘要与展示片段。
3. 旧 Key 登录时先走摘要，必要时走受控 legacy lookup。
4. 后台批量计算旧明文 Key 摘要并清空 `key`。
5. 轮换所有曾出现在数据库备份中的旧 Key。
6. 移除 DTO 的 `key` 字段，创建响应单独返回一次 `secret`。
7. 完成后添加 `key_hash NOT NULL`。

缓存 Key 也使用摘要或 Key ID，禁止把完整 Key 写入 Redis value 和日志。

Pepper 轮换采用 active 与 previous 两版本的有界双读。认证时分别使用对应 pepper 计算候选摘要；previous 版本命中后立即把该行渐进改写为 active 摘要。由于数据库中没有 Key 原文，未再次认证的旧 Key 无法离线重算，必须在轮换截止时撤销并通知用户重新创建。旧 pepper 不得无限期保留。

### 5.4 `accounts`

目标字段：

```sql
credentials_encrypted TEXT
credentials_key_version SMALLINT
credentials_aad_id UUID NOT NULL
credentials_meta JSONB NOT NULL DEFAULT '{}'
credentials JSONB NULL
```

使用独立 `ACCOUNT_CREDENTIALS_ENCRYPTION_KEY` 的 AES 256 GCM。每行随机 Nonce，密文绑定 `credentials_aad_id`、platform 与 key version 作为 AAD。独立 UUID 由应用在插入前生成，避免依赖数据库扩展，也避免新账号尚无数据库 ID 时出现先明文落库或二次写入窗口。`credentials_meta` 只放 base URL、模型能力等非 Secret。

迁移先双读，后加密回填，验证解密成功，再清空旧 `credentials` 中的 Token、Key、Cookie 和密码。现有 `backend/internal/repository/aes_encryptor.go` 可参考算法，但不能复用 TOTP Encryption Key。

### 5.5 `usage_logs`

新增：

```sql
provider VARCHAR(32)
access_group_id BIGINT
target_group_id BIGINT
pricing_rule_id BIGINT
pricing_version_id BIGINT
pricing_snapshot JSONB
billing_currency CHAR(3)
wallet_transaction_id BIGINT
settlement_status VARCHAR(24) NOT NULL DEFAULT 'settled'
```

`pricing_snapshot` 最少保存：

```json
{
  "billing_unit": "per_1m_tokens",
  "input_price": "1.500000000000",
  "output_price": "6.000000000000",
  "cache_input_price": "0.150000000000",
  "cache_write_price": null,
  "usage_price": null,
  "currency": "CNY",
  "rounding": "HALF_UP_8"
}
```

最终费用继续使用现有 `actual_cost` 列或新增 `billed_amount`。若复用 `actual_cost`，必须增加 `billing_currency` 并将 Go 类型改为 Decimal，避免把 CNY 与现有 USD 语义混淆。

建议索引：

```sql
UNIQUE (wallet_transaction_id)
INDEX (user_id, created_at DESC, id DESC)
INDEX (provider, created_at DESC)
INDEX (target_group_id, model, created_at DESC)
INDEX (pricing_version_id)
```

### 5.6 `usage_billing_dedup`

现有唯一键继续保留，并新增足以判断重放与返回原结果的字段：

```sql
request_fingerprint CHAR(64)
usage_log_id BIGINT NULL REFERENCES usage_logs(id) ON DELETE RESTRICT
wallet_transaction_id BIGINT NULL REFERENCES wallet_transactions(id) ON DELETE RESTRICT
billed_amount NUMERIC(20,8) NULL
billing_currency CHAR(3) NULL
settled_at TIMESTAMPTZ NULL
```

迁移时先允许新字段为空；旧记录不能伪造 fingerprint，只标记为 legacy。切换新结算路径后，新记录必须在同一事务中写入 fingerprint 和最终结果引用，再增加针对新版本记录的约束。唯一键冲突后读取已提交记录：fingerprint 相同则返回原 `usage_log_id`、流水和金额，fingerprint 不同则返回高危幂等冲突并告警。

`wallet_transactions` 的不可变性由数据库运行账号权限和 trigger 双重保证：应用账号不得执行 UPDATE 或 DELETE；经审批的维护账号也只能写补偿流水，不能改写历史金额。

## 6. 金额与舍入规则

1. 所有单位价格在 Decimal 中计算。
2. Token 价按每百万 Token 存储，公式先乘 Token 再除以 1,000,000。
3. 各费用分量保留至少 12 位中间精度。
4. 最终请求费用使用 `ROUND_HALF_UP` 保留 8 位小数。
5. Dashboard 展示可保留 2 到 6 位，账本值不因展示而变化。
6. 管理充值和退款请求使用字符串，例如 `"100.00"`。
7. 禁止把 Decimal 先转成 float 再写 NUMERIC。

## 7. 并发与事务

### 7.1 Usage 结算

扩展 `backend/internal/repository/usage_billing_repo.go` 的 `Apply` 事务。事务顺序：

1. 插入 `usage_billing_dedup`，冲突时核对 fingerprint 并返回原结算结果。
2. 查询或锁定用户余额。
3. 插入 usage log。
4. 插入唯一钱包流水，幂等键为 `usage:{api_key_id}:{request_id}`。
5. 更新余额和现有 quota 统计。
6. 回填 usage log 的 wallet transaction ID。
7. 提交后失效认证与账单缓存。

余额更新允许结果小于零，以满足已开始 Streaming 的真实结算。新请求在 middleware 中看到 `balance <= 0` 时拒绝。

如果 PostgreSQL 在最终 usage 到达后持续不可用，Gateway 必须先把不含 Prompt、Answer 和 Secret 的结算 envelope 写入持久化本地恢复日志并执行 checksum 与 fsync，再转发 Streaming 终止事件或结束非流式响应。恢复日志位于独立持久卷，以幂等键命名并加密；写入成功后立即进入 Billing degraded，拒绝所有新模型请求。启动时只要存在未确认记录或 degraded sentinel 就保持降级，由恢复任务按 dedup fingerprint 重放。全部记录结算成功、归档并完成账本对账后，才允许经 Admin step up 人工解除。

### 7.2 充值与调账

事务使用 `SELECT ... FOR UPDATE` 锁定用户行，计算 before 与 after，更新 `users.balance`，插入流水。管理员扣减不得使余额小于零。Usage 扣款可以产生负余额。

### 7.3 退款

退款必须引用原 usage 流水。为 `refund:{original_transaction_id}` 建立唯一幂等键。部分退款如未来需要支持，应加入退款序号和累计上限，V1 默认仅支持一次全额人工退款。

M6.2 使用 forward-only migration `241_wallet_refund_link.sql` 增加 `reverses_transaction_id` 自引用外键，并对 refund 行建立部分唯一索引。这样 HTTP `Idempotency-Key` 负责同一请求重放，`reverses_transaction_id` 唯一性独立保证即使请求键不同也不能重复退款；Repository 在同一事务内校验原流水属于同一用户、类型为 usage 且金额为负，再从原流水派生完整退款金额。

## 8. 对账

每日执行：

```sql
SELECT u.id,
       u.balance,
       COALESCE(SUM(w.amount), 0) AS ledger_balance
FROM users u
LEFT JOIN wallet_transactions w ON w.user_id = u.id
GROUP BY u.id, u.balance
HAVING u.balance <> COALESCE(SUM(w.amount), 0);
```

如上线前存在初始余额，为每个用户创建一条 `adjustment` 开账流水，确保流水总和可重建余额。

还需检查：

1. settled usage 必须有一条 usage 流水。
2. usage 流水金额必须等于 usage 最终费用的负值。
3. completed 外部订单必须引用一条 recharge 流水。
4. 当前价格版本必须属于同一规则。
5. 价格有效区间不可重叠。

任一差异触发 Billing degraded 告警，暂停新请求并人工处理。

## 9. 迁移与回滚策略

1. 所有迁移先在完整匿名化备份上演练。
2. 使用 expand、backfill、dual read、cutover、contract 五步迁移。
3. 大表新增列先 nullable，回填分批执行，再添加约束。
4. API Key 和账号凭据迁移完成前保留旧读路径，写路径只写安全格式。
5. CNY 语义切换只允许在空库执行，或按人工确认汇率生成逐用户开账流水。
6. Sub2API 迁移为 forward only，生产回滚依赖升级前 PostgreSQL 备份和旧镜像。
7. 每次发布记录 migration filename、checksum、执行耗时和行数。

## 10. 待验证事项

1. 当前本地数据库已确认为只有 M0 无价值测试余额并采用 1:1 CNY opening；任何真实 USD 目标库仍需单独决定迁移方式。
2. Ent 对 `decimal.Decimal` 的映射 prototype 已在 M6.1 通过生成、编译和真实 PostgreSQL integration 验收。
3. Composite target Group 在现有 Billing 和 Ops 投影中的最小改动点，需要 M6 用调用图和 migration prototype 确认。
4. usage log 大表索引体积和保留周期，需要 M0 基线数据估算。
5. 账号凭据整体加密后对 Scheduler 热路径的解密缓存方案，需要性能测试。
