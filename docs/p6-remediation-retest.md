# P6 缺陷修复与复验记录

复验日期：2026-11-13（Europe/Berlin）

复验对象：本地 `dev/shop` 工作树。该工作树尚无候选提交哈希，项目文件处于未跟踪状态；本记录仅复核原 P6 报告列出的五项缺陷，不能替代独立 Codex 会话的最终验收。

## 修复结果

| 原问题 | 修复与证据 | 复验结论 |
| --- | --- | --- |
| 超时扫单尚未执行时，迟到支付会转待发货 | `internal/app/payments.go` 在同一事务内解析并比较 `close_at`；到期付款关闭订单、释放一次库存、落 `LATE` 收款及迟到支付异常。`TestExpiredPendingPaymentBecomesLateException` 覆盖边界时刻。 | 通过 |
| 买家无法查询运单 | `internal/app/queries.go` 将订单左连接发货记录，`internal/web/templates.go` 在买家订单支付栏展示承运方与运单号。`TestPaymentAndShipmentRecordsAreQueryable` 与 `TestTransactionHandlersEndToEnd` 覆盖查询。 | 通过 |
| 渠道可绕过客服直接重试失败退款 | `internal/app/refunds.go` 仅接受 `PENDING_CHANNEL` 的退款通知；客服的 `RetryRefund` 将 `FAILED` 退款恢复为待渠道处理并写审计。页面增加 `/refunds/retry`。HTTP 回归测试验证直接成功被拒绝，客服重试后才成功。 | 通过 |
| 运营缺少收款、退款、发货记录 | 增加 `Payment` 查询和 `Dashboard.Payments`，运营页增加“交易与履约记录”中的收款、退款、发货表。`TestPaymentAndShipmentRecordsAreQueryable` 与 HTTP 集成测试覆盖记录出现。 | 通过 |
| 退款审核原因可为空 | 服务端对审核和重试原因执行去空白校验并返回 `ErrReasonRequired`；审核和重试输入框标记 `required`。`TestRefundReviewRequiresReason` 覆盖状态不被空原因改变。 | 通过 |

## 质量证据

- `gofmt -w` 已执行于本次涉及的 Go 文件。
- `go vet ./...`：通过。
- `go test -count=1 ./...`：通过，覆盖 `cmd/shop`、`internal/app`、`internal/web`。
- `go test -count=1 -coverpkg=./... -coverprofile=coverage-p6-final.out ./...`：通过；当前 Go 环境生成的 profile 文件名为 `coverage-p6-final`，`go tool cover -func=coverage-p6-final` 显示合并覆盖率 `85.5%`。
- `go test -race -count=1 ./...`：通过。使用 `C:\Users\rainz\Documents\Codex\toolchains\winlibs-16.2.0-ucrt\mingw64\bin\gcc.exe`（MinGW-w64 GCC 16.2.0）并设置 `CGO_ENABLED=1`；`cmd/shop`、`internal/app`、`internal/web` 均通过。执行时有 Go 遥测 token 写入权限告警，但测试命令退出码为 0，未影响编译或测试。

## Skill 复查结果

- `transaction-consistency`：未发现本次五项修复范围内新的高风险状态迁移。过期付款不会转待发货；失败退款必须经客服重试；正常退款与迟到退款仍走不同库存路径。
- `ecommerce-acceptance`：原 P6 的五个 P1/P2 问题均有代码和自动化回归证据，race 检测也已有真实通过结果。独立会话、CI/CD、部署和视频仍缺失，不能据此宣称整体最终验收通过。

## 结论

**五项缺陷的本地修复复验通过，race 检测也已通过。** 最终交付仍需在独立会话中复验；没有 GitHub/CI/CD/部署权限的项目过程项继续保持“无法验证”。
