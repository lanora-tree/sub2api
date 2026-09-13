# 项目进度记录

## 1. 当前状态

* 项目代号：中转站
* 上游项目：Sub2API
* 记录日期：2026-09-13 UTC+8
* 当前阶段：M6 用户、密钥、路由与钱包基础
* 阶段状态：`in_progress`
* 当前任务：M6.3 用户模型权限已完成；下一项 M6.4 API Key HMAC 摘要

M0、M1 与 M2 已完成。M6.1、M6.2 与 M6.3 已依次完成 CNY 钱包基础、管理员钱包操作和 Composite 用户模型权限，并通过真实 PostgreSQL、完整前后端回归和本地升级验收。M6 继续 `in_progress`，下一项为 API Key HMAC 摘要。未接入真实 Provider 凭据或真实收费数据。

## 2. 里程碑看板

| Milestone | 内容 | 状态 | 进度说明 |
| --- | --- | --- | --- |
| M0 | Fork 与原始系统运行 | `completed` | Fork、镜像、全栈、登录、Gateway、Streaming 与用量基线已验收 |
| M1 | 源码分析与设计文档 | `completed` | 源码审计、方案设计和交叉检查已完成 |
| M2 | 关闭公开与商业功能 | `completed` | 三层关闭守卫、全量回归、迁移和本地运行验收通过 |
| M3 | Codex Subscription 验证 | `pending` | 等待 M2、M6、M7、合规确认和合法测试账号 |
| M4 | OpenAI Official API | `pending` | 等待 M2、M6、M7 的基础能力 |
| M5 | DeepSeek Official API | `pending` | 等待 M2、M6、M7、模型名和价格人工确认 |
| M6 | 用户、密钥、路由与钱包基础 | `in_progress` | M6.1-M6.3 已验收；下一项 M6.4 API Key HMAC 摘要 |
| M7 | 动态价格与历史快照 | `pending` | 等待 M6 |
| M8 | Internal Recharge API | `pending` | 等待 M6 和密钥方案 |
| M9 | 管理后台精简 | `pending` | 等待核心后端功能稳定 |
| M10 | 用户后台精简 | `pending` | 等待 M6、M7、M9 |
| M11 | Codex 与 Cursor 验收 | `pending` | 等待三个 Provider 可用 |
| M12 | 安全、并发与异常测试 | `pending` | 等待 M6 到 M11 |
| M13 | Linux 生产部署 | `pending` | 等待 M12 |
| M14 | 小范围试运行 | `pending` | 等待 M13 和合规放行 |

详细任务、依赖和完成条件见 [task_plan.md](./task_plan.md)。

## 3. 本轮完成事项

### 3.1 源码基线

| 项目 | 结果 |
| --- | --- |
| Repository | `https://github.com/Wei-Shaw/sub2api.git` |
| Branch | `main` |
| Commit | `b1748c4ea99ce2120401a269142aa071e18a84da` |
| Commit time | `2026-09-03T15:40:51+08:00` |
| Latest tag at audit time | `v0.2.0` |
| Working tree after audit | clean |

### 3.2 已审计模块

* Backend 启动、配置、Wire、routes、handler、service、repository 和 Ent。
* Gateway 的 Messages、Responses、Chat Completions、Embeddings、Codex 和 Gemini 路径。
* OpenAI OAuth、API Key、DeepSeek、模型转换、Streaming 和 usage 解析。
* Group、Composite Route、Scheduler、Sticky Session、并发与限流。
* User、API Key、余额、计费、usage log、支付、兑换码、返利和权限。
* PostgreSQL migration、Redis、幂等记录、审计日志和 Secret 处理。
* Admin 与 User 前端、功能开关、Compose、反向代理及部署文档。
* 后端、前端和 deploy 测试资产。

### 3.3 已形成决策

1. V1 以 CNY 为唯一权威记账币种，API 金额均使用十进制定点字符串。
2. 保留 `users.balance` 作为热路径权威余额，新增不可变 `wallet_transactions`，不增加第二张余额主表。
3. 新增价格规则和不可变价格版本，每条 usage 保存价格快照。
4. Composite Route 增加 `target_group_id`，Gateway Group 启用 exact only 路由，三个 Provider 账号池严格隔离。
5. API Key 改为 HMAC 摘要存储且只展示一次，Account credentials 使用独立密钥域加密。
6. 已开始的 Streaming 请求允许完成并按实际 usage 扣费，余额可因此转负，之后的新请求被余额预检拒绝。
7. 外部充值使用 HMAC SHA256、Key ID、时间窗、Nonce、IP Allowlist、限流和数据库唯一约束。
8. V1 记录请求元数据、用量和错误，不保存 Prompt 或完整 Answer。

## 4. 本轮文件变更

新增规划文档：

1. `AGENTS.md`
2. `task_plan.md`
3. `architecture.md`
4. `database_design.md`
5. `api_spec.md`
6. `billing_design.md`
7. `provider_design.md`
8. `security.md`
9. `deployment.md`
10. `testing.md`
11. `findings.md`
12. `progress.md`
13. `README-runbook.md`
14. `CHANGELOG-custom.md`

上游源码目录只用于只读审计，未产生 tracked change。

## 5. 执行记录

### 5.1 已执行命令类别

* `git clone`、`git rev-parse`、`git log`、`git ls-remote` 和 `git status` 用于固定源码基线。
* `rg`、`sed` 和文件清单命令用于定位路由、schema、migration、service、前端和测试。
* `node --version` 与 `pnpm --version` 用于确认现有前端工具链。
* `pnpm install --frozen-lockfile` 用于尝试建立前端依赖基线。

### 5.2 工具链结果

| 检查项 | 结果 |
| --- | --- |
| Node.js | `v24.19.0` |
| pnpm | `11.19.0` |
| Go | 当前运行器未安装 |
| Docker | 当前运行器未安装 |
| Frontend dependency install | 依赖下载受网络策略限制，未完成 |

## 6. 测试与验证记录

### 6.1 已完成

* 确认上游 Git working tree 在审计后保持 clean。
* 统计测试资产：Backend `1239` 个 `_test.go` 文件，Frontend `251` 个 spec/test 文件，deploy 目录 `6` 个测试相关文件。
* 对设计中的路由、表、关键 service 和安全缺口进行源码路径交叉核对。
* 对全部交付文档执行文件完整性、标题、禁用标点、敏感值模式和关键术语一致性检查。

### 6.2 未完成

