# APAGE Spec 评审清单

针对 `apage-spec.md` 的改进项。按优先级排序,每项含问题、对应章节、建议动作与验收标准。

> 优先级:**P0** = 会导致实现冲突或上线即风险,必须先解决;**P1** = 重要缺口;**P2** = 完善度提升。

---

## P0 — 阻塞项

### [x] P0-1 修复 §12「上传文件」状态机矛盾
- **问题**:§12 `POST /api/v1/files`(multipart 同步上传)直接返回 `previewStatus: "ready"`,与 §11 定义的 `created→uploading→uploaded→scanning→converting→ready` 异步状态机冲突。刚上传的文件不可能立即 ready。
- **动作**:
  - 同步接口改为返回 `status: "scanning"` / `previewStatus: "pending"`,与 §12 presign complete 一致。
  - 明确 multipart 直传 vs presign 的选择阈值(如 `> N MB` 必须走 presign)。
- **验收**:任意上传路径返回的初始 status 都不为 `ready`;文档给出阈值常量。

### [x] P0-2 解决 `view_count` 异步 flush 与 `single_use`/`maxViews` 的原子性冲突
- **问题**:§14 要求 `single_use`/`maxViews` 原子消费、防并发重复;§19.7 又要求 `view_count` 先写 Redis 再异步 flush。两者并存会导致 `maxViews` 超发。
- **动作**:明确双路径——
  - `single_use`/`maxViews`:同步原子路径(Redis Lua 或 `INCR` 后判定),强一致。
  - 纯统计型 `view_count`:可异步 flush。
  - 在 §14 和 §19.7 各加一句交叉引用。
- **验收**:并发压测下 `maxViews=N` 的链接被访问次数 ≤ N。

### [x] P0-3 补齐列表类 API 与分页约定
- **问题**:§17 后台依赖「最近分享文件 / 访问日志 / 链接列表」,但 §8/§12 只有创建+单条查询,无 `GET /preview-links`、`GET /files`、`GET /audit-logs`,且全文无分页约定。
- **动作**:
  - 新增统一 list 接口与 **cursor/keyset 分页**约定(`limit` + `cursor` + 排序字段),复用已规划的 `tenant_id/created_at` 索引。
  - 列表响应统一 envelope(`items` + `nextCursor`)。
- **验收**:三类资源均有 list 接口;分页在大数据量下无偏移性能退化。

### [x] P0-4 新增「滥用治理」章节
- **问题**:`render.preview.example.com` + 自定义域名 + 任意文件公开链接 = 钓鱼/恶意分发载体,但全文无滥用治理。域名一旦被滥用会连累全体租户(被打入浏览器黑名单)。
- **动作**:新增一节,至少覆盖——
  - 滥用举报入口 + takedown/DMCA 流程。
  - 链接创建速率限制(防批量生成钓鱼链),按租户信任分级。
  - 新租户冷启动信任分级(限额、限公开能力)。
  - 接入 Safe Browsing / URLhaus 等黑名单比对。
  - 自定义域名与 render 域名隔离,避免主域名声誉受损。
- **验收**:文档给出举报→处置 SLA;高风险内容有自动/人工拦截路径。

---

## P1 — 重要缺口

### [x] P1-1 补充 User / Account 实体与 RBAC
- **问题**:§2 只有 Tenant,但 §14 `account.allowedUserIds`、§17「用户登录态」都依赖 User 概念,数据模型缺失。
- **动作**:新增 `User` 实体 + User↔Tenant 成员/角色关系(RBAC:owner/admin/member 等)。
- **验收**:账户级访问策略与后台登录态有数据模型支撑。

### [x] P1-2 定义三层 `expires_at` 优先级
- **问题**:`file_ref`、`file`、`preview_link` 各有过期时间,不一致时以谁为准未定义。
- **动作**:明确「取最小值(most restrictive wins)」;说明 file 删除时其上所有 link 级联立即失效。
- **验收**:文档有一张三层过期交互表/规则。

### [x] P1-3 在 API 示例中落实 Idempotency-Key 与限流头
- **问题**:Idempotency-Key 仅在 §统一约定 提及,所有示例未出现;429 无 API 层 `Retry-After`/`RateLimit-*` 约定。
- **动作**:
  - §8/§12 写接口示例补 `Idempotency-Key` header,并定义同键不同 body 的返回(409 vs 原结果)。
  - 统一定义 429 响应的 `Retry-After` / `RateLimit-Limit/Remaining/Reset` 头。
- **验收**:所有写接口示例含幂等键;限流响应头有统一规范。

### [x] P1-4 量化 secret 强度与比对方式
- **问题**:`ocps_`/`ocfs_` secret 仅说「≥128 bit」,未规定长度/字符集;未要求常量时间比对;`link_id` 未明确不可枚举。
- **动作**:
  - 规定 secret 编码(如 base62/base64url)与长度。
  - 要求 secret/password 比对使用常量时间。
  - 明确 `link_id`/`file_id` 为随机不可枚举,非自增。
  - 把 `oc*` 前缀换成中性前缀(当前疑似从 OpenClaw 派生,泄露内部渊源)。
