# APAGE 实施 Checklist（前后端完整实现计划）

> **执行状态（2026-06-22）**：本计划已据此实现。MVP-0 / MVP-1 及大部分 V1 控制面已落地并端到端验证（tunnel 预览、cloud 上传→扫描→预览、撤销、原子 maxViews、密码门、数据删除等真实跑通）。逐项状态见仓库根目录 [`IMPLEMENTATION-STATUS.md`](../IMPLEMENTATION-STATUS.md)；运行方式见 [`README.md`](../README.md)。需外部服务的项（ClamAV、LibreOffice、ACME、Safe Browsing、Admin SSO/MFA）按 spec §21 作为生产强化项，已留 `TODO(prod)` 接入点。

本文件是 APAGE 的端到端实施计划，按**实施顺序**组织，覆盖 `apage-spec.md`（产品/后端 spec）与 `apage-ui-spec.md`（前端 spec）的全部内容。每项为可勾选任务，`§` 引用对应 spec 章节，便于核对来源与避免遗漏。

> 阶段划分对齐 spec：MVP-0 / MVP-1 / 暂缓项（§21）+ 部署里程碑 Phase 0–3（§22.4）。
> 图例：`[B]` 后端 · `[F]` 前端 · `[I]` 基建/DevOps · `[S]` 安全/合规专项。
> 完成标准遵循 §18 SLO 与压测验收（§18 验收用例）。

---

## 阶段 P0 — 项目基建与本地验证（Phase 0，§22.4）

目标：可在本地用 Docker Compose 跑通 `fileRef → preview link → tunnel streaming → revocation`。

### P0.1 仓库与工程结构 [I]
- [ ] 建立 monorepo（或多 repo）：`apage-api`、`apage-gateway`、`apage-worker`、`apage-agent`、`apage-web`（§22.2 服务拆分）
- [ ] 三个后端服务为**独立容器、独立进程**，配置全部经 env var 注入（§22.3 迁移约束）
- [ ] 统一技术栈：Go（api/gateway/worker/agent），Next.js（web）（§23）
- [ ] API 框架选型 Fiber/Gin/Chi，队列选型 Asynq（§23）
- [ ] 容器镜像推送 ECR/GHCR，日志输出 stdout/stderr（§22.3）

### P0.2 本地编排 [I]
- [ ] `docker-compose`：api + gateway + worker + postgres + redis + **minio**（§22.4 Phase 0）
- [ ] Caddy/Nginx 反向代理，承载 HTTPS + WebSocket + wildcard routing（§22.2）
- [ ] 本地 wildcard DNS / hosts 映射 `*.preview.localhost`
- [ ] 环境变量清单落地（§33）：`APP_BASE_DOMAIN`、`DATABASE_URL`、`REDIS_URL`、`S3_*`、`JWT_SIGNING_SECRET`、`DIRECT_UPLOAD_MAX_BYTES` 等

### P0.3 数据库与迁移 [B]
- [ ] 引入 migration 工具（schema 用 migration 管理，§22.3）
- [ ] 建表（§32 表清单）：`tenants`、`users`、`memberships`、`quotas`、`agent_instances`、`file_refs`、`files`、`preview_links`、`custom_domains`、`audit_logs`、`abuse_reports`、`idempotency_keys`
- [ ] 索引：`preview_links(tenant_id, created_at)`、`files(tenant_id, expires_at)`、`audit_logs` 按 `tenant_id/created_at` 分区（§19.7）
- [ ] `preview_links.file_ref` 与 `file_id` 互斥约束（应用层 + 可空列，§2）

### P0.4 通用 API 约定（横切，所有 endpoint 复用） [B]
- [ ] Cursor/keyset 分页：`limit`(默认20/最大100)、`cursor`、`order`，禁用 offset（§统一 API 约定）
- [ ] 列表响应 envelope：`{ items, nextCursor, hasMore }`
- [ ] 列表只返回当前租户可见资源，跨租户视为不存在（§统一约定 / 404）
- [ ] 通用错误 envelope：`{ error: { code, message, requestId, retryable } }`
- [ ] HTTP 状态码映射：400/401/403/404/409/410/413/415/429/500/503（§统一约定）
- [ ] Idempotency-Key 中间件：写接口支持，24h 内同键返回相同结果，按 `tenant+instance+endpoint` 隔离；同键不同 body 返回 409（§统一约定 / P1-3）
- [ ] 限流响应：429 + `Retry-After` + `RateLimit-Limit/Remaining/Reset`（§统一约定）
- [ ] requestId 注入与日志关联

