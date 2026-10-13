# tg115-follow

通过 Telegram 个人账号（gotd/td）监听 [@HDHiveBot](https://t.me/HDHiveBot) 的消息，实现影视资源更新同步与每日自动签到，处理结果通过飞书 / 自建 TG Bot 推送。

## 特性

- 只监听 `@HDHiveBot` 的消息，其余消息在内存中直接丢弃，**不落盘**（仅持久化登录 session 文件）
- 断线自动重连，重连后通过 updates gap 恢复补拉消息
- 每日固定时间 + 随机偏移向机器人发送签到指令
- 二维码 / 手机号两种登录方式
- 支持 SOCKS5 / HTTP CONNECT / MTProxy 代理（配置为数组）
- TOML 配置，Docker Compose 一键部署
- URL 解锁与 115 转存爬虫：后续接入（`internal/listen.Processor` 接口）

## 快速开始

1. 到 <https://my.telegram.org/apps> 申请 `app_id` / `app_hash`
2. 复制配置：

   ```bash
   cp config.example.toml config.toml   # Docker 部署放到 ./data/config.toml
   ```

3. 填写 `telegram.app_id`、`telegram.app_hash`，按需配置代理 / 签到 / 通知

### 本地运行

```bash
go mod tidy
go run ./cmd/tg115-follow serve -config config.toml
```

### Docker Compose

```bash
mkdir -p data && cp config.example.toml data/config.toml  # 编辑配置
docker compose up -d --build
docker compose logs -f   # 默认二维码登录：直接在日志里扫码
```

## 登录方式

### 1. 二维码（默认，推荐）

```bash
docker compose up -d
docker compose logs -f tg115-follow
```

日志中会打印 ASCII 二维码，用手机 Telegram → 设置 → 设备 → 连接桌面设备 扫码即可。若开启了两步验证，需要交互输入密码（见方式 3 的 exec 用法）。

本地：`go run ./cmd/tg115-follow serve`

### 2. 手机号（环境变量）

```bash
LOGIN_METHOD=phone docker compose up
```

compose 配置已开启 `tty` + `stdin_open`，可用 `docker attach tg115-follow` 进入交互输入验证码（退出用 `Ctrl-p Ctrl-q`，不要按 `Ctrl-c`）。

### 3. 手机号 / 二维码（CLI，推荐用于两步验证）

容器保持运行的情况下 exec 进去单独执行登录：

```bash
docker compose exec tg115-follow tg115-follow login -config /data/config.toml -method phone
# 或重新走二维码
docker compose exec tg115-follow tg115-follow login -config /data/config.toml -method qr
```

登录成功后 session 写入 `/data/session.json`，重启容器自动复用，无需再次登录。

## 配置说明

见 `config.example.toml`，主要字段：

| 字段 | 说明 |
| --- | --- |
| `telegram.app_id` / `app_hash` | Telegram 应用凭证 |
| `telegram.session_file` | session 存储路径（唯一持久化文件） |
| `telegram.login_method` | `qr`（默认）/ `phone`，可被 `LOGIN_METHOD` 覆盖 |
| `[[proxy]]` | 代理数组：`type = socks5/http/mtproxy`，当前取第一个生效 |
| `listen.username` | 监听目标，默认 `HDHiveBot` |
| `checkin.time` / `jitter_minutes` / `command` | 每日签到时间与随机偏移 |
| `notify.feishu.webhook` | 飞书自定义机器人 webhook |
| `notify.tgbot.bot_token` / `chat_id` | 自建 TG Bot 推送 |

## 项目结构

```
cmd/tg115-follow/     入口（serve / login 子命令）
internal/config/      TOML 配置加载与校验
internal/tgclient/    gotd 客户端封装：代理拨号、QR/手机号登录、断线重连、gap 恢复
internal/listen/      @HDHiveBot 消息过滤与事件分发（爬虫在此接入）
internal/checkin/     每日定时 + 随机偏移签到
internal/notify/      飞书 / TG Bot 通知
```

## 注意事项

- `session.json` 等同账号凭证，请勿泄露，勿提交到仓库
- 新登录的 session 建议先在常用 IP 环境下养号，降低封号风险
- 后续计划：URL 解锁 → 115 网盘转存 → 结果通知