- **验收**:文档给出明确编码规则;前缀与产品名一致。

### [x] P1-5 Agent 安装与升级的完整性保障
- **问题**:§6.1 `curl | sh` 无校验;无 Agent 二进制签名、自动更新、tunnel 协议版本协商(仅一次性 `session.accepted`)与 min-version 兼容策略。
- **动作**:
  - 安装脚本提供 checksum/签名校验步骤。
  - Agent 二进制签名 + 自动更新 + 版本下限拒绝。
  - tunnel 握手加入协议版本协商与能力声明。
- **验收**:安装可校验;旧版本 Agent 可被服务端识别并按策略处理。

### [x] P1-6 Token 轮换与泄露处置
- **问题**:无 `agent_token`/`instance_api_key` 轮换、撤销、泄露 blast radius 说明;`device_fingerprint` 用途/隐私未说明。
- **动作**:补 token 轮换 API、撤销即断连流程、泄露处置 runbook、fingerprint 用途与隐私声明。
- **验收**:有可执行的 key rotation 与撤销路径。

---

## P2 — 完善度

### [x] P2-1 枚举 Tenant.plan 并补 quota/usage 实体
- **动作**:`plan` 枚举与 §20 Lite/Starter/Pro/Team 对齐;新增 quota/usage 实体支撑 §12 presign 前的配额校验。

### [x] P2-2 合规:数据驻留与删除权
- **动作**:补 GDPR 数据驻留(对齐 §19.5 租户主 region)、用户/租户数据删除(被遗忘权)流程。

### [x] P2-3 分类型 CSP
- **动作**:§15 一刀切 `default-src 'none'` 可能挡掉浏览器原生 PDF 渲染;对 PDF / 文本 / 图片 / HTML 分别给出可用 CSP。

### [x] P2-4 验收标准与压测绑定
- **动作**:把 §19.8 容量数字与 §18 SLO 绑成可测压测目标;关键路径(tunnel 首字节、revoke 生效 ≤5s)给验收用例。

### [x] P2-5 统一 ER 图
- **动作**:补一张实体关系图(Tenant/User/Instance/FileRef/File/PreviewLink/AuditLog),平衡当前「成本估算极细、数据模型偏薄」的结构。

---

## 附:状态跟踪

| 优先级 | 总数 | 已完成 |
|--------|------|--------|
| P0     | 4    | 4      |
| P1     | 6    | 6      |
| P2     | 5    | 5      |

> 全部 15 项已落实到 `apage-spec.md`。

---

## 第二轮评审 — MVP 可开发性（控制面/运行时 API 缺口）

第一轮聚焦数据面与安全。结合 `apage-ui-spec.md` 做端到端核对后，发现 UI 依赖但后端 spec 缺失的契约，已在 `apage-spec.md` 新增 §25–§33 修复。

### [x] R2-1 实例供给入口缺失（P0）
- **问题**:`apage-agent start --token apage_xxx` 的 token 来源未定义；无 `POST /instances`，整个 tunnel 流程没有起点。
- **修复**:新增 §26 实例供给 API，创建实例并一次性返回 `agentToken` + `instanceApiKey`。

### [x] R2-2 认证/账户 API 缺失（P0）
- **问题**:UI §6.4 注册/登录/邮箱验证/重置密码，后端无对应 endpoint。
- **修复**:新增 §25 认证与账户 API（含 register 原子建租户、防枚举、argon2id）。

### [x] R2-3 预览运行时端点缺失（P0）
- **问题**:产品对外核心表面（`GET /p/{linkId}/{secret}`、密码校验、账户/IP 准入、举报）只有文字流程，无 HTTP 契约。
- **修复**:新增 §30 运行时端点 + 准入判定顺序、§30 密码 unlock、举报端点。

### [x] R2-4 成员/域名/用量管理 API 缺失（P1）
- **问题**:UI §7.5/§7.7/§7.8 依赖，后端无契约。
- **修复**:新增 §27 成员、§28 自定义域名、§29 用量计费 API。

### [x] R2-5 落地基建缺失（P1）
- **问题**:无具体 DB schema/迁移、无健康检查端点、无 env 清单,Single-Box 无法直接搭建。
- **修复**:新增 §31 健康/指标端点、§32 表清单与索引、§33 环境变量清单。

### [x] R2-6 PreviewLink schema 二义性（P2）
- **问题**:实体只列 `file_ref`,但 cloud 模式需 `file_id`;无 `frozen_at` 区分撤销与冻结。
- **修复**:§2 Preview Link 补 `file_id`、`frozen_at`、`frozen_reason` 与互斥规则。

| 轮次 | 总数 | 已完成 |
|------|------|--------|
| 第二轮 | 6 | 6 |