* 未执行 Go 单元测试和集成测试。
* 未执行 Frontend lint、typecheck、Vitest 和 production build。
* 未启动 PostgreSQL、Redis、Backend 或 Frontend。
* 未执行 Codex CLI、Cursor、OpenAI SDK 或 DeepSeek 真实请求。
* 未执行 migration、备份恢复、负载、故障注入或安全动态测试。

这些项目必须在 M0 和后续对应 Milestone 中补齐。当前状态不能解释为运行验收通过。

## 7. 当前阻塞与待确认

| 编号 | 事项 | 影响 | Owner 建议 |
| --- | --- | --- | --- |
| B-001 | 当前环境缺少 Docker 与 Go | 无法建立可运行基线 | 开发负责人 |
| B-002 | 依赖下载受限 | Frontend 测试无法执行 | 开发负责人 |
| B-003 | Codex Subscription 多用户与收费授权未确认 | M3、M14 不可放行 | 业务负责人、法律顾问 |
| B-004 | LGPL 与 README_CN 商业声明关系未确认 | 商业发布风险 | 法律顾问 |
| B-005 | DeepSeek V1 对外模型名未确认 | M5 API 契约无法冻结 | 产品负责人 |
| B-006 | 初始模型价格和开放清单未确认 | M7 无法录入正式版本 | 产品、财务负责人 |
| B-007 | RPO、RTO、备份目标与密钥托管未确认 | M13 无法验收 | 运维、安全负责人 |

## 8. 下一步执行入口

下一次开发从 M0 开始：

1. 在正式 Fork 上固定本次 commit 或经批准的 release tag。
2. 准备 Docker Compose v2、Go 1.27、Node.js 和 pnpm。
3. 按 [README-runbook.md](./README-runbook.md) 的 M0 流程启动上游原版。
4. 执行 [testing.md](./testing.md) 的基线测试并把真实结果补到本文件。
5. 基线通过后开始 M2，先关闭公开入口和商业功能。
6. M2 合并后按 `M6 -> M7 -> M3/M4/M5` 推进；M8 可在 M6 后并行设计，但正式 Provider 验收不得早于密钥、路由、钱包和价格基础。

## 9. 2026-09-06 / 0.1.1-planning 修订

### 目标

关闭规划评审发现的协议漂移、里程碑依赖、结算恢复、数据库约束、密钥轮换和备份 RPO 缺口。

### 完成任务

* 统一 Internal Recharge 请求头与调用方身份规则。
* 把 M3、M5 的正式验收依赖补齐到 M6、M7，并明确实际执行主路径。
* 为 usage 去重记录补充 fingerprint 与原结算结果引用。
* 增加价格有效区间和 V1 exact route 的数据库唯一性约束。
* 明确 API Key pepper 在线渐进轮换与到期强制换 Key。
* 增加持久化结算恢复日志、重启保持的 Billing degraded 状态和人工解除流程。
* 将生产恢复基线改为基础备份加连续 WAL/PITR，目标 RPO 不高于 5 分钟。
* 统一测试资产统计为审计记录中的 6 个 deploy 测试相关文件，等待 M0 重新计数。

### 测试结果

本次仅修订规划文档；执行了压缩包完整性、Markdown 文件清单、关键术语和跨文档一致性检查。未执行 Sub2API 代码、数据库或运行时测试。

### 下一步

在具备 Docker、Go 和依赖下载能力的开发机执行 M0，并以实际源码确认所有拟议 schema、接口和测试入口后再分配 migration 编号。

## 10. 2026-09-07 / M0 固定基线与本地运行

### 目标

在不修改上游业务逻辑和数据库 schema 的前提下，固定审计提交，构建可重复镜像，启动本地全栈并保存测试、健康、资源和延迟证据。

### 完成任务

* 从官方仓库检出 `b1748c4ea99ce2120401a269142aa071e18a84da`，配置 `upstream` 并创建 `codex/m0-baseline` 分支。
* 创建本地专用 Compose override 和 Git 忽略的开发环境配置，镜像只绑定 `127.0.0.1:8080`，PostgreSQL 与 Redis 未发布宿主机端口。
* 构建 `sub2api:m0-b1748c4`。镜像 digest 为 `sha256:74a563871167fbb5c4106b63d7668d247c6fda1470adcec629e6033cfbcb3725`，大小 `44582373` bytes。
* 启动 PostgreSQL 18、Redis 8 和 Sub2API；Backend 与内嵌 Frontend 作为同一应用容器交付，三个容器均为 `healthy`，Admin UI 与登录页返回 HTTP 200。
* 管理员账号登录成功；首次管理写操作被上游管理员合规确认门禁按设计拒绝，因此没有绕过门禁创建 Group、User 或 Key。
* 把修订后的 14 份规划文档纳入仓库，并补充项目级 `AGENTS.md` 跟踪和 POSIX 部署文件 LF 规则。

### 修改文件

* `.gitattributes`
* `.gitignore`
* `deploy/docker-compose.m0.yml`
* `AGENTS.md`、`task_plan.md`、`architecture.md`、`database_design.md`、`api_spec.md`、`billing_design.md`、`provider_design.md`、`security.md`、`deployment.md`、`testing.md`、`findings.md`、`progress.md`、`README-runbook.md`、`CHANGELOG-custom.md`
* `deploy/.env` 仅保存本地无效测试凭据且受 Git 忽略，不得用于生产。

### 数据库变更

没有自定义 schema 或 migration 变更。首次启动仅在本地测试卷执行上游原始 migrations 和初始化；尚未创建 M0 测试 Group、User 与 Key。

### 执行命令

* `docker compose ... config --quiet`、`build sub2api`、`up -d`、`ps`、`logs`。
* `docker run ... go test -tags=unit ./...`。
* `docker run ... go test -tags=integration ./...`，Testcontainers 使用 Docker Desktop socket。
* `pnpm run lint:check`、`pnpm run typecheck`、`pnpm run test:run`、`pnpm run build` 均在固定的 Frontend builder 镜像中执行。
* 五个适用于当前平台的 deploy shell tests 在只读临时副本中执行；Windows CRLF 先在临时副本规范化，仓库源文件未被测试命令改写。
* `Invoke-WebRequest` 验证 `/health`、`/admin/dashboard` 和 `/login`。

### 测试结果

