# Shop 小型电商交易系统

当前实现覆盖订单、库存占用、支付通知、售前退款、迟到支付、仓库发货、运营维护和操作审计。

## 本地运行

```powershell
go test ./...
go vet ./...
go run ./cmd/shop -addr :8080 -db shop.db
```

打开 `http://localhost:8080/login`，使用页面内置演示账号切换角色。

角色包括买家、运营、仓库、客服和模拟支付渠道。模拟渠道页面可以触发有效/无效验签、金额不符、迟到支付、退款失败和成功重试。

## 当前验证状态

- `go test ./...`：通过。
- `go vet ./...`：通过。
- 合并整体覆盖率：85.5%。
- `go test -race -count=1 ./...`：通过，使用 WinLibs GCC 16.2.0。
- GitHub Actions 和容器部署定义见 `.github/workflows/quality-gate.yml`、`Dockerfile` 与 `docs/p7-git-pr-deploy.md`；公开 PR 与部署仍需可用的 GitHub/云平台连接和授权。

## 容器部署

```powershell
docker build -t shop-transaction:local .
docker run --rm -p 8080:8080 -v "${PWD}/data:/data" -e SHOP_COOKIE_SECRET="请设置随机长密钥" shop-transaction:local
```

容器启动后使用 `http://localhost:8080/healthz` 验证健康状态。生产环境必须使用持久化的 `/data` 卷和强随机 `SHOP_COOKIE_SECRET`。

## 数据边界

SQLite 负责持久化业务状态，订单保存商品名称、单价和总额快照。订单、库存、支付、退款、异常和审计记录使用事务及唯一约束保持幂等。
