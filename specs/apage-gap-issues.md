# APAGE 缺口 Issue 清单

> 来源:对照 [implementation-checklist](apage-implementation-checklist.md) 与
> [IMPLEMENTATION-STATUS.md](../IMPLEMENTATION-STATUS.md) 对实际代码做的完整审阅(2026-06-22)。
> 重点是**文档标 ✅ 但代码未达标**的项。`go build`/`go test` 均通过。
>
> 优先级:**P0** 安全/正确性阻断 · **P1** 标称完成但实为 stub/缺失 · **P2** 生产纵深 · **P3** 前端 · **DOC** 文档一致性。

---

## P0 — 安全 / 正确性(优先修)

> **进度(branch `fix/p0-security`)**:7 项全部 ✅ 已修,通过 `go build`/`go vet`/`go test`。逐项见各条 **状态**。

### APAGE-001 · 扫描器只信客户端声明的 MIME,可被改名绕过
- **状态**:✅ 已修。`worker.handleScan` 现读取对象首 512 字节做 `http.DetectContentType` 嗅探,声明 MIME 与实际内容不符即 `rejected`(读失败重排重试,fail-closed);新增 `internal/worker/scan_test.go` 覆盖改名绕过用例。
- **现状**:`scan()` 仅用 `f.MimeType`(上传时客户端/multipart header 提供)比对白名单,全仓无 `http.DetectContentType` 字节嗅探。改名 `.exe` 谎报 `application/pdf` 即可流转到 `ready`。
- **证据**:`internal/worker/worker.go:159-169`、`internal/api/files.go:58,152`
- **修复**:落盘后读取首部字节做真实嗅探,sniff 结果与声明 MIME 不一致即 `rejected`;为 ClamAV/Safe Browsing 预留接口但默认开启嗅探。
- **验收**:谎报 MIME 的文件被置 `rejected` 并写 `file.rejected` 审计。

### APAGE-002 · 滥用治理不可执行(冻结=死代码)
- **状态**:✅ 已修(租户管理员范围)。新增 `internal/api/moderation.go`:freeze/unfreeze link、freeze/unfreeze instance(owner/admin),写 `link.frozen`/`instance.frozen` 审计、失效 Redis 缓存、冻结实例即断 tunnel;store 的 Freeze* 改为租户隔离 + 返回是否命中。**仍待**:平台级处置分级(冻结租户/封禁)属 Admin 平面([[APAGE-037]])。
- **现状**:`FreezeLink`、`FreezeInstance` 零调用方;`SuspendTenant`/封禁方法不存在;runtime 只**读** `frozen_at` 却没有任何路径能写入。处置分级(链接→实例→租户→封禁)无法落地。
- **证据**:`internal/store/links.go:108`、`internal/store/instances.go:148`(无 caller);`internal/api/runtime.go:161-169`
- **修复**:补冻结端点(admin 或内部),接 audit 事件 `link.frozen`/`instance.frozen`/`tenant.suspended`;撤销同款 Redis 失效。
- **验收**:可冻结一条链接,≤5s 访问返回 410 并留审计。

### APAGE-003 · 信任分级与链接创建限流缺失
- **状态**:✅ 已修。`handleCreateLink` 现按租户限流 `linkcreate:<tenant>`,额度随 trust_level 变化(new 20 / basic 60 / trusted 120 每分钟);限流在幂等之外、冷启动新租户最低额度。
- **现状**:`TrustLevel` 注册时写 `"new"` 后**从不读取**;`handleCreateLink` 无任何 `s.limit()` 限流(仅 auth/abuse/unlock 有)。
- **证据**:`internal/api/auth.go:56`、`internal/api/links.go:32`
- **修复**:链接创建按 tenant/IP 限流;按 trust_level 冷启动收紧额度。

