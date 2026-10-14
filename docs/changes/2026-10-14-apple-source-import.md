# aplle-min.json 导入与 APTV 直播兼容

用户要求导入附件 `aplle-min.json`。输入 SHA-256：`75f970e77b91d9cac36c22840374f4df391b4117e330fd9927377fac240050bf`。

## 当前进度

- 27 个 `type=1` 影视接口已从后台预览并确认导入，正式服务器的 27 项接口检测全部通过。这只表示接口能读取，不等于已逐部验证视频播放。
- migu 列表只有 M3U 头信息，没有频道，跳过且未影响影视接口导入。
- Kimentanm 列表包含 APTV 扩展声明和每频道 `http-user-agent`。原导入器把任意 `#EXT-X-` 都当作 HLS 视频，因此误拒绝这份频道列表。本轮补齐兼容后再按原地址订阅。

## 程序修改

- 只把 `#EXT-X-APP`、`#EXT-X-APTV-TYPE`、`#EXT-X-SUB-URL` 当作应用元数据忽略；实际 HLS 分片 / 主播放清单仍拒绝作为频道列表导入。
- 读取频道 `http-user-agent` 和 `#EXTVLCOPT:http-user-agent`，经过长度、控制字符校验后，仅用于该频道的连接检测、媒体清单与分片请求。
- User-Agent 随服务端播放票据传递，不把登录凭据、Cookie 或其他任意请求头传给上游；不执行扩展标签，也不自动跟随标签中的订阅 URL。
- 保持没有 User-Agent 的旧频道 ID 稳定，避免已有房间、收藏引用因本次升级改变。
- 加密配置仅增加可选字段，无 SQL 迁移；旧代码回滚可以读取配置，新的 APTV 更新 / 指定标识播放能力在旧版暂不可用。

## 验证

本地针对性测试已通过：APTV 声明与 HLS 区分、每频道标识不串用、控制字符拒绝、旧频道 ID 稳定、加密保存 / 播放票据 / 分片请求保持指定标识。

代码基线：`40edc87`。回滚标签：`rollback/pre-apple-m3u-20261014`。

## 导入前备份

- `/opt/sameframe/backups/apple-sources-20261014-032041/`
- 数据库 SHA-256：`35e1fbd117d8cc275dd09195ac19b929297dfc777cdcbe541aa062a066617957`
- 加密目录快照 SHA-256：`8eee078db76d8f3828d5545587a60fcbc42ae282386e37bed143ecfedca4b3e9`
- 配置操作记录：`/opt/sameframe/releases/apple-sources-20261014-032041/operation.json`

程序发布、最终直播导入数、产物哈希和回滚命令将在完成后补齐。所有撤销保留后续用户活动，不恢复旧数据库。