| 项目 | 结果 |
| --- | --- |
| Compose config | 通过 |
| Production image build | 通过；首次因 YAML timestamp 未加引号导致 Go ldflags 失败，修正 override 后通过 |
| Backend unit | 通过，`go test -tags=unit ./...` |
| Backend integration | 通过；首次依赖下载遇到 `goproxy.cn unexpected EOF`，改用官方代理和隔离缓存后通过 |
| Frontend lint | 通过 |
| Frontend typecheck | 通过 |
| Frontend Vitest | 251 files、1825 tests 全部通过 |
| Frontend production build | 通过；保留上游 chunk size 与 Browserslist 警告 |
| Deploy tests | 适用于 Windows/Linux 基线的 5 项通过；`apple-container-test.sh` 仅适用于 macOS，本机不适用 |
| Test asset count | Backend 1239、Frontend 251、deploy 6，与规划审计一致 |
| Runtime health | `/health` 为 200 `{"status":"ok"}`；Admin UI 和登录页为 200；应用日志 error/fatal/panic 匹配数为 0 |
| `git diff --check` | 通过 |

### 资源与延迟基线

采样时 Sub2API、PostgreSQL、Redis 的 CPU 分别为 `0.75%`、`0.36%`、`0.24%`，内存分别为 `34.67 MiB`、`60.09 MiB`、`5.32 MiB`。十次本机 HTTP 采样的 P50/P95：`/health` 为 `2.56/3.57 ms`，`/admin/dashboard` 为 `2.90/11.06 ms`，`/login` 为 `2.77/2.95 ms`。该结果是空载 Windows Docker Desktop 基线，不代表生产容量。

### 新发现与风险

* `upstream/main` 已到 `ab99d56e9626e6cd731592dae8553c9758a0efa2`，比固定基线多 77 个提交并发布 `v0.2.1`。M0 继续使用规划指定提交以保证审计可重复；进入 M2 前必须决定是否在专用同步分支吸收差异。
* Windows `core.autocrlf=true` 会让缺少属性的 `.env.example`、`Dockerfile.goreleaser` 和 `deploy/Caddyfile` 以 CRLF 检出，使 POSIX 精确行匹配测试误报。已在 `.gitattributes` 为此类文件补充 LF 规则，并在临时 LF 副本复验通过。
* 管理写操作要求管理员本人确认 `v2026.06.10` 合规声明。该确认会记录用户、时间、IP 和 User-Agent，自动化不得代签。

### 阻塞与待确认

* 管理员本人尚未完成合规确认，M0 测试 Group、测试 User、测试 Key、模拟 Gateway 请求和 Streaming 首包基线未完成。
* 正式 Fork 已创建：`https://github.com/lanora-tree/sub2api`。本地 `origin` 指向 Fork，`upstream` 保持指向官方仓库，`codex/m0-baseline` 已发布并建立跟踪关系。
* 没有合法的真实 Provider 测试凭据；M0 只做 mock 或不收费探测，M3 到 M5 继续受各自人工门禁约束。

### 下一步

管理员在本地 UI 完成合规确认后，继续创建无真实业务数据的测试对象，验证 `/v1/models`、模拟 Gateway 与 Streaming，记录 request ID 和首包延迟；满足全部完成条件后才把 M0 标记为 `completed`，随后进入 M2。

## 11. 2026-09-07 / M0 正式 Fork 与运行环境恢复

### 目标

关闭正式 Fork 阻塞，发布固定基线分支，并恢复本地 Docker Desktop 运行环境，为管理员本人完成合规确认和剩余 Gateway 基线验收做准备。

### 完成任务

* 创建正式 Fork `https://github.com/lanora-tree/sub2api`。
* 配置本地 `origin` 指向 Fork，保留 `upstream` 指向官方仓库。
* 发布 `codex/m0-baseline` 并设置跟踪 `origin/codex/m0-baseline`。
* 创建或重置本地专用管理员 `m0-admin@sub2api.local`，使用项目 bcrypt 密码算法；密码不写入 Git 或进度记录。
* 更新 `task_plan.md`、`findings.md`、`progress.md` 和 `CHANGELOG-custom.md`，关闭 Fork URL 阻塞。
* Docker Desktop 4.68.0 因损坏的 Inference 与 Secrets Engine socket 启动失败；关闭未使用的 Docker AI/Inference 设置，并把临时 `Docker\\run` 与 `docker-secrets-engine` 目录分别重命名为可恢复备份 `run.stale-20260907-2210`、`docker-secrets-engine.stale-20260907-2212`，未删除镜像、容器、Secret 或数据卷。

### 修改文件

* `.gitattributes`
* `.gitignore`
* `deploy/docker-compose.m0.yml`
* 14 份规划与交接文档
* 本机 Docker Desktop 设置 `EnableDockerAI=false`、`EnableInference=false`、`EnableInferenceTCP=false`、`EnableInferenceGPUVariant=false`，该设置不纳入 Git

### 数据库变更

无。

### 执行命令

* `git remote add origin https://github.com/lanora-tree/sub2api.git`
* `git push -u origin codex/m0-baseline`
* `docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.m0.yml config --quiet`
* Docker Desktop 进程、日志、临时 socket 与设置的只读诊断命令

### 测试结果

* Fork 分支上传通过，Git tracking 配置正确。
* Compose 配置解析通过。
* 规划文件敏感值模式扫描无命中。
* `git diff --check` 通过。
* Docker Engine `29.3.1` 已恢复；Sub2API、PostgreSQL 与 Redis 容器均为 `healthy`。
* `/health` 返回 HTTP 200 `{"status":"ok"}`，`/login` 返回 HTTP 200。
* 专用管理员通过 `/api/v1/auth/login` 登录验证，返回 HTTP 200。

### 新发现与风险

Docker Desktop 4.68.0 的 Inference manager 和 Secrets Engine 会因遗留 Windows AF_UNIX socket 崩溃。已采用可恢复的临时目录重建方式处理；若问题复现，应保留日志并评估 Docker Desktop 修复版本。

### 阻塞与待确认

管理员本人仍需在本地 UI 阅读并确认上游 `v2026.06.10` 合规声明。自动化不代签。

### 下一步

恢复 M0 容器后，由管理员完成合规确认；随后创建无真实业务数据的测试对象并完成 `/v1/models`、模拟 Gateway 与 Streaming 基线。

## 12. 2026-09-12 / M0 完成验收

### 目标

在管理员本人完成上游合规确认后，关闭 M0 剩余门禁，创建无真实业务数据的测试对象，并完成模型列表、模拟 Gateway、Streaming、用量落库和资源基线验收。

### 完成任务