### APAGE-004 · 渲染域名隔离形同虚设(HTML/SVG 同域服务)
- **状态**:✅ 已修(完整 sandbox 渲染,非仅下载)。`internal/api/render.go`:① 控制面(preview 子域,带 cookie)先过 ip/account/password 门禁,再签发**短时不可猜的 grant**(HMAC(link|10min 窗口|SessionSecret))302 跳到 render 域;② render 域为**无 cookie 独立 origin**,凭 grant 授权,出 wrapper 页:HTML 用 `<iframe sandbox>`(无 allow-scripts/allow-same-origin)、SVG 降级为 `<img>`,严格 `script-src 'none'; sandbox` CSP;③ 新增 `/p/{id}/{secret}/raw` 子资源端点,校验 grant + 复检 ip + **在此消费 view**(wrapper 不消费,maxViews 仍每次一计)再流式输出;④ Caddyfile render 块补 `X-Forwarded-For` 供 raw 复检 ip。`RenderDomain` 未配置时退化为同源 sandbox 渲染(dev)。新增 `render_test.go` 覆盖 grant 轮换/绑定、域判定、wrapper sandbox。**剩余(非 P0)**:render 物理独立服务/网络隔离属部署演进([[APAGE-037]] / §22.4);grant 为 link 维度(≤20min 可分享),secret 仍是主凭据。
- **现状**:`cfg.RenderDomain` 从 env 读入但路由中零引用;HTML/SVG 与控制面走同一 `/p/{linkId}/{secret}`,无 Host 校验、无 sandbox iframe 包裹、无 SVG→安全图/文本降级。
- **证据**:`internal/config/config.go:17,72`、`internal/api/runtime.go:21,84`、`internal/api/csp.go:46-49`
- **修复**:HTML/SVG 仅在独立 render 域渲染 + sandbox iframe(无 allow-scripts/same-origin);SVG 转安全图或纯文本。

### APAGE-005 · 控制面无 CSRF 防护
- **状态**:✅ 已修。双提交 CSRF:登录/注册/`/auth/session` 下发非 HttpOnly `apage_csrf` cookie;`csrfGuard` 中间件对 session 认证的非安全方法校验 `X-CSRF-Token`==cookie(常量时间;Bearer 实例密钥豁免);前端 `web/lib/api.ts` 在非 GET 请求自动回带该头。
- **现状**:会话用 HttpOnly+Secure+SameSite=Lax cookie,但无 CSRF token;Lax 不能阻止顶层跨站 POST。
- **证据**:`internal/api/auth.go:223-226`(全仓无 `csrf`)
- **修复**:双提交 cookie 或 SameSite=Strict + CSRF token。

### APAGE-006 · Agent 本机注册接口无鉴权
- **状态**:✅ 已修。`init` 生成随机 `LocalToken` 存配置(0600)并打印;loopback `register` 用常量时间比对校验 `Authorization: Bearer`;`share` 自动携带。
- **现状**:`handleRegister` 信任任何 127.0.0.1 调用方,无可选本机 bearer。任意本机进程可注册文件生成 fileRef。
- **证据**:`internal/agent/local.go:52-91`、`cmd/agent/main.go:109`
- **修复**:启动随机 bearer,register/文件接口校验 `Authorization`。

### APAGE-007 · 上传 complete 丢弃 etag/sha256,不校验完整性
- **状态**:✅ 已修。新增 `objstore.Stat`;`handleCompleteUpload` 先 `Stat` 确认对象已落地(否则 409),客户端给了 etag 则比对(不符 409),并以**对象实际大小**为准入库,忽略伪造 size。注:sha256 深校验需下载对象,暂未做(已注释说明)。
- **现状**:`complete` 解析 `etag/size/sha256` 但只用了 `size`,不与对象核对。
- **证据**:`internal/api/files.go:186-227`
- **修复**:对存储对象做 HEAD 取 etag/size 比对,可选 sha256 校验,不符 `failed`。

---

## P1 — 标称完成但实为 stub / 缺失

> **进度(branch `fix/p0-security`)**:已修 **012 / 013 / 014 / 015 / 016 / 017 / 018 / 020**(✅),**019 / 022 部分**(🟡),并附带修 P2 的 [[APAGE-035]](lite 文件过期裁剪)。均通过 `go build`/`go vet`/`go test -race`(新增 agent range、gateway version、path size、billing 等测试 + migration 0002)。**010 背压(credit 流控)、011 registry 路由、023 DNS(CNAME+周期 recheck)亦已修**,并附带修 [[APAGE-033]](registry TTL)。**021 OAuth 亦已修**(config-gated 真实流程,配 provider 凭据即生效)。**024 Office 转换已移出范围**(产品决定:仅浏览、不支持浏览器编辑,移除转换套件)。**仅剩 1 项需外部基建**:023 ACME 签发(需真实域名 + ACME client)。

