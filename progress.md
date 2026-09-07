# 项目进度记录

## 1. 当前状态

* 项目代号：中转站
* 上游项目：Sub2API
* 记录日期：2026-09-07 UTC+8
* 当前阶段：M0 固定基线与本地运行验收
* 阶段状态：`in_progress`
* 下一阶段：管理员完成合规确认并补齐 M0 测试数据、Gateway 与 Streaming 基线

M1 静态源码审计与 14 份规划文档已完成。当前已在 Windows Docker Desktop 上构建并启动固定提交，完成大部分 M0 运行与测试验收；Sub2API 业务代码、数据库 schema 和 migration 均未修改。

## 2. 里程碑看板

| Milestone | 内容 | 状态 | 进度说明 |
| --- | --- | --- | --- |
| M0 | Fork 与原始系统运行 | `in_progress` | Fork、镜像、全栈、测试与登录通过；等待管理员合规确认和 Gateway 基线 |
| M1 | 源码分析与设计文档 | `completed` | 源码审计、方案设计和交叉检查已完成 |
| M2 | 关闭公开与商业功能 | `pending` | 等待 M0 |
| M3 | Codex Subscription 验证 | `pending` | 等待 M2、M6、M7、合规确认和合法测试账号 |
| M4 | OpenAI Official API | `pending` | 等待 M2、M6、M7 的基础能力 |
| M5 | DeepSeek Official API | `pending` | 等待 M2、M6、M7、模型名和价格人工确认 |
| M6 | 用户、密钥、路由与钱包基础 | `pending` | 等待数据库方案确认 |
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

## 12. 后续记录模板

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