### P0.5 前端设计系统基座 [F]
- [ ] CSS variable 实现 design token：色彩/字体/间距/圆角/阴影（UI §2.1–2.3）
- [ ] light / dark 双主题切换，组件只引用语义 token（UI §2）
- [ ] 栅格与布局：营销页 12 列/1200px、后台侧栏 240px(可折叠 64px)/内容 1280px、断点 sm/md/lg/xl（UI §2.4）
- [ ] 通用组件库（UI §3，三端复用）：Button/IconButton/Input/Select/Combobox/Checkbox/Radio/Switch/Badge/Tag/Table/Pagination/Tabs/Card/Modal/Drawer/Toast/Banner/Tooltip/CopyField/**SecretReveal**/CodeBlock/StatusDot/EmptyState/Skeleton/Stat/DateRange/ConfirmDialog
- [ ] 通用交互模式：状态 Badge 映射（UI §4.1）、危险操作二次确认（UI §4.2）、脱敏与密钥展示（UI §4.3）、加载/空态/错误（UI §4.4）、实时刷新轮询/SSE（UI §4.5）
- [ ] 无障碍基线：键盘可达、focus ring、对比度、aria、焦点陷阱、prefers-reduced-motion（UI §10）
- [ ] i18n 基座：中/英、相对时间+悬浮绝对时间、本地化数字/字节（UI §11）
- [ ] 前端与 REST 对接约定：cursor 分页、Idempotency-Key、错误 envelope（UI §12）

### P0.6 安全/ID 基础设施 [S]
- [ ] ID 生成器：所有公开 ID（`plink_`、`file_`、`fref_`、`inst_`…）随机不可枚举、禁自增（§ID 与 secret 编码规范）
- [ ] secret 生成：≥128 bit CSPRNG，base62/base64url 无填充，前缀 `aps_`（链接）/`afs_`（文件直链）（§ID 编码规范）
- [ ] secret/password 仅存 hash，**常量时间比对**（§14 / §15）
- [ ] argon2id 用于 password 存储（§14）
- [ ] 审计日志写入框架：事件 + 字段（event_id/tenant_id/actor_type/...，§15），异步入队落库（§19.7）

---

## 阶段 P1 — MVP-0：DNS + Tunnel 核心（Phase 1，§21 MVP-0）

目标：Wildcard 子域名 + Preview Agent + WebSocket tunnel + PDF/图片/文本预览 + 短期链接 + 撤销 + 访问日志 + 最小后台 + 三类 Agent 接入。

### P1.1 认证与账户（§25） [B]
- [ ] `POST /auth/register`：原子创建 User+Tenant(lite/new)+Membership(owner)+Quota（§25）
- [ ] `POST /auth/login`：argon2id 校验 + IP/账号限流防撞库（§25）
- [ ] `POST /auth/logout`、`GET /auth/session`（返回 user + tenants + role）（§25）
- [ ] 邮箱验证：`verify-email` / `resend-verification`，写 `email_verified_at`（§25）
- [ ] 密码重置：`forgot-password`（防枚举，恒 200）/ `reset-password`（§25）
- [ ] OAuth：`oauth/{provider}/start|callback`，对齐 `auth_provider=oauth`（§25）
- [ ] 会话：httpOnly+Secure+SameSite cookie 或 access/refresh token + CSRF 防护（§25）
- [ ] 后台 API 鉴权链：登录态 → 解析 Membership → 校验 role（§2 RBAC）

### P1.2 实例供给与凭证（§26） [B]
- [ ] `POST /instances`：校验 `instance_limit`，签发 `agentToken`+`instanceApiKey`（明文仅一次，存 hash），subdomain 唯一+保留字过滤（§26）
- [ ] `GET /instances`（列表/过滤）、`GET /instances/{id}`（连接健康/session/协议版本/allowlist）（§26）
- [ ] `DELETE /instances/{id}`（级联撤销链接）（§26）
- [ ] `POST /instances/{id}/rotate-credentials`、`/revoke-token`（撤销即断连，owner/admin，写审计）（§凭证生命周期 / §26）
- [ ] `POST /instances/{id}/allowlist-change-request`（只生成指令，需本机确认）（§6.3 / §26）
- [ ] 审计：`instance.created`（§15）