### APAGE-010 · Tunnel 背压是假的
- **状态**:✅ 已修(credit 流控)。tunnel 协议加 `flow` 帧 + `FlowWindow=16`:agent 初始 16 credits,**仅在持有 credit 时才发 chunk**,gateway 每relay 一个 chunk 给访客就回 `flow(+1)`。数学上 dataCh 占用恒 ≤ window,gateway readLoop 永不阻塞(消除 head-of-line),内存有界,慢访客→agent 阻塞在 `awaitCredit`→停读磁盘(真背压)。**能力位 `flowControl` 兜底**:gateway 未授予时 agent 退化为自由发送,新/旧混部不会死锁。新增 `flow_test.go`(含 race)。
- **现状**:仅 cap=8 的 Go channel,agent 全速读文件推二进制帧,无 window/credit 流控;大文件仍可撑爆 gateway 内存。注释"bounded buffer applies backpressure"系一厢情愿。
- **证据**:`internal/gateway/session.go:56`、`internal/agent/tunnel.go:163-179`
- **修复**:加 credit/window 协议,agent 按 gateway 反馈节流。

### APAGE-011 · 预览路由未用 registry(写死单 gateway)
- **状态**:✅ 已修(非回归)。gateway 注册时把自身可达 URL(`GATEWAY_ADVERTISE_URL`,默认 = `GATEWAY_INTERNAL_URL`)写入 registry;API 预览前 `resolveGatewayURL` 查 registry 拿到 owning gateway URL 路由,registry miss 时回退配置 URL(单 box 行为不变,为多 gateway 铺路)。顺带修 [[APAGE-033]]:`RegisterAgent` 现在注册即设 TTL(原先首次 TouchAgent 前无 TTL)。
- **现状**:预览走静态 `GATEWAY_INTERNAL_URL`,`LookupAgent` 仅用于状态展示;多 gateway 路由未接。
- **证据**:`internal/api/runtime.go:91`、`cmd/api/main.go:67`、`internal/api/instances.go:117`
- **修复**:预览前查 registry 解析 owning gateway 再拉流。

### APAGE-012 · 并发限流 / AGENT_BUSY 未强制
- **状态**:✅ 已修。`Session.tryAddStream` 在 `MaxConcurrentStreams` 满时原子拒绝;gateway `handleInternalStream` 返回 503 `AGENT_BUSY` + `Retry-After`(spec §7/§19.6 降级)。
- **现状**:`MaxConcurrentStreams` 只在握手广播,从不检查;`AGENT_BUSY` 永不触发;无 per-instance/tenant/link 并发闸。
- **证据**:`internal/gateway/server.go:115`
- **修复**:加 stream 信号量,超限 503 + Retry-After / AGENT_BUSY。

### APAGE-013 · Range 请求被 agent 忽略
- **状态**:✅ 已修。agent `handleStream` 新增 `parseByteRange`,支持 `bytes=a-b`/`a-`/`-N`,回 206 + `Content-Range` + 正确 `Content-Length`,seek 后只传请求区间;越界 start 回 `RANGE_NOT_SATISFIABLE`(→416);畸形 Range 按 RFC 7233 退化为 200 全量。超大文件回 `FILE_TOO_LARGE`。新增 `range_test.go`。
- **现状**:gateway 转发 `Range`,但 agent `handleStream` 永远开整文件、回 `Status:200` + 全量 Content-Length,不解析 range、不回 206/Content-Range/RANGE_NOT_SATISFIABLE。
- **证据**:`internal/agent/tunnel.go:127-180`
- **修复**:agent 解析 range 做分段,或显式对大文件拒绝并提示走 cloud。

