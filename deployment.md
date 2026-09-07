# 部署设计

## 1. 部署目标

开发环境使用 Windows、Docker Desktop、Docker Compose、本地浏览器、Codex CLI 与 Cursor。生产环境从单台 Linux 云服务器起步，Nginx 负责 HTTPS，Sub2API、PostgreSQL 和 Redis 运行在 Docker Compose 私有网络。

本阶段只提供设计与命令模板，不执行生产部署。

## 2. 当前 upstream 部署事实

1. Compose 文件位于 `deploy/docker-compose.yml` 和 `deploy/docker-compose.local.yml`。
2. 当前 `deploy/docker-compose.yml` 使用 `weishaw/sub2api:latest`、`postgres:18-alpine` 与 `redis:8-alpine`。
3. PostgreSQL 和 Redis 默认没有发布宿主机端口。
4. App 端口默认通过 `${BIND_HOST}:${SERVER_PORT}:8080` 发布。
5. App healthcheck 使用 `http://localhost:8080/health`。
6. PostgreSQL、Redis、App 都使用持久卷。
7. Redis 开启 AOF，PostgreSQL 设置显式 PGDATA。
8. `deploy/EDGE_SECURITY.md` 提供 Nginx 与 Caddy 的边缘安全参考。
9. 启动过程会自动执行 SQL migrations，迁移采用 checksum 记录，回滚依赖数据库备份或补偿迁移。

生产禁止直接使用 `latest`。需要固定经过验收的 tag 和 image digest。

## 3. 推荐规格

| 规模 | CPU | 内存 | 系统盘 | 数据盘 | 适用情况 |
| --- | --- | --- | --- | --- | --- |
| 最低 | 2 vCPU | 4 GiB | 30 GiB | 40 GiB SSD | 3 到 10 名轻量试用，低并发 |
| 推荐 | 4 vCPU | 8 GiB | 40 GiB | 80 GiB SSD | 10 到 50 名用户，常规 Codex 和 Cursor |
| 升级 | 8 vCPU | 16 GiB | 60 GiB | 160 GiB SSD | 长上下文、高并发或日志增长明显 |

触发升级评估的条件：

1. CPU 连续 15 分钟高于 70%。
2. 内存连续 15 分钟高于 75%，或发生 OOM。
3. PostgreSQL p95 查询超过 100 ms，连接池长期等待。
4. Gateway p95 首包延迟在排除上游延迟后显著增长。
5. Redis 延迟持续高于 10 ms 或频繁连接耗尽。
6. 数据盘使用率超过 70%，预计 30 天内达到 85%。
7. 并发排队持续超过可接受阈值。

## 4. Windows 开发环境

### 4.1 前置条件

1. Windows 11 或受支持的 Windows 10。
2. WSL2。
3. Docker Desktop，启用 WSL2 backend。
4. Git。
5. Node 与 pnpm 仅在宿主机调试前端时需要。
6. Go 1.27 仅在宿主机调试后端时需要。
7. Codex CLI 和 Cursor 当前稳定版。

### 4.2 Fork 与 remote

```powershell
git clone https://github.com/OWNER/sub2api-gateway.git
Set-Location sub2api-gateway
git remote add upstream https://github.com/Wei-Shaw/sub2api.git
git fetch upstream --tags
git remote -v
```

### 4.3 本地配置

在 `deploy` 目录复制 `.env.example` 为 `.env`，只填写本地随机值。`.env` 已被 Git ignore，仍需在提交前执行 Secret scan。

建议：

```text
BIND_HOST=127.0.0.1
SERVER_PORT=8080
SERVER_MODE=debug
TZ=Asia/Shanghai
```

密码和固定 Secret 使用密码管理器生成，不在终端历史、README 或聊天中粘贴。

### 4.4 启动与检查

```powershell
Set-Location deploy
docker compose -f docker-compose.local.yml config
docker compose -f docker-compose.local.yml up -d
docker compose -f docker-compose.local.yml ps
docker compose -f docker-compose.local.yml logs --tail 200 sub2api
```

浏览器访问 `http://127.0.0.1:8080`。确认 Admin 初始化后立即更换临时密码并启用 TOTP。