### P1.3 Preview Agent（客户端 Go 二进制）（§6） [B]
- [ ] `apage-agent init`（--instance/--agent-type/--workspace）与 `start --token`（§6.1/6.2）
- [ ] allowlist 目录限制 + 仅监听 127.0.0.1，可加本机随机 bearer（§统一 API 约定 / §6.3）
- [ ] 路径校验 7 步：绝对化→Unicode/大小写规范化→realpath→allowlist 内→fstat 防 TOCTOU→拒非普通文件→大小限制（§6.3）
- [ ] 禁止项：home 全目录/AGENTS.md/MEMORY.md/credentials/路径穿越/符号链接逃逸/隐藏文件/可执行预览（§6.3）
- [ ] 本机文件注册 API `POST 127.0.0.1/local/v1/files/register` → 返回 `fileRef`+metadata，原始路径不上传（§6.4）
- [ ] 本地 `fileRef → canonical path` 映射，重启可恢复，过期清理（§File Ref 规则）
- [ ] 安装完整性：install.sh + 二进制发布 SHA256 + 签名(minisign/cosign)，校验失败中止；版本化 URL（§6.1）
- [ ] 自动更新（校验签名后落地）+ 强制最低版本（§6.1）

### P1.4 Tunnel 协议与 Gateway（§7 / §19.4） [B]
- [ ] Agent 主动出站连接：WebSocket over TLS（或 HTTP/2 long-lived）（§7）
- [ ] 连接认证：agent_token + instance_id + device_fingerprint + rotating session key（§7）
- [ ] 握手协商：connect 帧带 protocolVersion/agentVersion/capabilities；低于下限拒绝；能力交集协商（§7 / P1-5）
- [ ] `session.accepted`：sessionId/maxConcurrentStreams/maxChunkBytes/idleTimeout（§7）
- [ ] 心跳：ping 15s / offline timeout 45s（§7）
- [ ] 控制帧 JSON + 二进制内容帧；每请求带 requestId；每 stream 可 cancel（§7）
- [ ] **Backpressure**：Gateway 向 Agent 传递背压，避免大文件占满内存（§7）
- [ ] 限流：单实例/单租户/单链接并发（§7 / §19.6）
- [ ] tunnel 错误码：FILE_NOT_FOUND/FILE_EXPIRED/ACCESS_DENIED/FILE_TOO_LARGE/UNSUPPORTED_TYPE/RANGE_NOT_SATISFIABLE/AGENT_BUSY/AGENT_OFFLINE/STREAM_CANCELLED/INTERNAL_ERROR（§7）
- [ ] Agent 连接注册表（Redis）：instance_id→gateway_id 映射，TTL < offline timeout（§19.4）
- [ ] 路由：Preview API 查 registry → 路由到对应 Gateway → 经 session 拉流（§19.4）
- [ ] 重连覆盖旧 session、多连接选最新 healthy、心跳失败删映射（§19.4）
- [ ] tunnel 内部帧：`file.metadata` / `file.stream`(range) / `file.stream.start` / 二进制 chunk / `file.stream.end`（§8）
- [ ] 审计：`agent.connected` / `agent.disconnected`（§15）

### P1.5 Preview Link 数据面 API（§8） [B]
- [ ] `POST /preview-links`（mode=tunnel）：入参 fileRef/expiresInSeconds/displayName/accessPolicy，带 Idempotency-Key（§8）
- [ ] 响应 URL 规则：`/p/{plink}/{aps_secret}`，secret 仅 path segment，仅存 hash（§8）
- [ ] 日志对 secret path segment 脱敏；跳第三方 `Referrer-Policy: no-referrer`（§8）
- [ ] `POST /preview-links/{id}/revoke`（§14），撤销 ≤5s 生效（失效 Redis cache，§19.7 / SLO §18）
- [ ] `GET /preview-links`（过滤 status/instanceId/mode，cursor，items 不含 secret）（§14 列表）
- [ ] 三层 expires 裁剪：创建时若超过底层 file/file_ref 剩余寿命则裁剪并返回实际 expiresAt（§11）