### APAGE-014 · Cloud 预览代理字节而非 signed URL 重定向
- **状态**:✅ 已修(opt-in,非回归)。新增 `S3_PUBLIC_ENDPOINT`(浏览器可达端点);配置后 `serveCloud` 对 **image/pdf 且无 view 上限**的 cloud 链接 302 到 `PresignGet` 签名 URL(对象存储直发字节,§19.3),否则继续代理。有 single_use/maxViews 的链接**不重定向**(避免 15min 签名 URL 被重放绕过上限),active 内容也不重定向(须留在 render 域 CSP 下)。presign 客户端用 public 端点签名(顺带修复 presign 上传的浏览器可达性)。compose 加 `S3_PUBLIC_ENDPOINT=http://localhost:9100`。
- **现状**:`serveCloud` 经 API `store.Get`+`ServeContent` 代理全部字节;`PresignGet` 是死代码。违反 §19.3"API 不代理大文件"。
- **证据**:`internal/api/runtime.go:209-230`、`internal/objstore/objstore.go:86`
- **修复**:cloud 放行优先 302 到 `PresignGet` 签名 URL。

### APAGE-015 · 计量只有 storage;egress/conversion 从不计;无 timeseries/billing
- **状态**:✅ 已修。egress 经 `countingWriter` 统计实发字节 → `redisx.AddUsage`(Redis 缓冲)→ worker 每 60s `DrainUsage`(GETDEL,精确一次)flush 到 `quotas` + 每日快照(migration 0002 `usage_daily`);conversion 在 worker 转换完成时计数。新增 `GET /usage/timeseries`(管理员,趋势)与 `GET /billing`(owner:套餐/价格/用量/升级项,`autoCharge:false`)。
- **现状**:仅 storage 计量,且为同步 SQL UPDATE 而非 Redis 缓冲异步 flush;`tunnel_egress_used`/`cloud_egress_used`/`conversion_used` 永不自增;`/usage/timeseries`、`/billing` 路由不存在;egress 临界降级缺失。
- **证据**:`internal/store/accounts.go:233-238`、`internal/api/server.go:102`、`internal/api/usage.go`
- **修复**:补 `AddEgress`/`AddConversion`(Redis 缓冲 + 异步 flush + 硬上限强一致);补两个端点;egress 临界限流。

### APAGE-016 · 审计同步 INSERT(非异步)
- **状态**:✅ 已修。新增 `Server.audit` 将审计入队 Redis `audit` 队列(快 LPUSH,替代请求路径上的 11 列 INSERT),worker 新增 `audit` 消费者落库;入队失败回退同步写,绝不丢审计。API 全部 21 处 `WriteAudit` 改为 `s.audit`。
- **现状**:`WriteAudit` 在请求路径上做阻塞 INSERT,注释自承认走捷径;违反 P0.6/§19.7 异步入队。
- **证据**:`internal/store/audit.go:13-22`
- **修复**:审计入队 + worker 落库。

### APAGE-017 · 幂等不检测同 key 异 body(无 409),覆盖面窄
- **状态**:✅ 已修。`idempotent` 现存 `{请求体哈希, 响应}`,同 key 异 body 返回 409;作用域加 instance 隔离(`dataScope.idemScope()`);覆盖从 3 扩到 5 个 create 接口(+create-domain、+invite-member)。revoke/delete 等天然幂等无需 key。
- **现状**:无 body 哈希,换 body 复用同 key 静默返回旧响应;作用域只 tenant+endpoint(缺 instance);仅接 presign/create-link/create-instance 3 个写接口,revoke/complete/delete/member 等无幂等。`idempotency_keys` 表也缺失(改放 Redis)。
- **证据**:`internal/api/middleware.go:209-234`、`internal/store/migrations/0001_init.sql`
- **修复**:存请求 body 哈希,异 body 返回 409;扩展到全部写接口;补 DB 表或在文档承认 Redis-only。

### APAGE-018 · 7 步路径校验缺 3 步
- **状态**:✅ 已修。`ResolvePath` 加 Unicode NFC 规范化(防 NFD/NFC 同形绕过 blocklist)+ `MaxPreviewBytes`(默认 100MiB)大小限制;对已打开 fd 的 fstat 在 stream 时已存在(`tunnel.go` `file.Stat()` 复检 IsRegular)。新增 size 测试。
- **现状**:缺 (a) Unicode/NFC 规范化(仅对 basename `ToLower`)(b) 对**已打开 fd** 的 fstat 防 TOCTOU(只 Lstat 路径)(c) 文件大小限制。
- **证据**:`internal/agent/pathcheck.go:41-78`
- **修复**:补全三步;stream 时用打开的 fd 做 fstat 再服务。

