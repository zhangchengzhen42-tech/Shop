# Skill 使用记录

本文件只记录本项目中实际执行过的 Skill，不将未调用的工具列为已使用。

## transaction-consistency

- 名称、来源与版本：自编 Skill，`C:\Users\rainz\.codex\skills\transaction-consistency\SKILL.md`，当前本地版本。
- 使用位置：P4 开发阶段，在补充核心交易测试后执行；检查输出见 `docs/transaction-consistency-p4-check.md`。
- 使用原因：订单、库存、支付、退款、发货和迟到支付共享状态，必须核对事务边界和幂等行为。
- 使用过程：输入题面规则、`internal/app` 服务代码、SQLite schema、测试文件和 `go test ./...` 结果；逐项检查原子库存占用、状态迁移、唯一约束、退款/发货竞争和异常审计。
- 实际帮助：检查发现模板样式未包在 `<style>` 中，以及固定服务端时间下订单 ID 仅依赖时间戳存在碰撞风险；已分别修复模板渲染和 `newID` 随机熵，并通过 HTTP 冒烟和并发测试复测。
- 证据及局限：证据为 `internal/app/service_test.go`、`internal/app/store.go`、`internal/web/templates.go`、`docs/transaction-consistency-p4-check.md`；当前竞态检测因环境无 CGO/gcc 未执行，数据库使用单连接池，尚未覆盖多进程 SQLite 竞争。

### P5 复查

- 使用位置：第五步测试与质量门禁；输出为 `docs/transaction-consistency-p5-check.md`。
- 使用原因：新增故障注入、HTTP 集成和并发测试后，需要再次确认没有引入库存、付款、退款或发货的非法状态迁移。
- 使用过程：检查 `internal/app/service_test.go` 的并发、重复通知、审计失败回滚和迟到支付场景，以及 `internal/web/server_test.go` 的身份和角色边界。
- 实际帮助：确认审计失败会回滚事务，且 HTTP Handler 仍仅通过领域服务改变状态；发现的唯一未闭合项是 race 环境缺少 gcc，已真实记录。
- 证据及局限：`docs/transaction-consistency-p5-check.md`、`docs/p5-quality-gate.md`；未覆盖多进程 SQLite 和 race。

## ecommerce-acceptance

- 名称、来源与版本：自编 Skill，`C:\Users\rainz\.codex\skills\ecommerce-acceptance\SKILL.md`，当前本地版本。
- 使用位置：P4 阶段收尾验收；检查输出见 `docs/ecommerce-acceptance-p4-check.md`。
- 使用原因：题面要求同时核对业务场景、角色权限、覆盖率、race、Skill 记录和外部过程证据，不能只看单元测试。
- 使用过程：输入题面、项目 `AGENTS.md`、当前分支文件、测试/vet/覆盖率输出、HTTP 冒烟结果和环境权限限制；逐项标记通过、未通过或无法验证。
- 实际帮助：确认功能链路已有可复现证据，同时明确覆盖率 74.8% 未达到 85%、race 未执行、GitHub MCP/CI/部署/工单权限不存在，避免把开发阶段结果误报为最终交付通过。
- 证据及局限：证据为 `docs/ecommerce-acceptance-p4-check.md`、`README.md` 和 `docs/p4-development-status.md`；未执行真实远程 PR、CI、部署和视频演示，因此对应项只能标记无法验证。

### P5 复查

- 使用位置：第五步本地门禁复查；输出为 `docs/ecommerce-acceptance-p5-check.md`。
- 使用原因：需要把单包覆盖率、合并覆盖率、race、外部权限和五类业务场景放在同一验收表中核对。
- 使用过程：读取测试输出和 coverage profile，核对 app 85.0%、web 87.7%、整体 85.7%，同时记录 race、CI、部署的不可验证原因。
- 实际帮助：将本地测试通过与最终交付验收明确区分，避免因缺少 gcc 或远程权限而误报最终通过。
- 证据及局限：`docs/ecommerce-acceptance-p5-check.md` 和 `docs/p5-quality-gate.md`；不是独立会话的最终验收。

### P6 验收

- 使用位置：第六步候选版本验收；输出为 `docs/p6-independent-acceptance.md`。
- 使用原因：需要依据题面逐项复核，而不是复述开发和测试阶段的通过结论。
- 使用过程：冻结 `dev/shop` 工作树，复跑 `go vet ./...` 和 `go test -count=1 ./...`，复核 85.7% 合并覆盖率，审阅支付/退款/页面代码，并执行本地 HTTP 发货与买家查询运单冒烟。
- 实际帮助：发现支付未校验订单有效期、买家运单缺失、客服重试缺失、运营记录查询缺失和审核原因可为空五项问题，结论调整为不通过。
- 证据及局限：`docs/p6-independent-acceptance.md`；该次验收当时并非独立 Codex 会话，且当时缺少 gcc、远程 CI/CD 与部署权限，均已在报告中标记。gcc 问题已在后续复验中解决。

