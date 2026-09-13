# 测试策略

## 1. 目标

测试需要证明四件事：协议兼容、Provider 隔离、账务准确、用户和 Secret 安全。每个 Milestone 都有独立验收，M12 执行完整安全、并发和异常回归，M13 完成部署与恢复验收。

## 2. 当前测试体系

| 层 | 当前事实 | 入口 |
| --- | --- | --- |
| Backend | Go test，存在大量 unit 与 integration tests | `make test-backend`、`backend/Makefile` |
| Frontend | ESLint、vue-tsc、Vitest、Vite build | `make test-frontend`、`make build-frontend` |
| Critical frontend | 根 Makefile 固定关键 Vitest 清单 | `make test-frontend-critical` |
| Database | migrations regression 和 testcontainers | `backend/migrations/*_test.go`、repository integration tests |
| Deployment | Compose、安全与 runtime shell tests | `deploy/tests` |

当前审计记录约有 1239 个 Backend `_test.go` 文件、251 个 Frontend spec 或 test 文件、6 个 deploy 测试相关文件。数量只反映静态扫描结果，M0 必须在固定 commit 上重新计数并记录命令。

## 3. 基线命令

```bash
git status --short
make test-backend
make test-frontend
make build
git diff --check
```

针对迁移和 Provider 的具体命令应从对应 Makefile 与测试文件确认。测试环境不得使用生产 Secret。

当前分析运行器缺少 Go 与 Docker，且依赖下载受网络策略限制。只确认 Node 与 pnpm 可执行，未完成测试。该限制记录在 `progress.md`，不视为测试通过。

## 4. 测试环境

### 4.1 Unit

使用纯 Go、SQLite 或 mock，不访问真实 Provider。冻结时间、随机数和 exchange rate 等非确定输入。

### 4.2 Integration

使用独立 PostgreSQL 与 Redis 容器。每个测试库从全部 migrations 构建，测试结束后删除专用测试资源。不得连接开发或生产数据库。

### 4.3 Staging

使用与生产相同 Compose、Nginx 和镜像。Provider 使用专用低额度测试账号。用户、Key 和订单号带 `staging` 标识。

### 4.4 客户端矩阵

每次记录：操作系统、客户端版本、Gateway commit、镜像 digest、Provider、模型、协议、是否 Streaming、结果与 request ID。

## 5. Unit Tests

### 5.1 路由

1. exact route 命中正确 target Group。
2. `explicit_routes_only=true` 时未知模型 fail closed。
3. disabled route 被拒绝。
4. target Group disabled 被拒绝。
5. target platform 与 target Group platform 不一致时保存失败。
6. target Group 为 Composite 时保存失败。
7. Prefix route 在 V1 Group 被拒绝。
8. Group fallback 为空，无法跨组。
9. Codex 和 OpenAI 同属 `openai` 时仍锁定各自账号池。
10. route 变更后缓存及时失效。
11. 同一 V1 Group、public model 和 endpoint 不能同时存在两条启用的 exact route。
12. 已被用户权限引用的 route 修改授权语义字段时被拒绝，必须新建 route 并重新授权。

### 5.2 权限

1. 新 User 默认无模型权限。
2. 授权单个 route 后只开放该模型。
3. User A 无法读取或修改 User B 权限。
4. disabled User、Key、route、Group 的优先级正确。
5. `/v1/models` 返回权限、启用 route、启用 target Group 和有效价格的交集，不因账号短暂 Cooldown 抖动；真实请求在无可调度账号时返回 503。
6. 两个管理员基于同一 ETag 并发完整替换时只能一个成功，另一个返回 409，不发生 lost update。
7. detector/account ownership 隐式路由、权限数据库错误和无权限行都在上游并发槽之前 fail closed。
8. OpenAI Images 省略 model 时按服务端默认 `gpt-image-2` 执行同一 Route 权限检查，不能利用默认值绕过授权。

### 5.3 API Key