### P1.6 权限模型（access_policy）（§14） [B]
- [ ] 策略类型：public_token / password / account / ip_allowlist / single_use / download_disabled（§14）
- [ ] access_policy schema：allowDownload/ipAllowlist/maxViews/singleUse/password{enabled,hash,attemptLimit}/account{required,allowedTenantIds,allowedUserIds}（§14）
- [ ] **强一致路径**：single_use(=maxViews 1)/maxViews 放行前 Redis Lua 原子消费，超限拒绝，禁异步补判（§14 / P0-2）
- [ ] **最终一致路径**：view_count/last_accessed_at 先写 Redis 异步 flush（§14 / §19.7）
- [ ] password：argon2id、attemptLimit 限流、常量时间比对（§14）
- [ ] ip_allowlist：仅信任自家 Edge 注入的真实 IP（§14）

### P1.7 预览运行时端点（访客面，§30） [B]
- [ ] `GET /p/{linkId}/{secret}`：8 步准入判定（secret 比对→三层 expires→revoked/frozen→single_use/maxViews 原子消费→access_policy→放行→异步计数→分类型 CSP）（§30）
- [ ] `POST /p/{linkId}/{secret}/unlock`：密码校验 + 下发 scoped cookie，错误不区分密码错/链接不存在（§30）
- [ ] account 准入：未登录跳登录，已登录校验 allowedUserIds/allowedTenantIds（§30）
- [ ] tunnel 放行经 Gateway 拉流；range 请求支持（§7/§30）
- [ ] 失效响应：过期/撤销/冻结 410，不存在/secret 不匹配 404，统一脱敏文案（§30）
- [ ] 审计：`preview_link.created` / `accessed` / `denied` / `revoked`（§15）

### P1.8 文件预览渲染（MVP 类型）（§13） [B][F]
- [ ] PDF：浏览器原生预览（CSP 放开 object/frame 或自托管 PDF.js）（§13 / §15）
- [ ] PNG/JPEG/WebP/GIF：图片预览（§13）
- [ ] TXT/MD/JSON/CSV/LOG：服务端渲染为安全 HTML 文本预览（§13）
- [ ] HTML：默认下载或 sandbox iframe（需租户显式开启）（§13）
- [ ] **分类型 CSP** 落地：图片/文本/PDF/HTML 各自 CSP，统一保留 `frame-ancestors 'none'` + nosniff（§15 / P2-3）
- [ ] 下载响应头：`Content-Disposition` + `Accept-Ranges: bytes`（§15）
- [ ] 安全响应头基线：X-Content-Type-Options/CSP/Referrer-Policy/Cache-Control/CORP（§15）

### P1.9 DNS / 域名 / TLS（MVP：wildcard）（§5） [I]
- [ ] Wildcard DNS：`*.preview.example.com → Preview Edge`（§5 / Route53 §22.2）
- [ ] Wildcard 证书 / Caddy 自动签发（§5 / §22.2）
- [ ] 子域名分配 `<instance>.preview.example.com`（§5）

### P1.10 Agent 集成（SDK / MCP / CLI）（§16） [B]
- [ ] 工具内部流程：路径→本机注册→fileRef→平台创建 link→返回 URL（§16）
- [ ] Tool `create_preview_link`（filePath/mode/expiresInSeconds → url/expiresAt）（§16）
- [ ] OpenClaw：CLI helper + MCP tool（§16）
- [ ] Hermes：SDK adapter + CLI helper（§16）
- [ ] Custom Agent：Local HTTP API + REST + SDK（§16）
- [ ] Adapter 要求：定位输出目录/注册/创建链接/返回 URL/错误处理（文件不存在/Agent offline/过期）（§16）
- [ ] Adapter 配置：agentType/agentName/defaultMode/workspaceRoots/defaultExpiresInSeconds（§16）

### P1.11 审计日志查询（§14 列表） [B]
- [ ] `GET /audit-logs`（过滤 event/resourceType/resourceId/actorType/from/to，cursor，仅管理员，secret 脱敏）（§14）

### P1.12 前端 — 公开网站（§6） [F]
- [ ] Landing：Hero/三模式对比/服务对象/能力亮点/代码示例/安全信任/CTA（UI §6.1）
- [ ] Auth 页：注册(邮箱+密码/OAuth)/邮箱验证/登录(失败限流)/忘记密码重置；注册即建首个 Tenant(owner)（UI §6.4）
- [ ] 极简居中卡片布局（UI §6.4）

