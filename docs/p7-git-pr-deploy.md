# P7 Git、PR 与部署记录

日期：2026-11-13（Europe/Berlin）

## 本地交付准备

- Git 远程已配置为 `https://github.com/zhangchengzhen42-tech/Shop.git`。
- 增加 `Dockerfile`：以 Go 1.27 构建 Linux 二进制，在容器的 `/data/shop.db` 使用 SQLite 持久化数据。
- 增加 `.github/workflows/quality-gate.yml`：格式检查、`go vet`、无缓存测试、race 检测和不低于 85% 的合并覆盖率门禁。
- 增加 `.dockerignore`，并更新 `.gitignore` 排除 SQLite 数据库、覆盖率产物和安装器临时输出。
- 健康检查端点：`GET /healthz`，预期返回 `200` 和 `ok`。
- 与 Docker 构建阶段一致的 `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" ./cmd/shop` 已在本机通过。
- 本地服务以 `:18081` 启动后，`GET http://127.0.0.1:18081/healthz` 返回 `200 ok`，测试进程已停止。

## 容器运行方式

```powershell
docker build -t shop-transaction:local .
docker run --rm -p 8080:8080 -v "${PWD}/data:/data" -e SHOP_COOKIE_SECRET="请设置随机长密钥" shop-transaction:local
```

打开 `http://localhost:8080/login`，并用 `http://localhost:8080/healthz` 进行健康检查。

生产环境必须配置强随机的 `SHOP_COOKIE_SECRET`，且必须为 `/data` 提供持久化卷；不应使用程序内置的本地演示 Cookie 密钥。

## 外部执行状态

| 项目 | 状态 | 证据或阻塞 |
| --- | --- | --- |
| Git 本地提交 | 已完成 | `daabbd7a feat(transaction): 完成交易系统交付`，当前 Git 配置为 `rainz <you@example.com>`。 |
| GitHub 推送与 PR | 无法执行 | 终端无法连接 `github.com:443`，且本机没有 `gh`。未强推或覆盖远程分支。 |
| GitHub Actions 执行 | 待推送后触发 | 工作流已就绪，当前无法推送触发。 |
| Docker 容器构建/运行 | 无法执行 | Docker CLI 已安装，但 Docker Desktop 守护进程启动 60 秒后仍未就绪；系统 WSL 服务此前返回访问被拒绝。 |
| 公开部署 | 无法执行 | 当前没有可用的云部署账户、令牌或目标服务，不能伪造部署地址。 |

## 后续操作

1. 配置有效 Git 提交邮箱并恢复到 GitHub 的网络连接。
2. 创建本地提交，推送 `dev/shop`，在 GitHub 创建到默认分支的 PR。
3. 等待 `quality-gate` 通过后，在具备持久化卷和环境变量管理的容器平台部署镜像。
4. 记录部署 URL、健康检查结果、PR 链接和 Actions 运行链接到本文件。