### APAGE-019 · download_disabled 是空操作
- **状态**:🟡 best-effort。active 内容(html/svg)现强制 attachment 下载、不内联;passive 内容 `download_disabled` 仍为 best-effort 内联(无下载入口 + `no-store`)。真正禁下载需 DRM/水印,非代码可彻底解决;字节一旦到浏览器即可另存,spec 本身也标 best-effort。
- **现状**:`setDownloadHeaders` 两分支都 `disp="inline"`,`allowDownload=false` 不产生任何差异。
- **证据**:`internal/api/runtime.go:234-240`
- **修复**:至少不发可下载入口、设 `no-store`;cloud 用短时 signed URL 控制。

### APAGE-020 · "三层 expiry" 实为两层
- **状态**:✅ 已修。补第三层「租户计划上限」:创建链接时按 plan 裁剪(`planMaxLinkTTL`,lite≤24h),`effectiveExpiry` 继续裁 link vs backing,`now` 由 runtime 单独判过期。顺带修 [[APAGE-035]]:上传/presign 也按 plan 裁剪文件过期(`clampExpiryToPlan`)。
- **现状**:`effectiveExpiry` 只裁 link vs backing,缺 now/floor 或租户策略层;注释称三层不实。
- **证据**:`internal/api/policy.go:14-25`
- **修复**:补租户计划上限层 + 创建时拒绝过期 backing。

### APAGE-021 · OAuth 完全不存在
- **状态**:✅ 已修(config-gated,真实流程)。`internal/api/oauth.go` 手写完整 OAuth2 流程:`/auth/oauth/{provider}/start`(state cookie 防 CSRF + 跳 provider)→ `/callback`(校验 state → 换 token → 取**已验证**邮箱 → 按邮箱 login 或自动建号 `auth_provider=oauth` → 起 session → 跳 /console)。支持 GitHub/Google;`/auth/providers` 发现端点 + login 页按已配置 provider 渲染按钮。未配 client id/secret 时该 provider 自动禁用(返回 404),**绝不伪装已接通**。无 provider 凭据无法 e2e 验证,但代码真实、符合 OAuth2 规范(同 `S3_PUBLIC_ENDPOINT` 的 config-gated 模式)。
- **现状**:全仓无 `oauth` 路由/handler;`AuthProvider` 永远 `"password"`。文档 🟡"路由已描述"偏乐观,实为 ❌。
- **证据**:`internal/api/server.go:73-79`、`internal/api/auth.go:57`
- **修复**:接 provider start/callback,或在文档/UI 移除 OAuth 入口。

### APAGE-022 · Agent 安装完整性 / 自动更新 / 版本下限 缺失
- **状态**:🟡 部分修。**版本下限已落地**:gateway 握手用 `versionAtLeast` 强制 `AGENT_MIN_VERSION`,低于即 `AGENT_TOO_OLD` 拒绝(原为死配置),新增 `version_test.go`。**仍待(需发布管线)**:install.sh、二进制 SHA256/minisign 签名校验、自动更新。
- **现状**:无 `install.sh`,无 SHA256/minisign/cosign 校验,无自动更新;`AGENT_MIN_VERSION` 定义但**从不强制**(gateway 只查协议版本)。
- **证据**:`internal/config/config.go:37,84`、`internal/gateway/server.go:81`
- **修复**:发布脚本 + 签名校验 + gateway 强制最低 agentVersion。