### P1.13 前端 — 租户后台 MVP 子集（§17 / UI §7） [F]
- [ ] 后台布局：左导航 + 顶栏(租户切换/plan 徽标/用量预警/用户菜单)，RBAC 控制可见（UI §7）
- [ ] 概览 Overview：Stat 卡(在线实例/活跃链接/存储/流量)、实例状态摘要、最近分享、空租户引导（UI §7.1）
- [ ] 实例 Instances：列表(status StatusDot/version/last_seen)、详情(连接健康/allowlist 只读+变更请求/危险区吊销 token)、添加实例引导安装命令（UI §7.2）
- [ ] 预览链接 Preview Links：列表(不显示 secret)、过滤、创建 Drawer(模式/有效期裁剪提示/access_policy 全字段/创建后 SecretReveal 一次性)、详情、撤销+批量撤销二次确认（UI §7.3）
- [ ] 审计日志页：表格+过滤+DateRange，secret 脱敏，cursor 分页，RBAC 子集可见（UI §7.6）
- [ ] 实时性：实例在线/链接撤销 ≤5s 反映，轮询/SSE（UI §4.5）

### P1.14 前端 — 访客预览页（§9） [F]
- [ ] 预览页骨架：极简顶条(displayName+下载按钮受 allowDownload 控制)+内容区（UI §9）
- [ ] 按类型渲染：PDF/图片/文本/MD/JSON/CSV/LOG（UI §9 / §13）
- [ ] 密码页：密码输入+错误限流提示+不泄露存在性（UI §9 / §30）
- [ ] 账户页：account 策略登录后访问（UI §9）
- [ ] 失效页：410/404/撤销/冻结统一友好文案，不暴露跨租户（UI §9）
- [ ] 完整移动适配（访客多移动端）（UI §10）
- [ ] no-referrer + 脱敏 URL + 不缓存敏感内容（UI §9 / §15）

### P1.15 健康检查与最小可观测（§31 / §18） [B][I]
- [ ] 各服务 `GET /healthz` / `/readyz` / `/metrics`（§31）
- [ ] 核心指标：agent_online_count/tunnel_stream_latency/preview_link_access_count 等（§18）

### P1.16 MVP-0 验收（§18 验收用例） [S]
- [ ] tunnel 小文件首字节 P95 ≤ 1s（§18 SLO）
- [ ] revoke 生效 ≤ 5s（命中链接立即 410）（§18）
- [ ] single_use/maxViews 并发压测访问次数严格 ≤ 上限（§18 / P0-2）
- [ ] Tunnel online detection delay ≤ 45s（§18）

---

## 阶段 P2 — MVP-1：DNS + Tunnel + Cloud（§21 MVP-1） [B]

目标：Cloud upload + S3/R2 + 预签名上传 + 病毒扫描 + 文件过期清理 + 基础计量。

### P2.1 Cloud 存储（§11）
- [ ] 对象存储接入（S3/R2/MinIO/OSS/COS），key 规范 `tenant/instance/file_id/{original,preview.pdf,thumb.webp}`（§11）
- [ ] 文件状态机：created/uploading/uploaded/scanning/rejected/converting/ready/failed/expired/deleted（§11）
- [ ] 三层 expires 优先级落地（most restrictive，cloud file 删除级联失效链接）（§11 / P1-2）

### P2.2 Cloud 上传 API（§12）
- [ ] `POST /files`（multipart 直传，≤ `DIRECT_UPLOAD_MAX_BYTES` 默认 8MiB，超出 413）（§12）
- [ ] 初始 status 永不为 ready（返回 scanning/pending）（§12 / P0-1）
- [ ] `POST /uploads/presign`（校验配额/大小/MIME allowlist，uploadUrl 15min 过期）（§12）
- [ ] `POST /uploads/{fileId}/complete`（etag/size/sha256）（§12）
- [ ] `GET /files/{fileId}` 状态查询、`GET /files` 列表（§12 / §14）

### P2.3 Cloud 预览链接（§12）
- [ ] `POST /preview-links`（mode=cloud，仅 status=ready 可创建；rejected/failed/expired/deleted 明确报错）（§12）
- [ ] 同 file 多链接、各自独立撤销/密码/过期（§12）
- [ ] cloud 放行优先返回对象存储 signed URL（§19.3 / §19.6）

### P2.4 异步处理（worker，§10 / §19.3）
- [ ] 病毒/MIME 扫描 worker（§10），rejected 流转 + 审计 `file.scanned`/`file.rejected`（§15）
- [ ] 任务队列 Asynq/Redis（§22.2），扫描/转换/缩略图独立 worker（§19.3）
- [ ] MIME sniffing（§15）

