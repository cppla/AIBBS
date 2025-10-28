# AIBBS，全球第一个AI驱动的论坛系统，没有一行代码来自于人类。

> 当前版本：**v1.0.0**（详见 [UPDATE.MD](UPDATE.MD) 获取完整更新记录）

一个使用 Go (Golang) + Gin + GORM 构建的完整 BBS（论坛）系统，实现用户注册登录、第三方登录、发帖回帖、每日签到、JWT 鉴权、速率限制与内容 XSS 过滤等核心能力。所有 API 返回统一的 JSON 结构，满足 MCP（Mobile Control Panel）风格的接口规范。

## AI开发规范

- 维护者：请不要主动写任何代码，一切交给AI完成，包括且不限于ChatGPT、Copilot、Cursor、Visual Studio等AI工具。
- 请务必使用万亿以上的大模型，包括且不限于GPT-5、Claude Sonnect、Gemini Pro、Qwen Max、Grok等。
- Prompt中务必包含：“更新README.md 内容和 更新UPDATE.MD更新记录”

## 版本信息

- **v1.0.0**：初始公开版本，覆盖注册登录、第三方认证、发帖评论、每日签到、安全防护与统一 API 响应。更多详情参见 [更新记录](UPDATE.MD)。
- **2025-10-09 Update**：修复自动迁移因旧版唯一索引命名导致的启动异常。


## 功能特性

- **用户体系**：本地注册/登录（bcrypt 哈希）、JWT 鉴权、Token 黑名单登出。
- **第三方登录**：GitHub、Google、Telegram OAuth/OIDC 支持，自动创建或合并用户数据。
- **论坛交互**：发帖、评论、分页列表、详情页（含评论与作者信息）。
- **每日签到**：记录签到日期、连续天数，发放可配置积分奖励。
- **安全防护**：参数化 ORM 操作、防 SQL 注入；Bluemonday XSS 过滤；速率限制；环境变量管理密钥。
- **工程规范**：模块化目录（controllers / routes / middleware / models / utils / config），JSON 配置（`config/config.json`），初始化 SQL，统一响应结构。
- **日志与模式**：Zap 结构化日志 + 滚动文件；Gin 访问日志独立文件（`log.GinPath`），运行模式支持 `log.GinMode`（debug/test/release）。
- **优雅关闭/平滑重启**：支持 SIGTERM 优雅关闭，SIGUSR2 零停机重启（FD 继承）。
- **国家访问控制**：支持国家白名单/黑名单（黑名单优先），基于外部 IP 接口识别，浏览器返回提示页面，API 返回 JSON。

## 快速开始

### 运行环境

- Go 1.21+
- MySQL 8.0+（或兼容版本）
- GitHub / Google OAuth 应用、Telegram Bot（可选）

### 安装步骤

1. **克隆代码并初始化依赖**

	 ```bash
	 git clone <your-repo-url> aibbs
	 cd aibbs
	 go mod tidy
	 ```

2. **配置应用（JSON + 可选环境变量覆盖）**

	 编辑 `config/config.json` 填写数据库、`JWTSecret`、OAuth 凭据等；也可通过环境变量覆盖同名字段。

3. **初始化数据库**

	```bash
	# 一键初始化（优先读取 config/config.json；支持 DATABASE_URI 或 DB_* 环境变量）
	python3 scripts/init_db.py
	```

4. **启动服务**

	 ```bash
	 go run main.go
	 ```

	 默认监听 `:8080`，可通过 `APP_PORT` 环境变量调整。

### 日志与运行模式

- 应用日志：`log.Path`（JSON 格式，滚动文件）
- Gin 访问日志：`log.GinPath`（JSON 格式，滚动文件）
- 级别：`log.Level`（debug/info/warn/error）
- 运行模式：`log.GinMode`（debug/test/release）
- 滚动参数共用：`log.MaxSizeMB`、`log.MaxBackups`、`log.MaxAgeDays`、`log.Compress`

### 优雅关闭与平滑重启（可选）

- 优雅关闭：向进程发送 SIGTERM，服务等待在途请求完成后退出。
- 平滑重启：向进程发送 SIGUSR2，子进程接管监听 FD，父进程优雅退出。

### 国家访问控制（可选）

- 配置项（`config/config.json` → app）：
	- `AllowedCountry`: 允许访问的国家白名单（为空则不启用白名单）
	- `DenyCountry`: 禁止访问的国家黑名单（优先于白名单）
- 识别来源：`https://api.cloudcpp.com/ip?ip=<client_ip>`（User-Agent 固定为 `AIBBS`）
- 命中规则：
	1) 在黑名单 → 403 拒绝
	2) 否则若配置了白名单且不在白名单 → 403 拒绝
	3) 其他情况允许；私网 IP 直接允许；查询失败时放行（fail-open）