* 核验数据库已记录管理员 `v2026.06.10` 合规确认；自动化没有代签或直接写入确认记录。
* 恢复 Docker Desktop 29.3.1，并确认 PostgreSQL、Redis、Backend 与内嵌 Frontend 健康。
* 创建或更新测试 Group `m0-mock-openai`、普通用户 `m0-user@sub2api.local`、测试 Key `m0-mock-key` 和模拟账号 `m0-mock-upstream`。
* 模拟账号只使用本地 Docker 网络和无价值占位凭据，不连接真实 Provider。
* 验证 `/v1/models`、非流式 `/v1/chat/completions` 与 Streaming `/v1/chat/completions`。
* 核对两条请求的 usage、成本、首包时间和用户余额变化。
* 验收后把模拟账号设为 inactive，并移除临时 mock 容器和脚本；未保存或输出测试 Key 明文。
* 重新抓取官方 `upstream/main` 引用并记录与固定基线的偏差，没有 merge 或 rebase。

### 修改文件

* `task_plan.md`
* `findings.md`
* `progress.md`
* `README-runbook.md`
* `CHANGELOG-custom.md`
* Docker Desktop 本地运行时目录备份不纳入 Git：`run.stale-20260912-01` 与 `docker-secrets-engine.stale-20260912-01`

### 数据库变更

没有自定义 schema 或 migration 变更。仅在本地测试数据中创建 Group ID 3、User ID 2、API Key ID 1 和 Account ID 1。Account 最终状态为 inactive。两条 usage 各记录 4 input tokens、3 output tokens 与 `0.0000550000` 成本，测试用户余额由 `100` 变为 `99.99989000`。

### 执行命令

* `docker compose ... ps`、`config --quiet`、`docker stats --no-stream` 和固定镜像 inspect。
* 管理员与普通用户通过 `/api/v1/auth/login` 登录；普通用户继续访问 `/api/v1/user/profile`。
* 通过 Admin 与 User API 创建或更新测试 Group、Account、User 和 Key。
* 本地临时 OpenAI Compatible mock 响应 `/v1/models` 与 `/v1/chat/completions`；验收后移除。
* PostgreSQL 只读查询核对合规版本、对象绑定、usage、首包时间、成本和余额。
* `git fetch upstream main` 与 `git rev-list --count b1748c4..upstream/main`。

### 测试结果

| 项目 | 结果 |
| --- | --- |
| Compose config | 通过 |
| 核心组件 | PostgreSQL、Redis、Backend、内嵌 Frontend 健康；应用仅发布 `127.0.0.1:8080` |
| 页面与健康 | `/health`、`/login`、`/admin/dashboard` 均 HTTP 200 |
| 登录 | Admin 与普通 User 登录通过，User profile 与预期账号一致 |
| `/v1/models` | HTTP 200，仅返回测试 Group 允许的 `gpt-5.4` |
| 非流式 Gateway | HTTP 200，内容 `m0 mock ok`，7 tokens，客户端耗时 `84.9 ms`，request ID `877027cf-d355-4bbf-8172-f9fe12d47b21` |
| Streaming Gateway | HTTP 200，3 个数据帧，内容 `m0 stream ok`，客户端首帧 `108.7 ms`、总耗时 `140.8 ms`，服务端落库 first token `76 ms`，收到 `[DONE]`，request ID `7678b24f-3a87-4c1f-bcb3-fbb87daf5851` |
| Usage 与余额 | 2 条 usage 均落库；合计扣费 `0.0001100000`，余额结果精确匹配 |
| 原始测试 | 固定基线先前已通过 Backend unit/integration、Frontend lint/typecheck/1825 tests/build 和 5 个适用 deploy tests；本次未修改业务源码，无需重复下载或重跑全量套件 |
| 核心资源采样 | Sub2API `0.89% / 35.92 MiB`，PostgreSQL `0.55% / 63.26 MiB`，Redis `0.21% / 8.559 MiB` |
| 固定镜像 | `sha256:74a563871167fbb5c4106b63d7668d247c6fda1470adcec629e6033cfbcb3725`，`44582373` bytes |

### 新发现与风险

* OpenAI API Key 账号未探测时默认走 Responses。M0 mock 对 `/v1/responses` 返回 404 后，上游探针正确写入 `openai_responses_supported=false`，随后 Chat Completions 直转成功。
* `upstream/main` 当前为 `4726bdd08b6201d426a80529b79be123a4008d20`，比固定基线多 349 个提交。M2 继续使用已验收基线；同步需要单独审阅。
* 上游原始系统仍明文保存 User API Key 与 Account credentials。M0 仅使用无价值测试值；任何真实 Secret 进入系统前必须完成 M6。
* Docker Desktop 4.68.0 再次遇到遗留 Inference 与 Secrets Engine socket；把精确运行时目录重命名为可恢复备份后恢复，未删除镜像、容器或数据卷。

### 阻塞与待确认

M0 无剩余阻塞。M3 的 Codex Subscription 授权、许可证解释、CNY 迁移、DeepSeek 模型名和生产运维决策仍按各自 Milestone 保持人工门禁。

### 下一步

M0 状态改为 `completed`，M2 改为 `in_progress`。下一项按顺序关闭公开注册、支付、促销、邀请、返利、第三方登录和无关入口，并为直接 API 请求补齐 fail closed 测试。

## 13. 2026-09-12 / M2 开始：范围收口安全说明

### 目标

用数据库默认开关、前端路由/菜单控制和后端路由守卫共同关闭公开注册、自助支付、优惠码、邀请码、返利、第三方登录、Model Plaza、可用渠道、插件管理与公开渠道监控；保留管理员创建用户、账号密码登录和 Gateway 主链路。

### 安全边界与失败默认

* 后端是最终边界：被关闭功能的直接 API 请求返回稳定的 feature-closed/404 响应，不能仅依赖前端隐藏。
* 开关读取失败或值缺失时按关闭处理；数据库迁移把目标开关显式写为 `false`。
* 支付配置入口保留给管理员用于受控恢复，但支付订单、套餐、Webhook 和公开支付页面在关闭时不可用。
* 不删除上游功能源码；恢复功能必须由管理员显式打开对应开关。

### 计划测试

* 覆盖通用后端功能守卫、注册/支付/Webhook/兑换码/公开页面以及可用渠道与监控的关闭响应。
* 覆盖前端功能开关、菜单隐藏和直接路由跳转。
* 回归管理员创建 User、普通 User 登录、API Key/Gateway 相关代码路径与既有测试。

## 14. 2026-09-12 / M2 完成：公开与商业功能范围收口