### APAGE-023 · 自定义域名 ACME/CNAME/周期校验不实
- **状态**:🟡 大部分修(ACME 仍 TODO)。`handleVerifyDomain` 现校验 **TXT(所有权)+ CNAME(路由)**,返回 expected-vs-observed 诊断(UI §7.5);**不再假报 `cert=issued`** —— 路由确认后置 `cert=pending`(注明 ACME 签发为 `TODO(prod)`)。worker 新增 `domainRecheckLoop`(每 30min,12h stale):TXT 消失则回退 `failed` + 审计(spec §28 定期检查)。新增 `DomainsToRecheck` store 方法、`normalizeHost` + 测试。**仍待**:ACME 自动签发/续期(需真实域名 + ACME client)。
- **现状**:仅 TXT 查询真实;CNAME 展示但不校验;cert 直接置 `issued`(无 ACME client);无周期 re-verify。
- **证据**:`internal/api/domains.go:107-110`、`internal/api/health.go:32`
- **修复**:接 ACME(autocert/lego)+ CNAME 校验 + 续期 worker。

### APAGE-024 · Office 转换 —— ❌ 移出范围(产品决定:仅浏览,不在浏览器编辑)
- **状态**:✅ 已按决定处理(**不实现,移除桩代码**)。APAGE 定位只读预览、不支持在浏览器里编辑 office,故 LibreOffice 转换套件直接砍掉:删除 worker 的 `handleConvert`/`needsConversion`/`convert` 队列消费者/scan 的转换分支/转换计量,以及 quota 的 `conversion` 维度与 `/usage`、`/billing`、前端用量页/定价页里的 conversions 展示;`file.converted` 审计事件删除。office 类型本就不在上传白名单,继续 415 拒绝。DB 的 `conversion_*` / `usage_daily.conversions` 列保留为惰性(默认 0,不再读写),避免对已应用 migration 动刀。

---

## P2 — 生产纵深

> **进度(branch `fix/p0-security`)**:已修 **030 / 031 / 033 / 034 / 035 / 037 / 038**(✅)+ **036 审计保留 / 039 能力协商 / 040 测试**(🟡 大部分),**032 大部分满足**(🟡)。**剩余(均需外部/基建)**:036 数据驻留/EU(多 region)、037 企业 SSO(外部 IdP)、039 rotating session key、040 audit 分区(对已应用 migration 动刀)。

### APAGE-030 · 通用 API 限流缺失(仅 auth 有)
- **状态**:✅ 已修。新增 `dataWriteRateLimit` 中间件:数据面写接口按租户限流 300/min(只限非安全方法,读/列表放行);访客 runtime(`handlePreview` + `handlePreviewRaw`)按源 IP 限流 600/min。配合已有的 link-create(按 trust)/unlock/abuse 限流。
- `internal/api/auth.go` 曾是唯一 `RateLimit` 调用方;数据面写接口、访客 runtime 无限流。

### APAGE-031 · 对象删除无退避/墓碑/上限;无 lifecycle 兜底
- **状态**:✅ 已修。`handleDelete` 现按 `attempt` 做**封顶指数退避**(10s→10min)异步重排(不阻塞消费者),超过 `maxDeleteAttempts=6` 进 `delete:dead` 死信(墓碑);删除目标简化为仅 original(无衍生物)。新增 `S3_LIFECYCLE_DAYS` 对象存储生命周期兜底(config-gated,默认 0)。新增 `parseRetry`/`retryBackoff` 测试。
- 失败即无退避无限重排,无死信;`objstore` 无 bucket lifecycle policy。

### APAGE-032 · view_count 异步 flush 与 link 读穿缓存未实现
- **状态**:🟡 大部分已满足。view_count **已异步 flush**(`flushViewCount`→`TouchLinkAccess`);撤销/冻结立即生效(runtime 直读 DB,`InvalidateLink` 失效 Redis cache)。**未做**:link 元数据读穿缓存(populate)—— 当前直读 DB,撤销即时,属性能优化而非正确性问题,暂不引入(避免 cache 一致性 bug)。
- `internal/redisx/redisx.go` 只有 `InvalidateLink`(DEL),无 populate;link 元数据直读 DB。

### APAGE-033 · RegisterAgent 忽略 ttl 参数
- **状态**:✅ 已修(随 [[APAGE-011]])。`RegisterAgent` 现在 `HSet` 后立即 `Expire(key, ttl)`,注册即有 TTL。
- `internal/redisx/redisx.go` `HSet` 不设过期,key 在首次 `TouchAgent`(+20s)前无 TTL。