### 4.5 本地客户端

Gateway Base URL 使用 `http://127.0.0.1:8080`。只使用专用测试 Key。Codex 和 Cursor 的实际配置格式在 M11 按当时版本记录到 `README-runbook.md`，当前不写死可能变化的客户端字段。

## 5. 生产目录

推荐：

```text
/opt/sub2api/
  compose.yaml
  compose.override.yaml
  .env
  config/
  data/
  postgres_data/
  redis_data/
  billing_recovery/
  backups/
  release-manifest/
```

权限建议：

* `/opt/sub2api/.env`：root 与部署组可读，mode 640 或更严格。
* Secret 单独目录：root only，mode 600。
* 数据目录：容器所需 UID/GID。
* `/opt/sub2api/billing_recovery`：仅 App 运行 UID 与受控恢复账号可读写，禁止日志采集器抓取正文，底层使用可持久化且有快照或复制能力的独立卷；不得只放在容器层或临时系统盘。
* 备份目录：专用 backup 用户，禁止 Web Server 读取。

## 6. Linux 生产准备

1. 使用受支持的 Ubuntu LTS 或 Debian stable。
2. 安装 Docker Engine 与 Compose plugin。
3. 启用自动安全更新，内核大升级使用维护窗口。
4. 配置 NTP，HMAC 时间窗依赖准确系统时间。
5. 创建非日常 root 的部署用户。
6. SSH 禁用密码登录，使用密钥和来源 IP 限制。
7. Firewall 只放行 80、443 和受限 SSH 端口。
8. 禁止把 Docker socket 提供给 App 容器。

## 7. Compose 生产约束

从 upstream Compose 创建最小 override，避免直接改动大量上游文件。

必须覆盖：

1. App image 固定 tag 与 digest。
2. `BIND_HOST=127.0.0.1`，让 Nginx 通过宿主回环访问 8080。
3. PostgreSQL 与 Redis 不设置 `ports`。
4. 固定 PostgreSQL 和 Redis major 版本及 digest。
5. 设置资源 limit 与 restart policy。
6. 设置固定 JWT、TOTP、Account Encryption、API Key Pepper 和 Recharge HMAC Secret。
7. 配置 JSON 日志、轮转与保留期。
8. 开启 URL Allowlist，禁止不安全 HTTP 与私网 upstream，所需内部 Host 逐项允许。
9. 设置 `server.trusted_proxies` 只信任 Nginx 地址。
10. 把结算恢复目录作为独立持久卷挂载；容器启动探针必须在该卷不可写或 fsync 失败时阻止 Gateway 接流量。

上线前执行：

```bash
cd /opt/sub2api
docker compose config --quiet
docker compose pull
docker compose up -d
docker compose ps
curl --fail --silent http://127.0.0.1:8080/health
```

## 8. Nginx 基线

下面使用保留示例域名和 IP。发布前替换，禁止在仓库保存私钥。

```nginx
limit_conn_zone $binary_remote_addr zone=sub2api_conn:20m;
limit_req_zone $binary_remote_addr zone=sub2api_auth:20m rate=5r/s;
limit_req_zone $binary_remote_addr zone=sub2api_api:40m rate=30r/s;
limit_req_zone $binary_remote_addr zone=sub2api_internal:10m rate=1r/s;

map $http_upgrade $connection_upgrade {
    default upgrade;
    '' close;
}

server {
    listen 80;
    server_name api.example.invalid;
    location /.well-known/acme-challenge/ { root /var/www/acme; }
    location / { return 301 https://$host$request_uri; }
}

server {
    listen 443 ssl http2;
    server_name api.example.invalid;

    ssl_certificate /etc/letsencrypt/live/api.example.invalid/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/api.example.invalid/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;

    client_header_timeout 10s;
    client_max_body_size 256m;
    large_client_header_buffers 4 16k;
    limit_conn sub2api_conn 40;

    location = /api/v1/internal/wallet/recharges {
        allow 203.0.113.10;
        deny all;
        client_max_body_size 16k;
        limit_req zone=sub2api_internal burst=10 nodelay;
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location ~ ^/(api/v1/)?auth/ {
        limit_req zone=sub2api_auth burst=10 nodelay;
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location / {
        limit_req zone=sub2api_api burst=60 nodelay;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_buffering off;
        proxy_request_buffering off;
        gzip off;
        proxy_read_timeout 1800s;
        proxy_send_timeout 1800s;
        proxy_pass http://127.0.0.1:8080;
    }
}
```