### 目标

在不删除上游实现的前提下，把 V1 范围外的公开注册、自助商业、第三方登录和无关门户能力改为管理员显式启用，并确保前端隐藏不能被直接 API 请求绕过。

### 完成任务

* 新增通用后端 `featureEnabledGuard`，关闭、缺失、仓储读取失败或服务未注入时统一返回 HTTP 404、`FEATURE_DISABLED` 和功能标识。
* 注册、优惠码、邀请码、支付订单、公开支付查询、全部支付 Webhook、兑换码、订阅、返利、可用渠道、公开监控、插件 UI 与对应管理 API 均接入服务端守卫；只保留支付配置读写入口供管理员受控恢复。
* 前端把范围外功能统一改为 opt-in；菜单、直接路由、公开支付回调页和订阅轮询均只在开关明确为 `true` 时启用，设置加载失败时按关闭处理。
* 新增 forward-only migration `239_v1_scope_defaults.sql`，将 19 个范围开关写为 `false`；该 migration 只在首次升级时执行，后续管理员仍可显式恢复开关。
* 新增 `deploy/docker-compose.m2.yml`，构建并运行固定本地镜像 `sub2api:m2-scope-closure`。

### 修改文件

* Backend routes：`backend/internal/server/routes/{feature_guard,auth,payment,user,admin}.go` 及关闭状态测试。
* Settings：`backend/internal/service/setting_features.go`、`setting_parse.go`、`setting_public.go` 及相关测试。
* Database：`backend/migrations/239_v1_scope_defaults.sql` 与 migration 回归测试。
* Frontend：`frontend/src/utils/featureFlags.ts`、`router/index.ts`、`App.vue`、Header、Sidebar 与路由/菜单测试。
* Deployment/docs：`deploy/docker-compose.m2.yml`、计划、运行手册、发现、进度和定制变更日志。

### 数据库变更

* `schema_migrations` 已记录 `239_v1_scope_defaults.sql`。
* 本地升级库逐项核对 19 个开关，结果全部为 `false`；不包含真实账号、真实 Provider Secret 或真实支付数据。

### 执行命令

* Backend：在固定 Go 1.27 builder 中执行聚焦 routes/migration/unit 测试与 `go test ./...`。
* Frontend：在固定 Node builder 中执行 `lint:check`、`typecheck`、全量 Vitest 和 production build。
* Runtime：`docker build`、`docker compose ... config --quiet`、`up -d --force-recreate`、`ps`、`docker logs`、只读 PostgreSQL 查询与本地 HTTP 黑盒检查。

### 测试结果

| 检查 | 结果 |
| --- | --- |
| Backend 全量 | `go test ./...` 全部 package 通过 |
| Frontend 全量 | lint 与 typecheck 通过；251 个文件、1838 个测试通过；production build 通过 |
| Compose/runtime | Sub2API、PostgreSQL、Redis 全部 healthy；`/health` HTTP 200 |
| 关闭 API | 注册、优惠码、邀请码、公开支付、Stripe Webhook 均 HTTP 404 + `FEATURE_DISABLED`；Model Plaza HTTP 404 |
| 开关口径 | Public settings 中注册、支付、订阅购买、优惠/邀请/返利、第三方登录、监控、可用渠道、Model Plaza、插件全部为 `false` |
| 保留主链路 | 密码登录、Admin User 创建和 Gateway 路由仍存在并保持认证边界；M0 已验收的 Admin/User/Gateway 实现未被 M2 修改，全量回归通过 |
| 镜像 | `sub2api:m2-scope-closure`，digest `sha256:9c2092111a6a816b276210ae682b99335d415dce25024bf068e6d4fdd0282d27`，44583506 bytes |

### 新发现与风险

* 上游 feature flag 的历史默认并不一致，Payment 与 Channel Monitor 原为 opt-out；M2 已改为前后端一致的 opt-in，避免设置请求失败时暴露入口。
* Migration 编号 239 基于当前 `upstream/main` 最大编号 238 分配。后续同步上游前仍需在专用分支检查编号与 checksum 冲突。
* 本地启动日志中的 URL allowlist、trusted proxies、CORS 与 GitHub version sync 警告属于既有开发配置；生产前仍由 M12/M13 收口。

### 阻塞与待确认

M2 无剩余阻塞。M6 涉及权威记账币种、不可变钱包流水、Key 摘要和 Account credential 加密；任何真实凭据或收费数据进入系统前必须完成。

### 下一步

按主路径进入 M6，先固化 CNY/存量数据迁移决策和 schema，再实现钱包事务、用户模型权限、API Key 摘要、Account credential 加密与显式目标 Group 路由。

## 15. 2026-09-12 / M6.1 开始：CNY 钱包流水基础

### 范围与决策

产品负责人已明确确认 CNY 为全站唯一权威记账币种。本任务只完成 `wallet_transactions`、现有测试余额开账、Decimal 金额边界和可重复对账基础，不在同一提交顺带实现 API Key 摘要、Account credential 加密、用户模型权限或 Composite target Group。

### 敏感改动说明

* 原因：现有 `users.balance` 只有当前值，无法形成不可变、可审计的 CNY 总账；新增账务功能继续使用 float 会扩大精度与并发风险。
* 影响面：`backend/ent/schema`、新增 migration、钱包 repository/service、Admin 余额变更的后续接入点，以及对应集成/并发/对账测试。Gateway usage 扣款暂不在本子任务切换，避免跨任务大改结算热路径。
* 回滚方式：应用可回退至 M2，仍以 `users.balance` 运行；migration 为 forward-only，不删除流水表或开账数据。测试库可从升级前备份恢复，生产不得执行逆向删除。
* 数据策略：当前库只有 M0 无价值测试余额，按数值 1:1 记为 CNY opening adjustment；不做汇率换算。任何真实 USD 存量必须另建经批准的迁移清单与汇率快照。

### 预计修改文件

* `backend/ent/schema/wallet_transaction.go`、User edge 与 Ent 生成文件。
* `backend/migrations/240_cny_wallet_transactions.sql` 及 schema/migration 测试。
* 独立的钱包 domain/repository/service 文件和 Decimal、并发、幂等、不可变、对账测试。
* `database_design.md`、`billing_design.md`、`findings.md`、`progress.md`、`CHANGELOG-custom.md`。

### 验收命令