- 响应形式：
	- 浏览器（非 /api 路径，Accept 包含 text/html）：返回 HTML 提示页（“当前 {国家}不被允许访问”）
	- API（/api/...）：返回 JSON 错误
- 缓存：IP→国家采用进程内缓存与 Redis 缓存（默认 TTL 24h）

### 注册防刷与验证码（可选）

- 新增 GET `/api/v1/auth/captcha` 获取验证码，返回 `{ id, image }`，`image` 为可直接展示的 data URI。
- 当在配置中开启 `register.CaptchaEnabled` 时，注册接口需提交 `captcha_id` 与 `captcha_answer` 字段。
- 通过 Redis 实现注册防刷策略：
	- `register.AttemptCooldownSec`：每次尝试间隔；
	- `register.MaxPerIPPerDay`：每 IP 每日成功注册上限；
	- `register.FailedMaxPerIPPerHour` + `register.TempBanMinutes`：每小时失败达到阈值后临时封禁。

配置示例（config/config.json）：

```jsonc
{
	"register": {
		"CaptchaEnabled": true,
		"MaxPerIPPerDay": 5,
		"AttemptCooldownSec": 10,
		"FailedMaxPerIPPerHour": 20,
		"TempBanMinutes": 60
	}
}
```

## 第三方登录配置指引