生产 Internal Recharge IP 应来自实际收费站固定出口。`203.0.113.10` 是文档保留地址。

如全站关闭 gzip 会影响静态资源，可以拆分静态页面与 Gateway locations，只在 SSE 与 WebSocket 路径关闭 gzip。上线前按 `deploy/EDGE_SECURITY.md` 验证客户端取消传播和缓存行为。

## 9. HTTPS 与域名

1. 使用独立 API 子域名。
2. DNS TTL 在首次上线前设为 300 秒，稳定后可提高。
3. 使用 Certbot 或受控 ACME 客户端自动续期。
4. 每周检查证书剩余时间，少于 21 天告警。
5. CDN 可后续增加。使用 CDN 时必须封锁 origin，仅允许 CDN egress，并重新配置 trusted proxies。

## 10. Secret 注入

开发与生产使用不同 Secret。生产变量至少包括：

```text
POSTGRES_PASSWORD
REDIS_PASSWORD
JWT_SECRET
TOTP_ENCRYPTION_KEY
ACCOUNT_CREDENTIALS_ENCRYPTION_KEY_V1
API_KEY_HASH_PEPPER_V1
BILLING_RECOVERY_ENCRYPTION_KEY_V1
INTERNAL_RECHARGE_KEYRING_FILE
```

`INTERNAL_RECHARGE_KEYRING_FILE` 指向 root only 的挂载 Secret 文件，保存 active、可选 previous Key ID、各自绑定的 source、有效期和 Secret；仓库中只提供无 Secret 的 schema 示例。`.env.example` 只列变量名和空值。主密钥不存入 PostgreSQL。Account encryption key 与 API Key pepper 轮换期间同样只允许 active 和 previous 两个受控版本。

## 11. 数据持久化

* PostgreSQL：业务权威数据，必须持久化，执行基础备份、连续 WAL 归档和每日逻辑备份。
* Redis：AOF 可以帮助恢复缓存状态，但不作为业务恢复来源。
* App data：配置、日志、运行时缓存和结算恢复日志。恢复日志必须位于独立持久卷并加密、校验、fsync；缓存可重建，未入库结算记录不可丢弃。
* Nginx 与证书：配置纳入受控仓库，私钥由 ACME 和 Secret 存储管理。

每次 `docker compose down` 前确认命令不含 `-v`。生产运维文档禁止提供无保护的卷删除快捷命令。

## 12. PostgreSQL 备份

### 12.1 策略

1. 定期生成可用于 PITR 的物理基础备份，并连续归档 WAL 到异地对象存储。
2. WAL 归档延迟持续监控；默认目标 RPO 不高于 5 分钟，超过即严重告警并暂停扩大试运行。
3. 每日另做一次 logical backup，低峰执行，用于可移植校验，不能单独承担账务恢复。
4. 保留 7 份每日、4 份每周、6 份每月逻辑备份；物理备份与 WAL 保留期必须覆盖批准的 PITR 窗口。
5. 所有备份先压缩再加密；同机只保留短期副本，异地对象存储保存完整周期。
6. 每份基础备份记录 SHA256、WAL 起止位置、schema migration 版本、镜像 digest 和加密 key ID。
7. 每月至少一次隔离 PITR 演练，至少恢复到一个非整点目标时间并测量真实 RPO、RTO。

### 12.2 备份模板

```bash
cd /opt/sub2api
backup_stamp=$(date -u +%Y%m%dT%H%M%SZ)
docker compose exec -T postgres pg_dump \
  --format=custom \
  --no-owner \
  --username "$POSTGRES_USER" \
  "$POSTGRES_DB" > "backups/sub2api_${backup_stamp}.dump"
sha256sum "backups/sub2api_${backup_stamp}.dump" > "backups/sub2api_${backup_stamp}.sha256"
```