### APAGE-034 · get-instance 报错的协议版本 + 占位 allowlist
- **状态**:✅ 已修。agent 在 connect 帧上报 `allowlist`(workspace 根);gateway 把 agent 真实 `protocolVersion` + allowlist 写入 registry(`RegisterAgent` 改用 `AgentReg` 结构);`LookupAgent` 返回结构体;get-instance 现展示**真实协议版本**(离线/未知时回退 floor)与**agent 上报的 allowlist roots**,而非硬编码。
- 返回服务端 floor 当作 `protocolVersion`,allowlist 为硬编码 note,非 agent 上报值。

### APAGE-035 · Lite 套餐过期未裁剪到 24h
- `internal/api/files.go` 接受任意 `expiresInSeconds`,lite 租户可设 30 天。

### APAGE-036 · 合规大面积缺失
- **状态**:🟡 部分修。**审计保留已落地**:worker `auditRetentionLoop` 每日按 `AUDIT_RETENTION_DAYS`(默认 90)分批 `PurgeOldAudit` 清除到期审计日志(spec §11/§15.6);GDPR/CCPA 删除此前已做。**仍待(需基建/产品)**:数据驻留 / EU-only(需多 region,单一全局 `S3Region`)、数据流向/子处理方披露(静态合规文案,属产品/法务)。

### APAGE-037 · Admin 后端不存在
- **状态**:✅ 已修(平台 admin 后端 + 最小前端)。**鉴权**:独立 `platform_admins`(migration 0003)+ 密码(argon2id)+ **强制 TOTP MFA**(自实现 RFC 6238,`internal/totp`,首登强制 enroll)+ **IP allowlist** gate;独立 admin session(Redis,SameSite=Strict)。**运营端点**(`/admin/v1/*`,全部审计、仅元数据):overview、租户 list/detail、trust 变更、suspend/restore(冻结全租户链接 + 失效缓存 + 阻止建链=真有牙)、abuse 队列 list/action(接上原孤儿 `ListAbuseReports`)、跨租户 audit 检索。**前端**:`/admin` 页接上登录+MFA+overview+租户 suspend/restore/trust。bootstrap 用 `ADMIN_BOOTSTRAP_*` 播种首个 admin;Caddy 加 `admin.localhost`(转发真实 IP 给 allowlist)。新增 `totp_test`。**仍待**:企业 SSO(SAML/OIDC,需外部 IdP);独立物理服务/网络隔离(部署演进);富 admin UI 面板(系统健康/SLO/域名运维,属 [[APAGE-052]] 前端)。

### APAGE-038 · API 服务无 /metrics 与业务指标
- **状态**:✅ 已修。API 现在用上 `MetricsAddr`(独立内部监听,不暴露在公网 host),`MetricsHandler` 输出 Prometheus 文本:`apage_agent_online_count`、`apage_active_links_count`、`apage_{scan,delete,audit}_queue_depth`、`apage_preview_access_total`(原子计数器,每次预览自增)。新增 store `CountOnlineInstances`/`CountActiveLinks`。
- 仅 gateway 暴露连接指标;API `MetricsAddr` 配了不用。

### APAGE-039 · 握手未读 device_fingerprint / capabilities;无 rotating session key
- **状态**:🟡 大部分修。gateway 握手现做**能力协商**:要求 agent 支持 `file.stream`(否则 `CAPABILITY_UNSUPPORTED` 拒绝),并记录 agent×gateway 的能力交集;`device_fingerprint` 在连接日志中记录(仅检测、不持久化/不追踪,spec 口径)。新增 `negotiatedCaps` 测试。**仍待**:rotating session key(更复杂,需协议改动);`file.metadata` 请求路径仍未触发(元数据走 Postgres,该 agent 端 handler 为冗余,留作 capability 占位)。
- agent 发送但 gateway 从不校验;无能力交集协商。

### APAGE-040 · 测试覆盖薄 + audit_logs 未分区
- **状态**:🟡 部分修。测试覆盖从 3 → 6 个包(本轮新增 gateway version/caps、worker scan/range/delete、tunnel 二进制帧、api moderation/render/domains 等)。**仍待**:store/redisx/objstore 需集成测试(依赖 DB/Redis/S3);`audit_logs` 分区需对已应用 migration 动刀(建表→迁数据→切换),属 DBA 操作,单表 + 索引对当前规模可接受,留作扩容项。