### P2.5 过期与删除（§11）
- [ ] 过期任务 ≥ 每小时扫描一次，清理延迟 P95 ≤ 2h（§11 / §18 SLO）
- [ ] 删除 File 删除 original/preview/thumb 所有对象（§11）
- [ ] 对象删除失败重试 + tombstone（§11）
- [ ] 撤销链接不删 Cloud 文件（§11）
- [ ] 对象存储 lifecycle policy 兜底（§19.3）

### P2.6 计量与配额（§2 Quota / §29）
- [ ] Usage 计数：storage/tunnel_egress/cloud_egress/conversions（Redis 缓冲异步 flush，硬上限强一致，§2 用量校验点）
- [ ] presign/上传前校验 storage 与文件大小（§2 / §12）
- [ ] `GET /usage` + `GET /usage/timeseries` + `GET /billing`（§29）
- [ ] egress 接近上限按 §19.6 限流降级

### P2.7 前端 — Cloud 文件与用量（UI §7.4 / §7.7）
- [ ] Cloud 文件 Files 页：列表/状态可视化(scanning/converting 进度,rejected/failed 原因)/上传(直传≤8MiB 或 presign,进度,轮询至 ready)/删除二次确认+输入文件名/级联失效提示/空态（UI §7.4）
- [ ] 创建链接 Drawer 支持选 ready 的 cloud 文件，非 ready 不可选（UI §7.3）
- [ ] 用量与计费页：各维度 已用 vs 上限进度条 + 趋势图 + 计费信息 + 升级入口；RBAC(owner 计费/admin 用量)（UI §7.7）
- [ ] 配额预警 Banner（UI §7.1）

### P2.8 计费模型（§20）
- [ ] 套餐枚举：Lite/Starter/Pro/Team 与额度对齐（§20）
- [ ] Lite 边界：仅平台二级域名、链接/文件 ≤24h、无自定义域名/SSO/SLA、超额提示升级不自动扣费、跑在 Single-Box（§20）
- [ ] 超额处理：提示升级或拒绝，不静默产生费用（§2 / §20）

---

## 阶段 P3 — V1 与生产化（§21 暂缓项 / §22.4 Phase 2–3）

### P3.1 高级转换（§13 V1） [B]
> **范围调整**：APAGE 为只读预览产品，不支持在浏览器内编辑 office；故 **office 文档转换(DOCX/PPTX/XLSX → LibreOffice)移出范围**,转换套件已移除。office 类型不接受上传(MIME 白名单拒绝)。
- [x] ~~DOCX/PPTX/XLSX → LibreOffice 隔离容器转 PDF~~ —— **移出范围(仅浏览,不编辑 office)**
- [ ] CSV 表格预览、Markdown→安全 HTML、代码 syntax highlight（§13）
- [x] ~~转换 worker / `file.converted` 审计~~ —— **移出范围(已删除转换套件)**
- [ ] ready SLO：PDF/image/text P95 ≤ 10s after upload（§18）

### P3.2 自定义域名（§5 / §28） [B][F]
- [ ] `POST/GET/DELETE /custom-domains` + `/verify`（§28）
- [ ] 验证流程：TXT 所有权 + CNAME + ACME 自动签证书 + 定期检查（§5）
- [ ] 配额 `custom_domain_limit` 校验（§2 / §28）
- [ ] 审计 `custom_domain.verified` / `failed`（§15）
- [ ] 前端域名向导(分步 TXT/CNAME CopyField/检查 DNS/失败诊断期望 vs 实测/续期状态)（UI §7.5）

