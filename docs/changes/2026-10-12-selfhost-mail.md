# 自建邮件服务

状态：服务器端准备完成，内部验收通过；外部发信未启用。25 端口、DNS/PTR 尚待完成，注册验证码暂不能送达外部邮箱。

## 新增

- 在现有 Debian 服务器安装 Postfix、OpenDKIM、Cyrus SASL，使用 `no-reply@sameframe.top`。
- 587 端口要求 TLS 和独立随机密码，只允许该账号使用自己的发件地址。
- 25 端口只接收本地域名邮件，不接受匿名转发；本机私有邮箱保存退信及验收邮件。
- DKIM 2048 位密钥，私钥仅保存在服务器；提供 SPF、DKIM、DMARC、MX、A 公共记录。
- 安装、连通性检查、内部投递验收、条件齐备后启用、回滚脚本。

## 调整

- 同帧后台预填自建 SMTP，密码通过已有密钥库加密保存，待外部条件齐备后才启用。
- 证书续期后重载 Postfix；邮件日志按天轮转。
- 补全上一版 `4bbf313` 的实际部署、API 验证和回滚记录。

## 移除

- 无应用功能删除，无数据库结构变更。

## 外部依赖与限制

- 2026-10-12 从服务器直连 Gmail、QQ、163 的多个 MX 及 Outlook 的 25 端口均超时；主机 OUTPUT 默认 ACCEPT。
- `38.76.190.179` 无 PTR；`mail.sameframe.top`、域名 MX、SPF、DMARC 尚未发布。
- 服务商是 SadIDC；用户要求暂不处理其 API。没有修改提供商控制台或提交工单。
- 没有 Cloudflare DNS 写入凭据，生成导入文件后交由域名所有者添加。
- 外网条件不满足时，SMTP 在后台保持关闭，且 587 端口额外拒绝非本地域名收件人。
- 本机 SMTP 接受、DKIM 验签和对方邮箱接收是不同验收阶段；收件箱送达须后续单独确认。

## 基线与回滚

- 应用版本：`4bbf313`，本次仅增加部署配置和运维脚本，不替换应用二进制或网页资源。
- 基线标签：`rollback/pre-selfhost-mail-20261012`。
- 私有状态：`/opt/sameframe/mail/`；清单 `deployment.json`。
- 备份：`/opt/sameframe/mail/backup-20261012-151939`，原配置与 PostgreSQL dump 已逐项校验 SHA-256。
- 完成服务器准备：`2026-10-12T15:28:01Z`；验收时间：`2026-10-12T15:35:32Z`。
- 回滚：`python3 /opt/sameframe/mail-ops/install.py --rollback`。
- 回滚恢复后台原 SMTP 配置、原证书续期脚本并停用邮件服务；保留密钥、队列、邮箱和软件包供检查。
- 不恢复旧数据库，不回退任何用户活动；若后续已添加 DNS/PTR，需另行回退。

## 软件版本

- opendkim 2.11.0~beta2-9.1+b1
- postfix 3.10.13-0+deb13u1
- sasl2-bin 2.1.28+dfsg1-9

## 验证

- Postfix、OpenDKIM 运行中并已设置自启动；TLS 证书验证通过。
- 同帧应用网络内使用随机 SMTP 密码和 STARTTLS 提交邮件，本机邮箱收到，自动补齐 Message-ID，DKIM 使用生成的公钥验证成功。
- 未登录转发、错误密码、冒用发件地址被拒绝；未完成设置时外部收件人收到 450，后台 SMTP 保持关闭。
- 从另一台机器连入 587 成功，匿名发信拒绝；入站 25 超时。
- 再次检测 126、QQ、Gmail 的出站 25，全部超时；全部 DNS/PTR 记录尚未发布。
- 邮件队列为空；没有向外部收件人投递邮件。用户提供的 126 邮箱留待外网条件满足后测试。
- 备份哈希与续期脚本语法通过；应用二进制及主 JS 哈希与部署前一致，readyz 正常。
- 运维脚本经过 Python 语法检查；配置以实际 Debian 软件包的运行与投递测试验收，无应用代码重编译。
- 首次安装遇到其他 apt 任务持锁，使用原备份恢复安装；修正 Debian SASL 配置目录权限/路径和暂缓投递规则后，完整内部验收通过。

## 交接文件与下一步

1. [Cloudflare DNS 导入文件](assets/sameframe-mail-dns.zone)：A/MX/SPF/DKIM/DMARC 共 5 条，mail 主机必须设为“仅 DNS”。
2. [SadIDC 请求文本](assets/sameframe-mail-provider-request.txt)：放通 TCP 25 出入站并配置 `38.76.190.179 -> mail.sameframe.top` PTR；只是草稿，未代发。
3. 完成后运行 `python3 /opt/sameframe/mail-ops/check.py`。
4. 全部通过后，使用 `activate.py --test-to <用户指定邮箱>` 签发 mail 主机证书、启用发信并投递一封测试信，再核对收件结果。
5. 登录 SMTP 的临时主机为 `sameframe.top`，激活后切换为 `mail.sameframe.top`，凭据已保存在服务器和同帧的加密配置里。

## 配置校验值

- `/etc/postfix/main.cf`：`c299636d7f84b4837cec2f439594c9742f1f065f8f356f3e62f2d2e85a8bc0fb`。
- `/etc/postfix/master.cf`：`d6aadd96b15e891e226f673f933229477fe579b1a2a0cd80f6b6d4650e424b75`。
- `/etc/opendkim.conf`：`de7e4d2b399b27691fbf381e3e94f6e584b82017f60f075b8c340015de25feff`。
- `/opt/sameframe/renew-tls.sh`：`cb5d4201e50a8f8b8dd7bb1cbd7ae27fa9454a2600fad0b68a65ff16fc3857bc`。


## 参考

- [Google 发件要求](https://support.google.com/mail/answer/81126)：发件认证、TLS、正反向 DNS。
- [Cloudflare 邮件 DNS](https://developers.cloudflare.com/dns/manage-dns-records/how-to/email-records/)：邮件主机的 DNS 记录和代理设置。
- [Postfix 参数](https://manpages.debian.org/trixie/postfix/postconf.5.en.html)、[OpenDKIM 配置](https://manpages.debian.org/trixie/opendkim/opendkim.conf.5.en.html)。
