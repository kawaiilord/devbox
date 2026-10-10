# 视频平台与 NAS 媒体源

此版本扩展 SameFrame 现有 `/api/v1` 服务，连接入口位于「我的媒体源 → 添加平台 / NAS」。
现有 WebDAV、Emby 和夸克入口继续可用。

## 视频平台

提供 Bilibili、YouTube、抖音、TikTok、Twitch、虎牙、斗鱼、AcFun 和央视的 URL 接入。
流程为：保存平台连接 → 粘贴视频或播放列表链接 → 解析 → 选片建房，或在房间片单中批量添加。
每页解析最多 50 项，可继续加载后续剧集；房间片单上限为 500 项。

平台解析使用固定版本 `yt-dlp 2026.8.19`，只读取媒体信息，不下载完整影片、不标记已观看。
输入域名和提取器类型均受限制；禁用用户配置、外部插件、浏览器 Cookie 自动读取、远程组件和地理绕过。
请求有时间、输出大小和并发上限。登录 Cookie 为可选的单行请求头值，加密保存；临时 Cookie 文件为
0600 权限，解析后删除。凭据只用于所选平台的主域，不发送给第三方 CDN。

画质菜单展示解析器返回的可用格式。普通文件通过 Range 代理播放；HLS 清单内的子清单、片段和密钥
地址转换为绑定父票据的加密链接；分离的 MP4 音视频使用 SIDX 索引生成 DASH 清单。
Web 使用随包提供的 Shaka Player 和 mpegts.js，桌面使用 media_kit/MPV。
直播提供观看、播放/暂停和换源，不提供点播式拖动与倍速。

站点登录状态、地区、风控、会员和资源权限仍由平台决定。没有实现 DRM 破解、验证码绕过、
平台账号密码登录或原生扫码登录。仅保存连接不会证明平台登录有效，需解析实际链接验证。
本机对公开 YouTube 视频的匿名检查返回了「Sign in to confirm you’re not a bot」，所以不能把
模拟接口测试当成真实 YouTube 账号实播验收。其他平台同样需要使用部署环境和实际资源验收。

## NAS 连接

| 类型 | 登录方式 | 浏览与原始文件播放 |
|---|---|---|
| 群晖 Synology | File Station 会话，支持可选 OTP | API 发现、共享目录、文件目录、下载流 |
| 飞牛 fnOS | RSA/AES 加密登录，可选 OTP，HMAC WebSocket 请求 | 原生目录浏览；文件播放使用已启用的 WebDAV 服务 |
| QNAP | File Station 账号密码会话 | 共享目录、分页文件列表、下载流 |
| Nextcloud | 用户名与应用密码 | OCS 验证用户，官方 WebDAV 文件路径 |
| Seafile | 账号密码换取 API Token | 非加密资料库、目录、短时文件链接 |
| TrueNAS | API Key | REST API v2，限定 `/mnt`，检查文件 realpath 后申请下载票据 |

这里接入的是上述文件服务，不包括所有厂商的影视刮削、原生转码、加密资料库和管理功能。
TrueNAS 需要仍提供 REST API v2 的版本；飞牛需要 WebDAV 文件服务可用，并且与登录地址使用同一 NAS 主机。
真实设备的版本、权限、验证码和文件格式兼容性需要单独验收。Web 播放原始 MKV/音轨的能力取决于浏览器。

凭据使用现有 AES-GCM vault。群晖、QNAP 和 Seafile 保存会话或 Token；Nextcloud 和飞牛因 DAV 播放需要
保存加密的应用密码/密码。OTP 不持久化。更新登录会保留媒体源 ID。

## 运行组件与网络

Docker 镜像包含 Python、Node 24 和固定版本的 yt-dlp。直接运行 Go 二进制时需要自行安装解析组件：

```sh
python3 -m venv /opt/sameframe-resolver
/opt/sameframe-resolver/bin/pip install 'yt-dlp[default]==2026.8.19'
# 系统还需安装受支持的 Node.js 运行时。
export SAMEFRAME_YTDLP_PATH=/opt/sameframe-resolver/bin/yt-dlp
```

所有受保护媒体源需要稳定的 `SAMEFRAME_VAULT_KEY`，房间短时播放票据需要 Redis。
平台媒体流只访问公共 HTTP(S) 目标，且在每次连接时验证 DNS 地址。

访问内网 NAS 时配置精确的主机与端口白名单，例如：

```env
SAMEFRAME_SOURCE_HOST_ALLOWLIST=nas.home:5001,nas.home:5006,fnos.home:5667,fnos.home:5005
SAMEFRAME_ALLOW_PRIVATE_SOURCES=false
```

没有端口时按 443 处理。白名单只放开指定服务端点，不会放开平台 CDN 的私网访问。
飞牛的登录端口和 DAV 端口需要分别配置。原有全局私有地址开关保留给隔离开发环境。

## 验证依据

自动化覆盖六种 NAS 的协议模拟、飞牛加密/OTP/HMAC、目录与 Range 转发、登录凭据隔离、
平台元信息与画质、解析子进程隔离、HLS 子资源票据、DASH 索引以及房间权限。
这些是兼容协议与应用流程测试，不替代真实第三方账号和 NAS 设备测试。

协议参考：[SyncTV NAS adapters](https://github.com/synctv-org/synctv/tree/main/synctv-media-providers/src)，
解析器参考：[yt-dlp](https://github.com/yt-dlp/yt-dlp)。Web 播放依赖版本及许可证位于
`client/web/vendor/versions.json` 和同目录 LICENSE 文件。