---

## P3 — 前端

### APAGE-050 · 无实时层(违反撤销/在线 ≤5s)
- **状态**:✅ 已修(轮询)。新增 `usePoll` hook(标签页隐藏时暂停),接到 instances/links/files 列表(5s),实例在线/链接撤销冻结/文件 scanning→ready 均 ≤5s 反映,替换原一次性 setTimeout。
- 无 SSE/WS/轮询;仅一次性 `setTimeout`。

### APAGE-051 · Settings 为占位
- **状态**:✅ 已修。新增**租户名编辑**(后端 `PATCH /api/v1/tenant` + `UpdateTenantName`,admin)+ 表单;危险区改为明确的「删除租户数据」按钮(typed `DELETE` 二次确认,接 GDPR data-deletion)。安全卡仍指向 Instances(凭据按实例管理,合理)。
- 大部分静态卡片;无租户资料编辑;危险区无按钮。

### APAGE-052 · Admin 控制台为静态壳
- 指标硬编码 `"—"`,无 API/鉴权/交互。`web/app/admin/page.tsx:21-24`

### APAGE-053 · 缺 OAuth / 邮箱验证 / 忘记密码页
- **状态**:✅ 已修。新增 `/verify`(token 验证 + 重发)、`/forgot`(防枚举,恒成功提示)、`/reset`(token + 新密码)页,接已有 verify-email/resend/forgot/reset 后端;login 页加 OAuth 按钮(021)+ "Forgot password?" 链接。
- 无 `/verify`、`/forgot` 路由。

### APAGE-054 · 导航不按 RBAC 隐藏
- **状态**:✅ 已修。console 布局按当前租户 role 过滤导航(viewer/member/admin/owner 分级,镜像后端 RBAC):域名/审计/用量需 admin,成员/设置需 member。
- session 带 `role` 但只展示,9 个导航对所有角色全显。

### APAGE-055 · 无真正 i18n
- 硬编码英文,`lang="en"` 固定;数字未本地化。

### APAGE-056 · 组件库 ~18/28 + Modal/Drawer 无焦点陷阱
- **状态**:🟡 部分修(a11y 已补)。`useDialogA11y`:Modal/Drawer 现有**焦点陷阱 + Esc 关闭 + 焦点还原 + `role=dialog`/`aria-modal`**。**仍待**:补齐 IconButton/Tag/Tabs/Tooltip/DateRange 等组件(按页面实际需要增量加,如 DateRange 随 [[APAGE-058]] 审计)。

### APAGE-057 · Usage 页缺趋势图/计费/升级,无 RBAC 拆分
- **状态**:✅ 已修。接 `/usage/timeseries`(30 天 egress SVG 趋势图)+ `/billing`(套餐/价格/升级项,owner-only:非 owner 静默 403 隐藏 billing 卡=RBAC 拆分)+ ≥80% 配额预警 Banner + 升级 CTA(链 /pricing,`autoCharge:false`)。
- 仅进度条;无图表/计费/升级/RBAC 拆分。

### APAGE-058 · 零散功能缺失
- **状态**:🟡 大部分修。✅ 链接**批量撤销**(多选 + typed `REVOKE` 二确)、✅ **上传进度条**(XHR 真实进度,+ 列表 5s 轮询见 [[APAGE-050]])、✅ **域名失败诊断**(Check DNS 弹窗显示 TXT/CNAME 期望 vs 实测 + ✓/✗)。**仍待**:链接详情视图、上传 presign 大文件路径、审计 DateRange(需后端 from/to 过滤)。

---

## DOC — 文档一致性

### APAGE-060 · 修正 IMPLEMENTATION-STATUS.md 高估项
- 将 APAGE-010/011/012/013/014/015/016/017/018/019/020/021/022 对应条目由 ✅ 降级为 🟡/⬜,并补 `TODO(prod)` 接入点;OAuth、install 完整性由 🟡 改 ❌。文档与代码不一致本身是最大交付风险。