* 聚焦 Go unit、repository integration 与 migration schema 测试。
* 真实 PostgreSQL 升级：检查 opening 流水、`SUM(amount)=users.balance`、唯一约束与 UPDATE/DELETE 拒绝。
* `go test ./...`、`git diff --check`、Compose 健康与 migration checksum 检查。

### 已实现

* 新增 `wallet_transactions` migration、CNY opening adjustment、数据库金额/符号/指纹/文本约束、查询索引和 UPDATE/DELETE 拒绝 trigger。
* 新增 WalletTransaction Ent schema 并生成关联代码；所有流水字段与边均为 immutable，外键删除策略为 RESTRICT。
* 新增 Decimal 钱包命令、固定八位表示、CNY/符号/长度校验、规范化 metadata 与 SHA-256 fingerprint。
* 新增原生 SQL wallet repository：事务级 advisory lock 防幂等竞态、`SELECT ... FOR UPDATE` 串行化用户余额、余额更新与流水插入同事务、失败回滚和精确对账。
* usage 允许零金额审计流水；充值/退款为正，usage 为零或负，普通调账非零；opening migration 可以为零。
* 新增单元、SQL mock、migration schema、不可变、opening、并发重试、并发余额与对账测试；integration harness 的 Docker 探测增加 5 秒超时。

### 当前验证结果

| 检查 | 结果 |
| --- | --- |
| Wallet service 定向测试 | 通过 |
| Wallet repository SQL mock | 通过 |
| Repository 全套非 integration | 通过；需在测试子进程清空强制 `127.0.0.1:9` HTTP 代理，避免本地 Aliyun mock 被代理 |
| Migration 与 Ent 全套 | 通过 |
| Ent / Wire generate | 通过 |
| Integration 测试源码 | `go test -c -tags integration` 通过，生成 114 MB 测试二进制 |
| PostgreSQL 并发/开账/trigger 实跑 | 待 Docker 恢复；当前未虚报为通过 |
| Service 全套非 integration | 钱包用例通过；另有 4 个 Windows plugin 临时文件占用失败和 1 个 content moderation 1 秒异步等待失败，均不在 M6.1 调用链 |
| `git diff --check` | 通过，仅有 Git 的 CRLF 转换提示 |

### 环境事件与恢复

Docker Desktop 4.68.0 因上次异常退出留下 AF_UNIX socket reparse point，启动时依次在 `dockerInference` 和 `docker-secrets-engine\\engine.sock` 失败。已把以下纯运行目录原地改名保留，未触碰镜像、容器、卷或 VHDX：

* `%LOCALAPPDATA%\\Docker\\run.stale-20260912-202922`
* `%LOCALAPPDATA%\\docker-secrets-engine.stale-20260912-203343`

Docker 在当前 Windows 会话仍会重现新套接字故障。官方同类问题的恢复记录要求完整重启 Windows；用户确认重启后，先检查 Docker 健康，再执行 PostgreSQL integration、升级库 migration、opening 对账和 trigger 黑盒验收。

### 当前状态与下一步

M6.1 实现完成但动态数据库验收未完成，保持 `in_progress`。Windows 重启恢复 Docker 后立即补齐真实 PostgreSQL 证据；随后才本地提交本子任务，并按顺序进入 M6.2 管理员充值/退款/调账接入。Gateway usage 事务切换仍留在后续 M6 账务整合任务。

## 16. 2026-09-13 / M6.1 完成：CNY 钱包流水基础

### 完成任务

* Windows 重启后确认 Docker Desktop 首次仍被旧 `Docker\\run\\dockerInference` 重解析点阻塞；在 Docker 进程退出后将精确运行目录移动到 `run.stale-after-reboot-20260912-221448`，再次启动后 Linux Engine 恢复。未执行 factory reset，未触碰镜像、容器、卷或 VHDX。
* 对当前本地测试库执行升级前 `pg_dump`，备份保存于 `%TEMP%\\sub2api-m6-backups\\pre-m6-wallet-20260913-015958.dump`，`pg_restore -l` 可读取 1193 个 TOC 条目。
* 在 Go 1.27 Linux builder 中通过 Docker socket 启动 PostgreSQL 18 与 Redis 8.4，实跑 migration schema、opening、不可变 trigger、并发更新、并发幂等重放、失败回滚和精确对账测试。
* 构建 `sub2api:m6-wallet-foundation`，以独立无宿主端口的临时实例连接现有本地测试库执行 migration；实例健康后完成只读对账并移除。稳定的 M2 实例全程保持健康。

### 动态数据库证据

| 检查 | 结果 |
| --- | --- |
| Docker Engine | Client/Server `29.3.1`，Docker Desktop Linux Engine 正常 |
| 聚焦 integration | `go test -tags integration ./internal/repository` 的 M6.1 用例全部通过，包耗时 `10.461s` |
| 本地 migration | `240_cny_wallet_transactions.sql` 已记录，SHA-256 checksum 长度 64 |
| 本地 opening | 2 个 User、2 条 opening 流水、2 个唯一幂等键 |
| 本地对账 | `users.balance - SUM(wallet_transactions.amount)` 差异用户数为 0 |
| 约束与 trigger | 非 CNY 流水为 0；不可变 trigger 恰好 1 个；`users.balance` 为 `NUMERIC(20,8)` |
| Backend 全量 | Linux Go 1.27 执行 `go test ./... -count=1`，所有 package 通过，包括 service、repository、migration 与 Ent schema |
| Runtime | M6.1 临时实例健康；验收后已移除，M2 的 Backend、PostgreSQL、Redis 均保持 healthy |

### 状态与下一步

M6.1 完成并可形成独立提交。Migration 已在本地测试库执行，因此 `240_cny_wallet_transactions.sql` 从此按已发布 migration 对待，不得重写、重命名或删除。M6 继续保持 `in_progress`，下一项为 M6.2：把管理员充值、退款、调账和用户余额查询接入钱包事务与授权边界；Gateway usage 事务切换仍留在后续 M6 账务整合任务。

## 17. 2026-09-13 / M6.2 开始：管理员钱包操作与余额查询

### 目标与边界

本子任务只把管理员充值、管理员正负调账、一次性全额 usage 退款，以及管理员/当前用户余额查询接入 M6.1 钱包事务。Gateway usage 结算、外部充值、API Key 摘要、Account credential 加密和前端整体 CNY 改版仍按后续子任务处理。

### Schema 与账务变更说明