| 平台     | 配置关键点 |
|----------|-------------|
| GitHub   | 在 [Developer Settings](https://github.com/settings/developers) 中创建 OAuth App，设置回调为 `http(s)://<host>/api/v1/auth/oauth/github/callback`，将 Client ID/Secret 写入 `config/config.json` 或以环境变量覆盖。 |
| Google   | 在 Google Cloud Console 创建 OAuth Client（Web 应用），授权回调 URI：`http(s)://<host>/api/v1/auth/oauth/google/callback`。启用 `profile` / `email` Scope。 |
| Telegram | 通过 [@BotFather](https://t.me/BotFather) 获取 bot token，将 `TELEGRAM_BOT_TOKEN` 写入 `config/config.json` 或以环境变量覆盖，前端需集成 Telegram Login Widget 并将登录数据 POST 到 `/api/v1/auth/telegram`。 |

> **注意**：`OAuthRedirectBase`（或环境变量 `OAUTH_REDIRECT_BASE_URL`）用于生成回调地址，请配置为对外可访问的域名（含协议）。

## MCP 风格响应格式

所有接口统一返回 JSON：

```json
{
	"code": 0,
	"message": "success",
	"data": {
		"...": "..."
	}
}
```

- `code = 0` 表示成功，非零表示业务错误码。
- `message` 为简洁描述（中文或英文均可）。
- `data` 承载业务数据，错误时可为空。

## 主要 API 路由

| 方法 | 路径 | 说明 | 鉴权 | 样例 |
|------|------|------|------|------|
| POST | `/api/v1/auth/register` | 用户注册 | 否 | `{"username":"alice","password":"Secret123","display_name":"Alice"}` |
| POST | `/api/v1/auth/login` | 用户登录 | 否 | `{"username":"alice","password":"Secret123"}` |
| GET  | `/api/v1/auth/me` | 当前用户（含 is_admin） | 是 | 返回 `user.is_admin` 用于前端显示管理员操作 |
| POST | `/api/v1/auth/logout` | 用户登出（Token 黑名单） | 是 | Header: `Authorization: Bearer <token>` |
| GET  | `/api/v1/auth/oauth/:provider/login` | 获取 OAuth 授权 URL（provider=`github`/`google`） | 否 | 返回 `authorization_url`、`state` |
| GET  | `/api/v1/auth/oauth/:provider/callback` | OAuth 回调处理 | 否 | 前端在授权后跳转，后端签发 JWT |
| POST | `/api/v1/auth/telegram` | Telegram 登录验证 | 否 | 前端提交 Telegram Widget 返回的 JSON |
| GET  | `/api/v1/posts` | 分页帖子列表 | 否 | Query: `page=1&page_size=10` |
| GET  | `/api/v1/posts/:id` | 帖子详情（含评论） | 否 | - |
| POST | `/api/v1/posts` | 创建帖子 | 是 | Body: `{"title":"Hello","content":"<p>world</p>"}` |
| POST | `/api/v1/posts/:id/comments` | 对帖子评论 | 是 | Body: `{"content":"Nice!"}` |
| DELETE | `/api/v1/comments/:commentId` | 删除评论（本人或超级管理员） | 是 | Header: `Authorization: Bearer <token>` |
| POST | `/api/v1/signin/daily` | 每日签到 | 是 | 返回奖励积分、最新连续天数 |
| GET  | `/api/v1/signin/status` | 签到状态 | 是 | 返回累计积分、连续天数、最近签到时间 |

**示例：用户发帖**

```http
POST /api/v1/posts
Authorization: Bearer <token>
Content-Type: application/json

{
	"title": "新版本发布",
	"content": "<p>今天发布了新版本 🎉</p>"
}

响应：
{
	"code": 0,
	"message": "success",
	"data": {
		"post": {
			"id": 1,
			"title": "新版本发布",
			"content": "<p>今天发布了新版本 🎉</p>",
			"created_at": "2025-01-01T12:00:00Z",
			"user_id": 2
		}
	}
}
```

## 数据库结构

- `users`：用户账户信息（本地与 OAuth）
- `posts`：帖子主体，关联作者
- `comments`：帖子评论，关联帖子与用户
- `sign_ins`：每日签到记录（奖励积分、连续天数）
-	`page_views`：按天与路径聚合的页面访问统计
- `uploaded_files`：上传文件记录（文件路径、公开 URL、过期时间），用于定期清理自焚文件

 建表脚本位于 `scripts/init.sql`，可直接导入。首次启动若检测到数据库为空，服务会提示执行 `python3 scripts/init_db.py` 并退出；避免在未初始化时误操作数据库。

## 安全与防护

- **密码安全**：bcrypt 哈希，本地永不存储明文。
- **JWT 鉴权**：`JWT_SECRET` 来自环境变量，登陆退出均校验 Token，有内存黑名单支持退出生效。
- **内容过滤**：所有用户输入（帖子、评论、昵称等）均通过 Bluemonday 进行 XSS 清洗。
- **速率限制**：对登录、发帖、评论、签到等敏感接口施加基于 IP 的限流策略，默认每分钟 60 次，可在 `config/config.json` 或环境变量中配置。
- **SQL 安全**：统一由 GORM ORM 执行，避免手写 SQL 注入风险。

## 附件上传与自焚配置

- 前端在发帖/编辑时选择文件会自动上传到本地目录 `/static/uploads/YYYY/MM/DD/`，成功后立即将 URL 插入正文（图片以 Markdown `![alt](url)` 插入，其他类型直接插入 URL）。
- 单文件上限：50MB。
- 自焚机制：上传文件默认在 60 分钟后自动删除。该行为可通过配置调整：

```jsonc
{
	"app": {
		"UploadsSelfDestructEnabled": true,     // 是否启用自焚
		"UploadsSelfDestructMinutes": 60        // 自焚分钟数
	}
}
```

- 后台有清理器每 5 分钟扫描一次过期文件，确保即使服务重启也能清理过期文件。
 
- 展示规则：不再在文章尾部单独展示附件列表，所有附件以正文内插入为准。

## 超级管理员与内容删除

- 在 `config/config.json` 中配置超级管理员用户名列表，具备删除任何帖子与评论的权限：

```jsonc
{
	"app": {
		"AdminUsernames": ["root", "admin"]
	},
	"admin": { // 兼容旧格式
		"Usernames": ["root", "admin"]
	}
}
```

- 也兼容扁平字段 `AdminUsernames`（数组），两者皆可，优先读取 `admin.Usernames`。
- 普通用户可删除“自己的评论”和“自己的帖子”；超级管理员可删除任何帖子和评论。

返回的用户对象（登录/注册/me/profile 等）包含布尔字段 `is_admin`，前端据此展示管理员操作入口。

### 关于管理员账号与密码

- 管理员身份仅通过“用户名是否在管理员列表中”判定；配置文件中不存放任何密码。
- 管理员账号的密码与普通用户一致，通过“正常注册/登录流程”创建并以 bcrypt 哈希存储；系统从不保存明文密码。
- 如果在 `AdminUsernames` 中添加了某用户名，但数据库中还没有该用户，请先用该用户名注册一个账户（或通过现有登录方式创建用户），之后该用户即被认定为管理员。
- 换言之：配置里的管理员用户名只授予“角色”，并不创建或设置密码。请不要尝试在配置中填写密码（出于安全考虑也不支持）。

## 开发与测试

- **代码格式化**：已配置 gofmt，提交前可运行 `gofmt -w ./...`。
- **单元测试**：可执行 `go test ./...`（当前示例未附带测试用例，可按需补充）。
- **热加载**：建议配合 [Air](https://github.com/cosmtrek/air) 等工具在本地开发时实现自动重载。

## 后续可扩展方向

- WebSocket 新帖/评论实时推送
- 角色/权限体系、后台管理
- 富文本存储与审核流程
- Redis 分布式限流、Token 黑名单持久化

欢迎在此基础上继续扩展打造适合自身业务的论坛系统。