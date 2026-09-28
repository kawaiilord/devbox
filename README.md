# 邻宇 · AI 社交与双宇宙

邻宇（暂定名）是一个持续开发中的 AI 社交应用。当前为 **M1 / 0.1 基础版本**：真实账户、好友、个人及平台宇宙、角色迁移、持久记忆、模型配置、文件任务和独立管理后台。

## 当前能力

- 响应式「聊天、好友、发现、我的」及独立的 `/admin` 管理后台。
- 注册登录、密码哈希、HttpOnly 会话、账户停用、权限检查及审计。
- 多个兼容 Chat Completions 或 Anthropic Messages 的模型，为好友单独切换。密钥加密保存。
- 文字对话请求真实配置的模型。未配置服务时明确提示，不返回伪造 AI 回复。
- 个人及平台宇宙；规则驱动生活事件、共同经历及朋友圈。每 15 分钟推进场景，可手动开启个人活动。
- 迁移包含版本校验与幂等记录，角色 ID 和用户关系保持连续。
- 记忆持久保存，包含来源及世界范围，支持删除；当前读取为按范围筛选的最近记忆。
- 通用任务有规划循环、取消、重试、步骤上限和文件版本。首批执行器支持文档、CSV、JSON、HTML 和源码读写下载。
- 缺少能力的目标明确标记阻塞。当前不会执行生成的程序、部署、浏览器自动化或外部系统修改。
- 后台显示真实用户、在线情况、角色归属、任务状态及审计。默认视图不返回私人聊天正文和密钥。

## 后续阶段

实时语音与视频、克隆音色、移动端锁屏通知、流式文字、向量记忆检索、可配置日程、MCP 连接器、Skill 包、浏览器和代码沙箱、媒体生成及更多后台角色在后续阶段实现。通话按钮明确为未接入状态。

## 运行

需要 Python 3.13+、Node.js 22+。

```bash
python3 -m venv .venv
.venv/bin/pip install -r backend/requirements.lock
cd web
npm ci
npm run build
cd ..
.venv/bin/uvicorn app.main:app --app-dir backend --host 127.0.0.1 --port 8000
```

打开 http://localhost:8000。注册后会创建一个个人宇宙和两位示例好友。进入「我的 → 添加模型」，输入公网 HTTPS Base URL、准确模型 ID 和自己的 API Key，再为好友选择模型。

兼容接口的 Base URL 形如 `https://provider.example/v1`。Anthropic 适配器接受服务根地址或以 `/v1` 结尾的地址。本版本仅支持 443 端口公网 HTTPS 服务，禁止回环、内网、云元数据地址及重定向。

浏览器开发模式可在 web 中运行 `npm run dev`，API 默认代理到本机 8000 端口。

## 创建管理员

注册接口不接受角色参数。管理员通过受控命令创建，无固定默认密码。

```bash
export UNIVERSE_ADMIN_EMAIL='your-admin@example.com'
read -rs -p '管理员密码（至少 10 位）: ' UNIVERSE_ADMIN_PASSWORD
export UNIVERSE_ADMIN_PASSWORD
PYTHONPATH=backend .venv/bin/python -m app.manage
unset UNIVERSE_ADMIN_PASSWORD
```

登录后访问 /admin。如账户已存在，命令不会自动提升权限或修改密码。

## 数据和配置

默认使用 `.data/universe.db`（SQLite），适用于单实例开发。使用 PostgreSQL 时设置：

```text
DATABASE_URL=postgresql+psycopg://USER:PASSWORD@HOST:5432/DATABASE
```

`UNIVERSE_DATA` 指定数据目录；`UNIVERSE_SECRET` 指定加密主秘密。未设置时自动生成仅当前用户可读的 `.data/.secret`，需与数据一起保存，否则旧模型密钥无法解密。

HTTPS 部署设置 `COOKIE_SECURE=true`。`ENABLE_WORKER=false` 关闭内置开发 Worker。当前 Worker 面向单进程，多实例队列、事务 Outbox 和正式数据库迁移为后续工作。

.env.example 是配置示例，可导出环境变量或通过 uvicorn --env-file 加载。不要将 .env、.data、会话、用户数据和 API Key 提交到公开仓库。

## 验证

```bash
.venv/bin/ruff check backend
.venv/bin/ruff format --check backend
.venv/bin/pytest -q
cd web
npm run build
npm run format:check
```

测试使用独立临时数据库。PostgreSQL 测试使用单独的 TEST_DATABASE_URL，库名必须包含 _test；不会使用常规 DATABASE_URL 清空数据。

浏览器回归需要 Chromium：

```bash
cd web
E2E_BASE_URL=http://127.0.0.1:8000 CHROME_PATH=/usr/bin/chromium npm run test:e2e
```

脚本在测试实例创建标记为测试的账户并截图。可通过 E2E_ADMIN_EMAIL、E2E_ADMIN_PASSWORD 测试后台。模型契约和 Agent 测试使用可控响应替身，不代表真实模型的生成质量和延迟。

## GitHub 开发环境

.devcontainer 提供初始化配置。GitHub Actions 编译前端并进行 SQLite / PostgreSQL 后端回归。Codespace 用于开发、编译与测试，生产部署独立规划。

## 文档

- docs/engineering-plan.md：工程计划。
- docs/adr/001-foundation.md：本阶段架构决策和限制。
- docs/verification.md：验证方法与证据。
