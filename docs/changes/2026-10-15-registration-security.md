# 注册来源记录与批量注册限制（2026-10-15）

状态：已上线，自动测试、线上浏览器验收及回滚检查通过。
基线代码：aa40dbc；原线上应用 dccd15a；果果取流故障继续按用户要求留待下次处理。

## 新增、修改与移除

- 新注册账号原子保存注册时间、注册方式、服务器解析的访问出口 IP、代理连接地址、可信代理来源、IPv6 /64 计数网络。
- 记录 User-Agent、请求语言、浏览器 Client Hints、设备标识摘要、第一方签名浏览器标识摘要，以及客户端上报的系统、语言、时区、屏幕和自动化标志。
- 环境摘要只用于关联线索，不把同一浏览器型号或相同屏幕配置单独作为封禁依据；不采集密码、媒体令牌、Canvas/音频指纹或精确位置。
- 默认同一 IPv4 IP / IPv6 /64 网络每 24 小时周期 5 次、同一设备及同一浏览器分别 3 次成功注册。周期从首次成功注册开始，额度在站点策略中可调整，0 表示不限制。
- PostgreSQL 事务内原子计数并创建用户及注册记录，拒绝并发超额；失败、重复邮箱、无效邀请码不消耗成功注册额度。
- 注册额度独立于 Redis 短期限流，跨应用重启、跨实例有效；账号删除不返还当前周期额度。过期计数在后续成功注册时清理。
- 密码注册与第三方登录首次建号共用限制；后台创建账号记录管理员操作来源，明确不是用户自身环境，不占自助注册额度。
- 已封禁设备在创建用户事务内拒绝，避免先留下账号再于绑定设备时失败。
- 管理后台“用户与访问”增加“注册环境”，仅用户管理权限可查看；支持同 IP、网络、设备、浏览器和环境的关联账号，最多展示 25 个。
- 注册页补充环境记录说明；已有账号不伪造或补记历史注册 IP，明确显示无历史记录。
- 保留原登录、邀请码、审核和邮箱验证设置；移除仅依赖可更换设备 ID 进行短期限流的不足。

## IP 信任边界

- 应用只信任显式配置的代理地址提供的单个 X-Real-IP，忽略直接请求中的 X-Forwarded-For、CF-Connecting-IP 等伪造值。
- 可信代理未提供有效客户端地址时拒绝新注册，避免把所有用户记成代理地址。
- 生产 Nginx 已对 Cloudflare 来源网段配置 real_ip，并用解析后的 remote_addr 覆盖发给应用的 X-Real-IP。应用端只信任其 Docker 代理网络；应用端口仍仅绑定本机。
- 本次检查同帧域名解析到生产服务器；未更改原站或其代理。
- IP 是可核验的访问出口；VPN、代理后的底层家庭 IP 无法由本站 HTTP 请求直接确定。浏览器环境属于可修改的客户端信息，记录会明确其来源。
- 代理解析依据：[NGINX realip 官方文档](https://nginx.org/en/docs/http/ngx_http_realip_module.html)。

## 数据与回滚

新增迁移 023_registration_security.sql：registration_records 与 registration_quotas 及索引。用户注册记录随账号正式删除清理，独立摘要计数保留至周期过期。
迁移为增量，旧版应用可以忽略新增表；代码回滚保留用户活动及记录，不恢复旧数据库。

发布前已备份数据库、网页及后端镜像。具体路径和哈希见下方发布记录。

## 验证

- 新增直接连接伪造头、可信代理来源、IPv4 映射地址、IPv6 /64、非法环境上报和伪造浏览器 Cookie 测试。
- 新增注册记录不可被后续登录覆盖、无权限不可读取、旧账号缺失来源、浏览器 ID 关联与额度测试。
- 新增内存与 PostgreSQL 并发注册、设备跨 IP / OAuth 计数、失败回滚、重复邮箱不占额、封禁设备阻断测试。
- JavaScript 语法检查、CI 及真实浏览器验收均通过。

## 上线验收与发布记录

- 发布提交：3120e531b71214b67bcc65926a64469b6894b472。
- [CI 38719345246](https://github.com/kawaiilord/devbox/actions/runs/38719345246)：server、flutter-web、flutter-windows、flutter-linux、server-runtime、guoguo-provider 六项全部成功。
- 内存及 PostgreSQL 实测同一网络 18 个并发注册，仅 5 个成功；同设备跨 IP / OAuth 共用 3 次额度；重复邮箱不占额度、超额不残留账号、封禁设备阻断均通过。
- HTTP 浏览器关联测试：保留签名浏览器 Cookie，同时更换设备 ID 和来源 IP，第 4 次注册返回 429，附带 Retry-After。
- 线上 Edge 实际注册验收通过，页面 JavaScript 错误为 0。注册请求携带三种伪造 IP 头，数据库仍保存可信代理解析的访问出口 IP，而非伪造值或 Docker 代理地址。
- 线上成功记录 Windows、Edge User-Agent / Client Hints、zh-CN、Asia/Shanghai、1280×950 屏幕及浏览器/设备/环境摘要。自动化验收的 WebDriver 标志如实记录为 true，未将这个单独信号作为封禁依据。
- 现有“邀请码 + 管理员审批”模式保持生效，测试注册返回 202 待审批。测试账号已移入回收站，其会话失效；使用的测试邀请码不能复用。
- 未登录读取注册详情返回 401；普通用户与运营角色的 403 权限隔离由自动测试验证。旧管理员账号返回 record=null，未补造历史来源。
- 线上管理后台已验证“用户管理 → 注册环境”及“站点策略 → 注册额度”字段。当前默认值为 IP 5、设备/浏览器 3。
- 部署就绪检查、备份哈希及回滚可用性检查通过。

部署完成时间：2026-10-15 10:25:50 UTC（北京时间 18:25:50）。

| 资源 | SHA-256 |
| --- | --- |
| 新服务端 | 0321917c0f677391d510322661fac110d633679df31497f53a1a6a6e6c49ce4e |
| 新网页 main.dart.js | bef1732e36cbeff346f094d4d71f4a0c397e97c1b56ba8f574914f1f2b9aaf03 |
| CI 网页压缩包 | 336716e51bb76c4c93a0d5d87cabbe54178ce97fa0b71eaee2bb7b9c6d918880 |
| 原服务端 | c13e0c09cfb6148365d0c7bde7ca1c6fa48877021ea735f198fbd96bee19f620 |
| 数据库备份 | 86365e1ed1a0cdfdd0fe30b4327e02e4f57d8438cbf35b470ea398ba8d705a28 |

备份：`/opt/sameframe/backups/registration-security-20261015-102508`

部署清单：`/opt/sameframe/releases/registration-security-3120e531b71214b67bcc65926a64469b6894b472/deployment.json`

回滚命令：

```sh
python3 /opt/sameframe/releases/registration-security-3120e531b71214b67bcc65926a64469b6894b472/release.py --rollback
```

回滚保留新增用户、注册记录与计数表；不会恢复旧数据库。回滚旧代码后，新的注册记录采集与每日额度不再执行，重新部署本版本可恢复功能。

验收文件在 `artifacts/registration-security-20261015/`：

- `registration-environment.png`：后台注册来源和环境。
- `registration-policy.png`：额度配置及保留的注册设置。
- `register-notice.png`：注册页采集说明。
- `live-acceptance.json`：浏览器验收结果，不包含登录密码、令牌或邀请码。