### P3.3 滥用治理与内容安全（§15.5） [S][B][F]
- [ ] 域名隔离：HTML/SVG 仅在 `render.preview.example.com` 渲染，与控制面物理隔离（§13 / §15.5）
- [ ] 渲染隔离：独立渲染域名 + sandbox iframe(无 allow-scripts/allow-same-origin) + 严格 CSP + SVG 转安全图/文本（§13）
- [ ] 链接创建限流 + 信任分级（trust_level new/basic/trusted），新租户冷启动低信任（§15.5）
- [ ] 主动扫描：钓鱼特征/恶意 hash/可疑 HTML + Safe Browsing/URLhaus 黑名单比对，命中自动冻结（§15.5）
- [ ] 举报端点 `POST /public/abuse-reports` + 预览页举报入口（§30 / §15.5 / UI §9）
- [ ] takedown/DMCA 受理 + 处置 SLA(高危 1h/一般 24h) + CSAM 法律上报（§15.5）
- [ ] 处置分级：冻结链接→冻结实例→冻结租户→永久封禁（§15.5）
- [ ] 新增审计事件：abuse.reported/flagged_by_scanner/blacklist_hit/link.frozen/instance.frozen/tenant.suspended/takedown.received/actioned（§15.5）
- [ ] 前端管理后台滥用与处置页（工单队列/处置面板/申诉/不渲染不可信内容）（UI §8.3）

### P3.4 合规与数据治理（§15.6） [S][B]
- [ ] 数据驻留：租户主 Region 固定，文件/元数据/审计/备份同 Region；可选 EU-only（§15.6 / §19.5）
- [ ] 删除权（GDPR/CCPA）：删除原文件/衍生产物/preview_link/file_ref 映射 + tombstone + 删除确认 + 审计（§15.6）
- [ ] 数据处理透明度：三种数据流向披露、遥测采集范围、子处理方清单（§15.6 / §24）
- [ ] 审计日志保留 90 天可调 + 到期匿名化/清除（§11 / §15.6）
- [ ] 前端设置页数据与合规：region 展示 + 发起删除请求二次确认（UI §7.9）

### P3.5 成员与权限（§27） [B][F]
- [ ] `GET/POST/PATCH/DELETE /members` + invite/accept（§27）
- [ ] 至少保留一个 owner 校验；角色越权校验（§27）
- [ ] 前端成员页：列表/邀请/角色变更/移除二次确认/权限说明表（UI §7.8）

### P3.6 租户设置（UI §7.9） [F]
- [ ] 租户资料(name/默认过期/默认模式)、安全(instance_api_key/agent_token 创建轮换撤销 SecretReveal)、通知渠道、危险区删除租户强确认（UI §7.9）

### P3.7 管理后台 Admin Console（UI §8） [F][B]
- [ ] 独立域名 `admin.example.com` + 独立鉴权 + 强制 SSO+MFA + 公网隔离/IP 白名单（UI §8 / §5 信息架构）
- [ ] 平台概览：全局 Stat + SLO 实时面板 + 告警列表（UI §8.1 / §18）
- [ ] 租户管理：列表/详情/运营操作(trust_level/配额/暂停恢复/冻结)全二次确认+审计；仅元数据不看明文（UI §8.2）
- [ ] 系统健康：组件状态/Gateway 视图/队列/存储/容量对照/故障指引（UI §8.4 / §19.8/19.9）
- [ ] 全局审计：跨租户检索 + 管理后台自身操作审计 + 留存策略（UI §8.5）
- [ ] 域名与证书运维：验证/签发/续期/重试 + render 域名声誉监控（UI §8.6）

### P3.8 部署演进（§22.4） [I]
- [ ] Phase 2 迁移 AWS Lean Production：ECS Fargate + ALB + RDS + ElastiCache + S3 + SQS + Route53/ACM + CloudWatch（§22.3 方案 B / §22.4）
- [ ] IaC（Terraform/CDK）（§22.3）
- [ ] Phase 3 高并发：Gateway/API 多副本 + Redis Cluster + RDS read replica + CloudFront + 多 AZ + 按租户限流隔离（§22.4 / §19）

### P3.9 高并发与限流隔离（§19.6） [B][I]
- [ ] 限流维度：tenant/instance/link/source_ip/agent session/file_id（§19.6）
- [ ] 限制项：在线 Agent 数/并发 stream/链接 RPS+view/egress Mbps/转换并发/单文件大小/单 stream 时长（§19.6）
- [ ] 降级：429 / Agent busy 503+Retry-After / signed URL 优先 / 队列拥堵给 pending / 大文件提示切 Cloud（§19.6）
- [ ] 多 Region 策略（高阶）：Active-Active Edge、就近连接、主 Region 固定、region hint（§19.5）