1. 生成熵和格式。
2. 数据库只保存 HMAC hash、prefix、last4、version。
3. Create response 返回一次 Secret。
4. List 与 Get 不返回 Secret。
5. HMAC pepper version 双读。
6. 错误 Key 使用 constant time 路径。
7. 单 Key 禁用不影响其他 Key。
8. 缓存失效与 TTL。
9. legacy 明文迁移后无法从数据库恢复完整 Key。
10. previous pepper 命中后渐进更新为 active 版本。
11. 轮换截止后未迁移的旧 Key 被撤销，移除 previous pepper 后仍可正常认证 active Key。

### 5.4 Decimal Billing

1. input、output、cache read、cache write 公式。
2. 0 Token、1 Token、极大 Token。
3. 12 位单价与 8 位最终舍入。
4. 价格版本有效区间边界。
5. 请求开始后调价仍使用冻结版本。
6. 缺价格与多价格匹配 fail closed。
7. V1 不进入硬编码 fallback。
8. CNY JSON 为字符串。
9. Provider 特殊 usage 字段不重复计费。
10. Subscription 内部价不自动推导真实成本。
11. 同一 pricing rule 的有效区间重叠被数据库排他约束拒绝。
12. `latest_published_version_id` 不参与运行时选价，未来版本不会提前生效。

### 5.5 Wallet

1. recharge、usage、refund、adjustment 的符号与 before、after。
2. 同 Idempotency Key 相同 body 返回原结果。
3. 同 Key 不同 body 返回 conflict。
4. usage request ID 并发只扣一次。
5. refund 并发只成功一次。
6. Admin 负调账不能制造负余额。
7. Streaming usage 可以使余额为负。
8. 负余额后的新请求被拒绝。
9. users balance 与流水累计对账。

### 5.6 HMAC Recharge

1. 正确签名。
2. body 任一 byte 改变导致签名失败。
3. method、path、timestamp 或 nonce 改变失败。
4. Timestamp 过期和未来超窗。
5. Nonce 重放。
6. Key ID 未知、停用和轮换窗口。
7. source 与 Key ID 绑定。
8. 同 external order 幂等重放。
9. 同订单不同 User 或金额冲突。
10. Redis 不可用 fail closed。
11. 请求体尝试提交 source 作为未知字段被拒绝，持久化 source 只来自 Key ID 配置。
12. 签名失败不占用 nonce；签名成功后同 nonce 的第二次请求被拒绝。
13. JSON 字段顺序或空白变化只影响 HMAC raw body，规范业务字段相同仍命中同一订单 fingerprint。

## 6. Database Integration Tests

1. 全新 PostgreSQL 执行全部 upstream 与 custom migrations。
2. 从上一 release schema 执行增量 migrations。
3. 重复启动不重复应用 migration。
4. migration checksum mismatch 触发启动失败。
5. 新表、字段、FK、CHECK、unique 和 partial indexes 存在。
6. API Key 明文回填和清除脚本对账。
7. Account credential 加密回填可全部解密。
8. pricing version 不可变。
9. wallet transaction 禁止 update 与 delete，维护操作需要专门受控流程。
10. User 软删除后账本仍保留。
11. target Group 被 route 使用时禁止删除。
12. transaction 中任一步失败时余额、usage 和流水全部回滚。
13. `usage_billing_dedup` 保存 fingerprint 和原结算引用，相同重放返回原结果，不同 fingerprint 返回冲突。
14. 钱包流水对应用运行账号不可 UPDATE 或 DELETE。

## 7. Gateway Integration Tests

使用 mock upstream 检查完整请求流：

1. API Key auth 到 route、permission、price、scheduler、provider、usage 和 wallet。
2. Responses 普通与 SSE。
3. Chat Completions 普通与 SSE。
4. Tool Calls 多轮。
5. Reasoning content。
6. Cache usage。
7. upstream model rewrite。
8. request ID 传播。
9. 客户端断开。
10. 上游在首包前和首包后失败。
11. usage 缺失。
12. Billing degraded 后拒绝新请求。

## 8. Provider Tests

### 8.1 Codex Subscription

| 用例 | 结果要求 |
| --- | --- |
| OAuth 登录与刷新 | Token 更新成功，日志无 Token |
| Responses | 普通响应结构兼容 |
| Streaming | 首包、增量、结束与 usage 正常 |
| Reasoning | effort 和返回字段正常 |
| Tool Calls | 多轮 continuation 正常 |
| Sticky | 同 session 留在同一有效账号 |
| 账号失效 | 只在 Codex Group 内切换 |
| 多账号并发 | 不超过账号 concurrency |
| 计费 | usage、快照、流水一致 |