## skill-creator

- 名称、来源与版本：既有 Skill，`C:\Users\rainz\.codex\skills\.system\skill-creator\SKILL.md`。
- 使用位置：创建本题两个自编 Skill 时使用；文件为 `C:\Users\rainz\.codex\skills\transaction-consistency\SKILL.md` 和 `C:\Users\rainz\.codex\skills\ecommerce-acceptance\SKILL.md`。
- 使用原因：题面要求每题至少两个可被 AI 工具使用的专项 Skill，并要求用途、输入、步骤、输出、失败处理和完成标准完整。
- 使用过程：按 Skill 结构补齐触发场景、输入、检查步骤、输出格式、失败处理和完成标准，并执行本地快速校验。
- 实际帮助：两个 Skill 均通过本地格式校验，并可在本次 P4 检查中实际使用。
- 证据及局限：校验输出及文件位于上述 Skill 路径；Skill 本身不替代 Go 测试、部署或真实协作平台证据。

## computer-use

- 名称、来源与版本：既有 Skill，`C:\Users\rainz\.codex\plugins\cache\openai-bundled\computer-use\26.1106.51428\skills\computer-use\SKILL.md`。
- 使用位置：前端原型和 Figma 页面观察阶段，不是当前 Go 核心实现阶段。
- 使用原因：需要观察用户提供的 Figma 页面结构并核对原型迁移方向。
- 使用过程：打开用户提供的页面，观察角色导航、工作台卡片和操作入口，再据此形成轻量 HTML 原型。
- 实际帮助：为后续页面布局提供参考；没有将浏览器观察当作后端业务验收证据。
- 证据及局限：原型文件及相关记录在 `C:\Users\rainz\Documents\Codex\2026-11-13\new-chat\outputs`；Figma 页面不作为当前本地 Go 代码的版本依赖。

## P6 缺陷修复与复验

### transaction-consistency

- 名称、来源与版本：自编 Skill，`C:\Users\rainz\.codex\skills\transaction-consistency\SKILL.md`，当前本地版本。
- 使用位置：P6 验收发现问题后的修复与复验阶段；输出见 `docs/p6-remediation-retest.md`。
- 使用原因：原验收发现付款有效期、退款失败重试和库存/异常路径存在高风险业务缺陷，修复后需要重新检查状态迁移与库存规则。
- 使用过程：输入原 P6 缺陷清单、`internal/app/payments.go`、`internal/app/refunds.go`、`internal/app/queries.go`、Web Handler/模板及新增回归测试；逐项复核到期付款关闭并释放一次库存、迟到支付异常不恢复库存、失败退款仅能由客服重试、正常/迟到退款的库存差异。
- 实际帮助：指导将 `close_at` 判断纳入付款事务，并阻止渠道直接把 `FAILED` 退款通知为成功；新增三项领域回归测试和一项 HTTP 端到端断言。
- 证据及局限：`docs/p6-remediation-retest.md`、`internal/app/service_test.go`、`internal/web/server_test.go`；已使用 WinLibs GCC 16.2.0 运行并通过 `go test -race -count=1 ./...`，但仍未覆盖多进程 SQLite 竞争。

### ecommerce-acceptance

- 名称、来源与版本：自编 Skill，`C:\Users\rainz\.codex\skills\ecommerce-acceptance\SKILL.md`，当前本地版本。
- 使用位置：P6 缺陷修复后的局部验收复查；输出见 `docs/p6-remediation-retest.md`。
- 使用原因：需要验证修复不仅改变了代码，还覆盖买家运单查询、客服重试入口、运营收退款/发货记录和审核原因的可操作性。
- 使用过程：读取原 P6 报告的五项问题，对照领域代码、页面模板、HTTP 集成测试、`go vet ./...`、`go test -count=1 ./...` 和覆盖率结果逐项确认。
- 实际帮助：要求保留原 P6 的历史“不通过”报告，并把本次结论限定为五项缺陷的本地复验通过，避免将同会话复查、缺少 race/CI/部署证据误报为最终独立验收通过。
- 证据及局限：`docs/p6-independent-acceptance.md`、`docs/p6-remediation-retest.md`、`coverage-p6-final`；race 已有通过证据，独立会话、远程 CI/CD、部署和视频证据仍缺失。
