# 电商交易系统验收报告

验收版本：本地 `dev/shop` 分支，未推送远程。
环境与数据：Windows 本地，Go + SQLite，HTTP 冒烟地址 `http://localhost:18080`，预置 2 个商品、3 个买家和员工/渠道演示账号。

| 验收项 | 结论 | 证据 | 备注 |
| --- | --- | --- | --- |
| 下单占库存与价格快照 | 通过 | `internal/app/service_test.go` | 并发及改价后快照均有测试 |
| 超时关闭和重复释放 | 通过 | `TestValidationTimeoutAndOperationalQueries` | 服务端时钟 |
| 支付验签、金额、渠道交易号和重复通知 | 通过 | `TestPaymentValidationAndRefundRetry`、`TestPaymentDuplicateIsIdempotent` | 无效结果持久化 |
| 迟到支付和迟到退款 | 通过 | `TestLatePaymentCreatesExceptionWithoutInventoryRecovery`、`TestValidationTimeoutAndOperationalQueries` | 不恢复库存 |
| 售前退款失败重试与库存恢复 | 通过 | `TestRefundSuccessRestoresStockOnce`、`TestPaymentValidationAndRefundRetry` | 成功只恢复一次 |
| 退款/发货竞争及一次发货 | 通过 | `TestRefundRequestAndShipmentHaveOneLegalWinner`、`TestRefundRejectedAllowsShipment` | 条件更新互斥 |
| 角色权限和操作页面 | 有条件通过 | `internal/web/auth.go`、HTTP 冒烟 | 已验证签名 Cookie；缺少浏览器完整回归 |
| 20 并发场景 | 通过 | `TestConcurrentOrdersDoNotOversell` | 本地单进程测试 |
| 核心包覆盖率 >=85% | 不通过 | `coverage.out`、`go tool cover -func coverage.out` | 当前 74.8% |
| go vet | 通过 | `go vet ./...` | 已执行 |
| go test -race | 无法验证 | 命令输出 | CGO_ENABLED=0，无 gcc |
| GitHub MCP、CI、部署、真实工单/Meegle | 无法验证 | 环境权限说明 | 未伪造过程证据 |

## 结论

有条件通过开发阶段检查，不可作为最终交付验收通过。后续必须补到核心包 85% 覆盖率、提供可用 CGO/race 环境，并在有权限的环境完成远程、CI、部署和演示证据。