### 8.2 OpenAI Official

测试 API Key、Responses、Chat Completions、Streaming、usage、模型 mapping、401、429、5xx、Timeout、账号禁用和 Key 轮换。确认不会选择 OAuth Subscription Account。

### 8.3 DeepSeek

测试当前官方 Models API、V4 Chat Completions、Streaming、usage、reasoning content、Tool Calls、thinking、余额查询、401、402、429、5xx 与 Timeout。

如保留 `deepseek-chat` 和 `deepseek-reasoner` 逻辑别名，额外确认实际模型 rewrite 和 thinking profile。旧模型名不得未经转换直接发送到上游。

## 9. Codex 客户端验收

使用当时最新稳定 Codex CLI，记录官方认证和配置文档版本。测试：

1. Base URL 与 Gateway API Key。
2. `/v1/models`。
3. `/v1/responses`。
4. Streaming 首包和长任务。
5. Reasoning effort。
6. Function 与 Tool Calls。
7. 多轮会话与 previous response。
8. Sticky Account。
9. 上游账号 401 后恢复。
10. 429 和 Timeout 的客户端行为。
11. 客户端取消请求。
12. 余额不足错误是否可理解。

只实现 Chat Completions 不构成 Codex 兼容验收。

## 10. Cursor 客户端验收

使用当时最新稳定 Cursor。不同版本可能使用不同 endpoint 和模型探测，需要抓取脱敏 Gateway access log确认。

测试：

1. Base URL。
2. API Key。
3. `/v1/models`。
4. `/v1/chat/completions`。
5. Streaming。
6. 长上下文。
7. Tool Calls 和 Agent mode。
8. Reasoning 模型。
9. 模型 mapping。
10. 401、403、429、5xx 和 Timeout。
11. 客户端取消与重试是否引发重复结算。

任何 Cursor 私有 endpoint 或非标准行为标记为待验证，禁止从经验猜测。

## 11. OpenAI SDK 验收

至少使用 Python 和 JavaScript 官方 SDK：

1. Models list。
2. Chat Completions 普通与 Streaming。
3. Responses 普通与 Streaming。
4. 自定义 Base URL。
5. Tool Calls。
6. Timeout 与 retry。
7. 错误 envelope。

保存 SDK version，避免浮动依赖。

## 12. Streaming Tests

1. Nginx 不缓冲，首个 event 在上游发送后及时到达。
2. SSE 不被 gzip 聚合。
3. 30 分钟长流保持连接。
4. `[DONE]` 或 Responses 完成事件保持协议，并且只在结算提交或恢复记录 fsync 后发送。
5. usage 在结束事件正确解析。
6. 客户端断开能通知 upstream 并释放并发槽位；结算协程在有界宽限期内排空最终 usage，缺失时写恢复记录。
7. 首包后 upstream 5xx 或断流有稳定错误和结算状态。
8. 余额在流中变负时流继续完成。
9. 流结束后下一请求被余额预检拒绝。
10. Nginx reload 不应主动中断现有长流，具体行为需 staging 验证。

## 13. Sticky 与 Session Tests

1. 相同 API Key、target Group、模型和 session 命中同账号。
2. 不同 User 或 API Key 不共享绑定。
3. 不同 target Group 不共享绑定。
4. previous response 优先级符合当前 Scheduler。
5. 绑定账号 disabled、cooldown、exhausted 后在同 Group 重选。
6. Redis restart 后安全重建。
7. TTL 到期后重新选择。
8. Nginx 保留实际客户端 session header。
9. 同 session 并发不超过 sticky waiting 限制。

## 14. Concurrency Tests

使用 `go test -race` 覆盖纯 Go 并发，使用真实 PostgreSQL 与 Redis 覆盖分布式竞争。