### P3.10 可观测性、SLO 与压测（§18） [I][S]
- [ ] 全部关键指标 + SLO 监控 + 告警项（Gateway error>1%/reconnect storm/队列 backlog/scanner 不可用/删除重试 backlog/单租户流量异常）（§18）
- [ ] 验收压测绑定容量数字（§18 / §19.8）：负载基线、连接规模收敛、关键路径用例、扩容触发验证
- [ ] 故障注入演练（§19.9）：Gateway 重启/Redis 抖动/PG 只读/对象存储异常的降级行为
- [ ] 扩容指标接入自动扩缩（Gateway 连接 70%/egress 70%/P95/Redis/队列/PG 连接池 80%）（§19.8）

---

## 横切关注点（贯穿所有阶段）

### 安全清单（§15 必须实现） [S]
- [ ] 短期 token / 高熵 secret(≥128bit) / secret 仅 hash / URL 不带 query secret / 日志脱敏 secret
- [ ] 链接可撤销 / 文件大小限制 / 租户级限流 / MIME sniffing
- [ ] 路径规范化 / 禁路径穿越 / 禁符号链接逃逸
- [ ] 上传病毒扫描 / Office 转换隔离容器 / HTML sandbox / 审计日志

### 凭证生命周期（§凭证生命周期与泄露处置） [S]
- [ ] instance_api_key / agent_token 轮换 + 宽限期 + 撤销即断连
- [ ] 泄露 blast radius runbook；device_fingerprint 仅检测不追踪、隐私披露

### 数据库与缓存策略（§19.7） [B]
- [ ] 高频访问计数不同步写主表；链接访问先 Redis cache miss 再 DB
- [ ] view_count 异步 flush；maxViews/single_use 同步原子；撤销立即失效 Redis cache；审计异步落库
- [ ] Redis 用途：session registry / link cache / 限流计数 / view_count 缓冲 / idempotency key（不存长期关键数据）

### 产品边界文案（§24） [F]
- [ ] 对外明确区分 DNS-only / Tunnel relay / Cloud 三种数据流向，UI 安全文案统一口径（§24 / UI §11）

### 响应式与无障碍（UI §10） [F]
- [ ] 营销页移动优先 / 后台 lg 以上侧栏常驻 md 以下抽屉化 / 管理后台桌面为主 / 预览页完整移动适配
- [ ] WCAG 2.1 AA 全量校验

---

## 与 spec 的覆盖映射（自检：是否遗漏）

| spec 章节 | 覆盖于本 checklist |
|---|---|
| §2 核心概念/实体/RBAC/Quota | P0.3, P1.1, P2.6, §32 |
| §统一 API 约定（分页/幂等/限流/错误） | P0.4 |
| §3–5 方案A/架构/DNS | P1.9, P3.2 |
| §6 Preview Agent | P1.3 |
| §7 Tunnel 协议 / §19.4 路由 | P1.4 |
| §8 Tunnel API | P1.5 |
| §9–12 Cloud 方案/存储/API | P2.1–P2.3 |
| §13 预览能力 / 分类型 CSP | P1.8, P3.1 |
| §14 权限模型 + 列表 API | P1.6, P1.11 |
| §15 安全要求 + 审计 | 横切安全清单, P0.6 |
| §15.5 滥用治理 | P3.3 |
| §15.6 合规数据治理 | P3.4 |
| §16 Agent 集成 | P1.10 |
| §17 管理后台（租户） | P1.13, P2.7, P3.5–3.6 |
| §18 可观测/SLO/压测 | P1.15, P3.10, P1.16 |
| §19 高并发部署 | P3.8, P3.9 |
| §20 计费模型 | P2.8 |
| §21 MVP 范围 | 阶段划分 |
| §22 部署方案 | P0.1–P0.2, P3.8 |
| §23 技术栈 | P0.1 |
| §24 产品边界 | 横切产品边界文案 |
| §25–33（新增控制面/运行时/基建） | P1.1–P1.2, P1.7, P2.6, P3.2/3.5, P0.3, P1.15, P0.2 |
| UI §1–4 设计语言/组件/交互 | P0.5 |
| UI §5 信息架构 | P1.12–P1.13, P3.7 |
| UI §6 公开网站 | P1.12 |
| UI §7 租户后台 | P1.13, P2.7, P3.5–3.6 |
| UI §8 管理后台 | P3.7 |
| UI §9 访客预览页 | P1.14 |
| UI §10 响应式无障碍 | 横切 |
| UI §11 i18n/文案 | P0.5, 横切产品边界 |
| UI §12 前端技术对齐 | P0.5 |
| UI §13 对应关系 | 本映射表 |
