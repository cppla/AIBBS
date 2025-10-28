# AIBBS 更新记录

> 本文档用于记录 aibbs 的所有版本变更，请在每次发布时同步更新版本号与关键信息。

## v1.0.0 · 2025-10-09

### 核心功能
- 本地用户注册、登录、登出，密码采用 bcrypt 哈希存储。
- GitHub、Google、Telegram 第三方登录流程，自动合并/创建用户，支持 OAuth 状态校验。
- 帖子发布、评论回复、帖子分页与详情查询，并预加载作者与评论信息。
- 每日签到功能，记录签到时间、连续天数并发放奖励积分。

### 安全与工程
- 所有数据库操作基于 GORM，默认使用参数化语句防止 SQL 注入。
- 发帖、评论、昵称等用户内容经 Bluemonday 清洗，降低 XSS 风险。
- 实现 JWT 鉴权与内存黑名单机制，敏感接口叠加基于 IP 的速率限制。
- 提供 `.env` 配置加载、MySQL 初始化脚本、模块化目录结构与统一响应封装。

### 文档与可运维性
- 完成 README 快速上手指南、第三方登录配置说明、API 路由清单。
- 新增版本常量 `config.Version` 以便程序内复用，README 标注当前版本并指向本更新记录。
- 针对历史数据库的 `users.username` 唯一索引提供自动修复逻辑，避免 AutoMigrate 失败，通过删除所有旧 UNIQUE 索引让 GORM 重新创建 `idx_users_username` 唯一索引。

---

## 2025-10-13

### 新增
- 日志与模式：引入 Zap + Lumberjack 日志，`log` 分组支持 Level/Path/MaxSizeMB/MaxBackups/MaxAgeDays/Compress；Gin 访问日志独立输出（`log.GinPath`），支持 `log.GinMode` 切换 debug/test/release。
- 优雅关闭/平滑重启：新增 `utils/grace_server.go`，支持 SIGTERM 优雅关闭与 SIGUSR2 零停机重启（FD 继承）。
- 国家访问控制：新增 `AllowedCountry` 与 `DenyCountry` 配置；新增中间件 `CountryFilter`，黑名单优先，浏览器返回提示页，API 返回 JSON；IP→国家识别集成云端接口并加入进程内与 Redis 缓存。
- 初始化脚本：新增 `scripts/init_db.py` 基于 PyMySQL 初始化数据库；保留 `scripts/init.sql`。

### 变更
- 移除 `scripts/init_db.sh`，README 改为使用 Python 脚本初始化；服务启动若检测到空库，提示执行初始化并退出。
- GORM 迁移策略更安全：禁用迁移期外键改动；仅在表不存在时才执行 AutoMigrate，避免误删旧索引（修复 `uni_users_username` 引发的 1091 错误）。

---

## 2025-10-15

### 新增
- 注册安全加强：新增开源验证码与防刷策略。
	- 新增 `GET /api/v1/auth/captcha` 接口返回 `{id,image}`。
	- 注册支持验证码校验（可在 `config/config.json` → `register.CaptchaEnabled` 开关）。
	- 基于 Redis 的注册防刷：尝试冷却、每日成功次数限制、失败过多临时封禁。
	- 新增配置分组 `register`：`CaptchaEnabled`、`MaxPerIPPerDay`、`AttemptCooldownSec`、`FailedMaxPerIPPerHour`、`TempBanMinutes`。

### 管理员与前端操作
- 后端在认证相关响应（注册、登录、OAuth/Telegram 登录、`/auth/me`、`/auth/profile`）中新增 `user.is_admin` 字段。
- 配置支持在 `app.AdminUsernames` 或 `admin.Usernames` 中设置超级管理员列表（兼容旧格式，优先读取 `admin.Usernames`）。
- 前端：管理员在帖子详情页和评论区可直接看到“删除”按钮，并可删除任意帖子与评论；普通用户仅能删除自己的内容。

---

## 2025-10-16

### 附件上传与自焚（历史记录）
- 上传方式改为本地存储：保存到 `/static/uploads/YYYY/MM/DD/`，上传成功后自动将 URL 插入发帖/编辑正文。
- 文件大小限制为 50MB，允许上传任意文件类型（安全起见仍建议在前端限制图片类型）。
- 当时引入了“附件上传后定时删除”的配置与后台清理机制（现已在 2025-10-28 全部移除）。
- 对应的配置项、数据表与清理器均已废弃，不再保留具体键名与实现名以减少误导。

---

## 2025-10-28

### 前端交互与通知
- 全量将浏览器 `alert()` 替换为站内 `notify()` 提示；删除 `confirm()`，统一为自定义 Bootstrap 模态 `confirmModal()`，用户体验更一致。
- UI：在侧边栏用户信息与个人页显示“管理员”徽标（当 `user.is_admin = true`）。

### 附件展示策略
- 不再在帖子文末单独展示附件列表；附件在上传成功后直接插入正文，以正文内容为准（`posts.attachments` 字段保留以保持向后兼容）。

### CORS 调研与策略
- 评估云图床上传的跨域预检（OPTIONS）行为，发现返回 404 导致浏览器阻断；因此回退到本地上传方案（详见 2025-10-16），避免跨域风险。

### 文档
- README 增补“管理员账号与密码”说明：管理员只按用户名判定，配置中不存放密码；密码由注册/登录流程创建并以 bcrypt 存储。
- README 在“附件上传与自焚配置”中明确不再渲染文末附件列表；“数据库结构”补充 `page_views` 与 `uploaded_files`。

### 清理
- 移除本地上传自焚相关：删除所有相关配置、解析与默认值，同时移除后台清理机制与对应数据表/模型（仅在更新记录中保留“已移除”的说明，不再出现具体键名）。