| 场景 | 断言 |
| --- | --- |
| 同一 User 100 个请求 | User concurrency 不超限，所有槽位最终释放 |
| 不同 User 并发 | 数据和 slot 隔离 |
| 同一上游账号 | Account concurrency 不超限 |
| 多 Codex 账号 | 按 Scheduler 分配，无跨 Group |
| usage 同 ID 并发结算 | 一条 usage 和一条流水 |
| 充值同订单并发 50 次 | 余额只增加一次 |
| 退款并发 20 次 | 只生成一条 refund |
| 充值与 usage 同时发生 | 无 lost update，before 和 after 连续 |
| Admin 调账与 usage | 余额与流水对账一致 |

## 15. Failure Tests

| 故障 | 预期 |
| --- | --- |
| Provider Timeout | 同 Group 受控重试或返回 504，释放 slot |
| Provider 429 | 账号 Cooldown，遵守 reset，组内重选 |
| Provider 401 | 账号退出调度，OAuth 可刷新，API Key 要求轮换 |
| Provider 5xx | 同 Group 有界重试，无跨 Provider |
| 额度耗尽 | 映射 Exhausted，组健康下降 |
| Redis restart | 缓存可重建，关键鉴权和防重放 fail closed |
| Backend restart | 无重复结算；存在恢复日志或 degraded sentinel 时启动后继续拒绝新请求 |
| PostgreSQL 短暂异常 | 有限重试，事务无半成功 |
| PostgreSQL 持续异常 | 终止事件前持久化并 fsync 结算 envelope，Billing degraded，拒绝新请求 |
| 恢复日志重复或损坏 | 重复记录幂等，checksum 损坏保持降级并要求人工处理 |
| 客户端断开 | upstream cancel、slot release；有真实 usage 则结算，缺失则 needs review 并保持可追溯 |
| 余额不足 | 新请求 403，已开始流不被切断 |

## 16. Security Tests

1. IDOR 和跨 User 查询。
2. 普通 User 调用 Admin route。
3. disabled User 使用旧 JWT 和 API Key。
4. SQL injection、JSON oversized body、header abuse。
5. SSRF 与 upstream URL allowlist。
6. HMAC timing、replay、source spoof 与 IP header spoof。
7. Audit redaction。
8. 日志与数据库 Secret scan。
9. 备份只含密文凭据。
10. Nginx trusted proxy、TLS、端口和 rate limit。
11. dependency、container 和 SBOM 扫描。
12. route 配置误操作不会跨 Provider。

## 17. 前端测试

1. Public settings 初始注入与 feature flag 无闪烁。
2. 注册、支付、promo、affiliate、redeem、Model Plaza 等菜单隐藏。
3. 直接访问关闭 route 被重定向或显示 404。
4. User 页面只展示自己的字段。
5. Key Secret 一次展示并禁止返回页面再读取。
6. CNY Decimal string 正确格式化。
7. Admin 调价显示版本历史。
8. Wallet write 的 idempotency 和错误提示。
9. 响应式布局和主流浏览器。

执行：

```bash
pnpm --dir frontend run lint:check
pnpm --dir frontend run typecheck
pnpm --dir frontend run test:run
pnpm --dir frontend run build
```

## 18. 备份与部署验收

1. Compose config 校验。
2. 容器 healthcheck。
3. 只开放预期公网端口。
4. PostgreSQL、Redis、Backend 分别重启。
5. 每日备份成功、checksum 有效、加密上传成功。
6. 新环境恢复，账本对账为零差异。
7. Upgrade 与 application rollback。
8. 数据库恢复回滚和备份时点后订单补录演练。

## 19. 完成标准

一个 Milestone 只有同时满足以下条件才能 completed：

1. 直接相关 unit 与 integration tests 通过。
2. Backend 全量 test 通过，或已批准的 unrelated known failure 有证据。
3. Frontend lint、typecheck、critical tests 和 build 通过。
4. `git diff --check` 通过。
5. 文档、progress、findings 和 changelog 已更新。
6. 没有新增高危安全问题。
7. 手工验收有版本、时间、request ID 和结果记录。

## 20. 测试报告模板

```text
日期：
Commit：
Milestone：
环境：
数据库版本：
Redis 版本：
客户端版本：
执行命令：
通过：
失败：
跳过及原因：
关键 request ID：
账本对账：
新风险：
结论：
```