以上仅为逻辑备份模板。物理基础备份、`archive_command` 或托管 PostgreSQL PITR 配置、加密与上传命令由最终云服务和对象存储决定，必须在 M13 固化并测试。不要把密码作为命令行参数。

### 12.3 配置备份

备份 Compose、Nginx、非 Secret config 和 release manifest。`.env` 和密钥单独加密，访问权限与数据库备份分离。

## 13. 恢复流程

恢复必须在隔离环境先验证：

1. 停止入口流量。
2. 创建新的 PostgreSQL 实例或全新数据库。
3. 验证备份 SHA256 和解密成功。
4. 使用与备份匹配的应用镜像。
5. 从物理基础备份恢复，并重放 WAL 到批准的目标时间；逻辑备份恢复仅作为兼容性演练的另一条路径。
6. 启动 Redis 空实例，让缓存重建。
7. 使用测试 Secret 阻止连接真实 Provider。
8. 检查 migration version、恢复时间线、用户数、余额、钱包流水、usage 和价格版本。
9. 检查持久化结算恢复日志；如有未归档记录，保持 Billing degraded 并幂等重放。
10. 执行账本对账和客户端 smoke test，记录最后一条可见业务事件与实际数据截止点。
11. 人工确认 RPO、RTO 和对账结果后切换生产流量。

Redis 不参与钱包和账单恢复。

## 14. 升级与 upstream 同步

### 14.1 升级前

1. 阅读 upstream release notes 和 migration。
2. 创建 Git branch `sync/upstream_YYYYMMDD`。
3. 备份数据库、配置和 release manifest。
4. 在 staging 合并并解决冲突。
5. 执行完整测试矩阵。
6. 构建并固定新 image digest。

### 14.2 发布

1. 进入维护窗口。
2. 暂停 Internal Recharge 或让外部站点重试。
3. 停止新 Gateway 请求，等待在途 Streaming 完成。
4. 再次备份数据库。
5. 拉取固定镜像并启动。
6. 检查 migrations、health、登录、三 Provider smoke、账本对账。
7. 恢复流量并观察至少 30 分钟。

### 14.3 回滚

应用兼容性问题优先回滚镜像。数据库 migration 为 forward only，出现不兼容 schema 时恢复升级前备份，或执行经过评审的补偿 migration。

回滚前停止写流量，保存故障时间窗内的充值和 usage 列表。恢复旧库会丢失备份时点后的写入，必须通过外部订单与脱敏结算记录补录。

## 15. Health 与告警

检查：

```bash
docker compose ps
curl --fail --silent https://api.example.invalid/health
docker compose logs --since 15m --tail 500 sub2api
docker compose exec -T postgres pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB"
docker compose exec -T redis redis-cli ping
```

严重故障告警：Backend health 连续三次失败、PostgreSQL 不可用、Redis 不可用、Billing degraded、任一 Provider 全部账号不可用、五分钟 5xx 超过 5%、钱包对账差异、每日备份失败、证书少于 21 天。

## 16. M13 验收清单

1. 镜像、PostgreSQL、Redis 均固定 tag 和 digest。
2. 公网端口扫描只显示 80、443 和受限 SSH。
3. HTTP 自动跳转 HTTPS。
4. SSE 首包和长流无缓冲。
5. WebSocket 与 Session header 通过。
6. App、PostgreSQL、Redis 重启后数据完整。
7. Internal Recharge 只接受白名单来源。
8. 物理基础备份、连续 WAL 归档、每日逻辑备份、加密、异地上传和延迟告警有效。
9. 隔离 PITR 演练通过，实测 RPO 不高于批准值，账本对账为零差异。
10. 升级和回滚演练完成。

## 17. 待确认

1. Linux 发行版和云服务商。
2. 域名、DNS 和证书管理方式。
3. 是否使用 CDN 或 VPN 保护 Admin。
4. 异地备份对象存储与加密方案。
5. 可接受的 RPO 与 RTO。规划默认 RPO 不高于 5 分钟、RTO 不高于 4 小时；必须以 PITR 演练实测并由业务负责人批准。只有每日 `pg_dump` 时不得开启真实充值或收费流量。