* 原因：退款必须引用原 usage 流水，并且即使管理员换用新的请求幂等键，也只能全额退款一次；仅靠 HTTP 幂等键不能表达该业务唯一性。
* 影响：新增 forward-only migration `241_wallet_refund_link.sql`，为 `wallet_transactions` 增加只读的 `reverses_transaction_id` 自引用外键、refund 类型配对约束和部分唯一索引。`240_cny_wallet_transactions.sql` 保持原样。
* 回滚：应用可回退到 M6.1，但 migration 不逆向删除。新增列为空时不影响旧代码；已写退款流水不可更新或删除，只能用新的补偿调账纠正。
* API：所有新增金额字段均为十进制定点字符串；管理员写操作要求可解析到真实管理员的认证 subject、step-up 门控、非空 `Idempotency-Key` 和非空备注；当前用户查询仅从认证 subject 获取 user ID。
* 负余额：管理员负调账禁止把余额降到零以下；usage 退款只退原 usage 流水的完整绝对金额。普通管理员接口不能写任意 refund 金额。

### 计划测试

* Service：字符串金额、小数位/范围、操作类型、管理员/备注/幂等键门禁、退款派生与错误映射。
* Repository：退款原流水校验、跨用户拒绝、非 usage 拒绝、一次全额唯一性、并发退款、事务回滚与对账。
* Handler/route：管理员写入、当前用户隔离、Decimal 字符串响应、缺少幂等键或身份拒绝。
* Milestone 子任务结束前：Ent/Wire generate、Backend 全套、Frontend lint/typecheck/相关测试、真实 PostgreSQL integration、本地升级库 migration 与黑盒验收、`git diff --check`。

## 18. 2026-09-13 / M6.2 完成：管理员钱包操作与余额查询

### 完成任务

* 新增 `WalletService`，管理员充值只接受正数且最多两位小数，调账只接受非零有符号字符串且最多八位小数；所有管理员写入强制真实 operator、非空请求幂等键和非空备注。
* 管理员充值、调账、一次性全额 usage 退款统一进入 M6.1 钱包事务；退款金额从原负数 CNY usage 派生，不接受调用端金额。成功写入后失效 API Key auth 与 billing balance 缓存。
* 新增管理员余额查询和当前用户余额查询。普通用户 handler 只读取认证 subject，不接受外部 User ID；响应金额固定为八位十进制字符串。
* 新增 forward-only migration `241_wallet_refund_link.sql`：退款自引用外键、类型配对 check、同一 usage 一次退款的部分唯一索引，以及同用户/负数 CNY usage/完整反向金额的数据库 trigger 校验。
* 管理员钱包写路由接入 step-up 中间件和 Admin audit action；旧 `/users/:id/balance` 只作为相同严格契约的兼容别名，不再支持任意 set。
* 通用 Admin User 创建强制零初始余额，通用更新拒绝 balance 字段，并移除旧 float 型 `AdminService.UpdateUserBalance`；Admin UI 删除创建余额输入，并把充值/扣减改为 CNY 字符串、稳定幂等键、必填备注和 TOTP step-up。

### 数据库变更与恢复

* 原因、影响和回滚边界已在 M6.2 开始记录及 `database_design.md` 固化；migration 240 未改写。
* 升级前备份：`C:\Users\Administrator\AppData\Local\Temp\sub2api-m6-backups\pre-m6-2-20260913-064855.dump`，573479 bytes，`pg_restore -l` 可读取 1218 个 TOC 条目。
* `241_wallet_refund_link.sql` 已写入本地测试库，checksum 长度 64。该 migration 从此视为已发布，禁止重写、重命名或删除。
* 应用可回退 M6.1；schema 保持 forward-only。测试库可用上述备份重建，生产纠错只能新增补偿调账，不得修改不可变流水。

### 验收结果

| 检查 | 结果 |
| --- | --- |
| Backend 聚焦测试 | Service、repository SQL mock、admin/user handler、route step-up、audit 全部通过 |
| Backend 全量 | Go 1.27 Linux builder 执行 `go test ./... -count=1`，所有 package 通过 |
| Frontend | ESLint、Vue typecheck 通过；252 个文件、1841 个测试通过；production build 通过 |
| PostgreSQL integration | PostgreSQL 18.1 + Redis 8.4 实跑 migration、opening/immutability、refund trigger、并发一次性退款和 repository 套件，包耗时 `39.373s` |
| 本地升级库 | migration/FK/check/unique index/trigger 各 1；2 个 User、2 条 opening 流水；无非法 refund link；对账差异用户数 0 |
| Runtime 黑盒 | 临时无宿主端口实例 healthy，`/health` 返回 ok；未认证的 user/admin 钱包请求均 HTTP 401；验收后临时实例已移除 |
| 稳定实例 | 现有 `sub2api:m2-scope-closure` 实例全程 healthy，未提前切换未完成的 M6 分支 |
| 镜像 | `sub2api:m6-admin-wallet`，digest `sha256:d34ceec2902ed37a6861e39a12b4dfc4267341b782e8f0680dca02dd33068f5e`，44730339 bytes |
| Git | Ent/Wire generate 通过；`git diff --check` 通过（仅 CRLF 提示） |

### 剩余边界与下一步

* Gateway usage 尚未切入 wallet transaction，完整收费闭环和 scheduled reconciliation 仍属 M6 后续任务。
* 公开注册由 M2 保持关闭；若未来恢复，非零默认余额必须通过开账事务，不能直接写 `users.balance`。
* M6 保持 `in_progress`。按计划进入 M6.3：新增 `user_model_permissions` 并在请求前执行用户与模型授权。之后依次完成 API Key HMAC 摘要、上游凭据加密、对账告警和 Composite 精确目标 Group 路由。

## 19. 2026-09-13 / M6.3 开始：用户模型权限

### 目标与边界

本子任务只新增 `user_model_permissions`、管理员完整替换 API 和 Gateway 请求前授权检查，并让模型目录按当前用户权限过滤。API Key HMAC 摘要、Account credential 加密、价格门禁、usage 钱包结算、对账告警以及 Composite `target_group_id`/`composite_explicit_routes_only` 仍按后续 M6/M7 子任务实施；不把尚未完成的 M6 分支切换为稳定本地实例。

### Schema 与 Gateway 敏感改动说明

* 原因：现有 Group 绑定只能限定访问入口，不能表达“某个 User 是否允许使用某条逻辑模型 Route”；继续依赖模型前缀或账号池能力会让未授权模型进入调度链路。
* 影响：新增 forward-only migration `242_user_model_permissions.sql`、Ent schema/生成代码、权限 repository/service、Admin handler/route、Gateway route middleware 和对应 API 客户端类型。权限检查位于 Composite 显式 Route 命中后、`requireGroup` 与 handler 获取上游并发槽之前；模型目录只保留显式授权的逻辑模型。
* 回滚：应用可回退至 M6.2，新增表与历史权限行保留且不逆向删除；生产纠错通过新的权限替换或后续 forward migration。稳定 M2 容器不加载该未完成版本，所以本地既有调用不会被默认拒绝策略提前中断。
* 默认策略：新用户没有权限行即拒绝；Route 停用后授权自动失效；数据库或权限查询异常一律 fail closed；只认 Composite 显式 Route，detector/account ownership 产生的隐式决策不能获得授权。
* 路由语义：一旦 Route 被任何权限行引用，当前已有的 `public_model`、`match_type`、`target_platform`、`upstream_model` 与 `endpoint` 禁止原地修改；`target_group_id` 在后续 M6 路由子任务加入时同步扩展该 trigger。

### 上游与迁移号复核

* 2026-09-13 已重新 `git fetch upstream main`；`upstream/main` 为 `bdb42e22f81fcb633ff0a060961211dd2bcb515b`（2026-09-12，固定审计基线落后 399 个提交）。本任务继续在已验收的固定基线上开发，不静默同步。
* 当前 `upstream/main` migration 最大编号为 238，本分支最大编号为 241，因此本子任务使用 242；已发布的 239、240、241 不修改。

### 计划测试

* Migration/真实 PostgreSQL：复合主键、外键、索引、默认拒绝、Route 停用失效、语义字段 trigger、允许启停/优先级/备注修改。
* Repository/service：完整替换的版本冲突、去重/非法 Route、并发覆盖保护、禁用 Route、管理员 actor 和事务回滚。
* Handler/route：管理员认证、step-up、BIGINT 字符串边界、GET/PUT envelope、审计与稳定错误映射。
* Gateway：显式 Route 授权通过；无记录、禁用权限、隐式 Route 在上游并发前拒绝并返回 403 `MODEL_NOT_ALLOWED`，数据库错误 fail closed 并返回 503 `MODEL_PERMISSION_UNAVAILABLE`；Models 仅返回授权交集。
* 回归：Ent/Wire generate、完整 Backend、Frontend lint/typecheck/全量 Vitest/build、真实 PostgreSQL integration、升级库备份/migration/黑盒健康、`git diff --check`。

## 20. 2026-09-13 / M6.3 完成：用户模型权限

### 实现结果

* 新增 `user_model_permissions` 复合主键表、Ent edge schema、repository/service 和管理员 GET/PUT API。权限集合以强 ETag 作为不透明版本，PUT 在用户维度持有事务级 advisory lock 后做完整替换；历史行保留，空数组撤销全部权限。
* 管理员 PUT 由 step-up 中间件保护；GET 和 PUT 均有明确审计 action，PUT 记录目标 User、Route 数量和新版本。所有浏览器可见 BIGINT ID 使用字符串，避免 JavaScript 精度损失。
* Composite 模型请求在显式 exact Route 命中后、进入 handler 和取得上游并发槽前校验用户权限。无记录、Route/Group 停用、隐式 detector/account ownership 或数据库错误均 fail closed；稳定错误为 403 `MODEL_NOT_ALLOWED` 或 503 `MODEL_PERMISSION_UNAVAILABLE`。
* OpenAI、Codex 和 Gemini 模型目录只返回当前用户被授权且有效的 Route；空集合不再回退到静态或账号模型。Images 请求省略 model 时按现有默认 `gpt-image-2` 执行相同权限检查。
* Route 一旦被权限行引用，Group、public model、match type、Provider、upstream model、endpoint 和删除状态不可原地改变；priority、enabled 与 notes 保持可操作。未来 M6 的 `target_group_id` forward migration 必须扩展同一 trigger。

### 测试与本地升级证据

| 项目 | 结果 |
| --- | --- |
| 代码生成 | `go generate ./ent`、`go generate ./cmd/server` 通过 |
| Backend | Linux 容器内 `go test ./... -count=1` 全部通过 |
| Frontend | lint、typecheck、252 个测试文件 / 1842 个测试、production build 全部通过 |
| PostgreSQL integration | PostgreSQL 18：默认拒绝、授权、停用失效、版本冲突、语义 trigger、并发完整替换恰好一成一败均通过 |
| 升级前备份 | `C:\Users\Administrator\AppData\Local\Temp\sub2api-m6-backups\pre-m6-3-20260913-200948.dump`，581329 bytes，1222 条 TOC，SHA-256 `F2E54C28569B01A6784A8475FBF78C0B3B4A981F019016580E72231ED5243BDB` |
| 本地 migration | `242_user_model_permissions.sql` checksum 长度 64；表 3 个索引、4 个 PK/FK 约束、1 个 Route trigger；初始权限行 0 |
| 候选镜像 | `sub2api:m6-user-model-permissions`，digest `sha256:e0c582eaac77b72d37226b068e9330ac3e95ba8fa78db9b397a84e3ca8d8de8a`，44859222 bytes |
| 黑盒 | 无宿主端口候选实例连接本地 PostgreSQL/Redis 并 healthy；候选与稳定实例 `/health` 均为 ok；未认证 Admin 请求为 401；临时实例验收后移除 |
| 稳定实例 | `sub2api:m2-scope-closure` 全程 healthy，仍只发布 `127.0.0.1:8080`，未切换未完成的 M6 分支 |
| Git | `git diff --check` 通过（仅 Windows CRLF 提示） |

### 剩余边界与下一步

* M6.3 只保护 Composite 显式 Route；目标账号池目前仍只能锁定 platform，必须在后续 M6 增加 `target_group_id` 和 `composite_explicit_routes_only` 后才能隔离同平台不同成本池。
* 模型目录尚未与 M7 有效价格求交；M7 完成前该分支不是可收费上线版本。Gateway usage 钱包结算、对账告警、API Key 摘要和 Account credential 加密也仍未完成。
* 后台权限选择 UI 留在 M9；M6.3 已提供完整 Admin API 客户端契约。M6 下一顺序项为 M6.4 API Key HMAC 摘要与 Secret 一次展示。

## 21. 后续记录模板

每次工作结束追加一节，不覆盖历史记录。

```markdown
## YYYY-MM-DD / Milestone

### 目标

### 完成任务

### 修改文件

### 数据库变更

### 执行命令

### 测试结果

### 新发现与风险

### 阻塞与待确认

### 下一步
```
