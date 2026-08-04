# 测试规范

> 施工 agent 必须按本文档编写测试。不得自由发挥测试策略。

---

## 1. 必须通过的 CI 检查

```bash
go test -race -count=1 ./...      # 全量 race 检测
go test -race -count=10 ./internal/bus/ ./internal/execute/  # 核心模块重复
go vet ./...                       # 静态分析
govulncheck ./...                  # 已知漏洞
```

---

## 2. 单元测试清单

### 2.1 decimalutil

| 文件 | `internal/decimalutil/decimalutil_test.go` |
|------|------------------------------------------|
| 测试 1 | `FromFloat64(1.05000, 5)` 必须等于 `decimal.NewFromString("1.05000")` |
| 测试 2 | `FromFloat64(1.05000, 5)` 再 `ToFloat64` 往返精度不丢失 |
| 测试 3 | 边界：`FromFloat64(0, 0)`、`FromFloat64(-1.5, 1)`、极大值、极小值 |
| 测试 4 | `FromFloat64(1.05, 5)` 必须不等于 `decimal.NewFromFloat(1.05)`（验证不使用直接转换） |

### 2.2 QuoteBus

| 文件 | `internal/bus/quote_bus_test.go` |
|------|----------------------------------|
| 测试 1 | 单 Publish → 单 Subscriber 收到 |
| 测试 2 | 单 Publish → 多 Subscriber 全收到（同一 symbol） |
| 测试 3 | 不同 symbol 的 Publish 互不干扰 |
| 测试 4 | drain-then-replace：快速 Publish 3 条 → Subscriber 读到的是最新（非第一条） |
| 测试 5 | 慢消费者不阻塞 Publish（用 time.After 验证） |
| 测试 6 | 并发：10 goroutine 同时 Publish 不同 symbol，无 data race |
| 测试 7 | Unsubscribe 后不再收到消息 |
| 测试 8 | Snapshot 超时返回部分结果 |

### 2.3 Adapter 重连状态机

| 文件 | `internal/adapter/reconnect_test.go` |
|------|--------------------------------------|
| 测试 1 | Running → Recv error → Reconnecting → 成功 → Running |
| 测试 2 | Reconnecting → backoff 指数增长（验证 1s→2s→4s） |
| 测试 3 | 重连超限（>10 次/分钟）→ emergencyClose 被调用 |
| 测试 4 | INVALID_TOKEN → 完整重连（调用 Connect） |
| 测试 5 | 并发：重连期间 PlaceOrder 被拒绝 |

### 2.4 执行管线

| 文件 | `internal/execute/pipeline_test.go` |
|------|-------------------------------------|
| 测试 1 | 2 腿全部 Filled → 返回 nil |
| 测试 2 | 3 腿全部 Filled → 返回 nil |
| 测试 3 | 1 腿 Filled + 1 腿 Rejected → hedge(filled_leg) 被调用 |
| 测试 4 | 超时 → 所有已填充 leg 被 hedge |
| 测试 5 | Pre-trade revalidation 失败 → 不下单 |
| 测试 6 | 并发下单：所有腿的 PlaceOrder 在不同 goroutine 中被调用 |

### 2.5 熔断

| 文件 | `internal/risk/circuit_breaker_test.go` |
|------|----------------------------------------|
| 测试 1 | 连续 5 笔亏损 → 策略熔断打开 |
| 测试 2 | 连续 4 笔亏损 + 1 笔盈利 → 计数器重置 |
| 测试 3 | 窗口亏损超限 → 策略熔断打开 |
| 测试 4 | 全局日亏损超限 → Check 返回 false |
| 测试 5 | Kill Switch 触发后 → IsKilled 返回 true（内存 + 磁盘） |
| 测试 6 | Kill Switch 重启后 → IsKilled 返回 true（持久化验证） |

### 2.6 幂等去重

| 文件 | `internal/execute/idempotency_test.go` |
|------|----------------------------------------|
| 测试 1 | 同一个 ClientID 两次 PlaceOrder → 第二次返回缓存结果 |
| 测试 2 | SyncFromOrders 恢复缓存 |
| 测试 3 | TTL 过期后 Get 返回 false |

### 2.7 错误码分类

| 文件 | `internal/errclass/errclass_test.go` |
|------|--------------------------------------|
| 测试 1 | REQUOTE → RetryFresh |
| 测试 2 | MARKET_CLOSED → Halt |
| 测试 3 | NO_MONEY → Abort |
| 测试 4 | TOO_MANY_TRADE_REQUESTS → Retry |
| 测试 5 | INVALID_VOLUME → Abort |
| 测试 6 | 所有 MT5 ErrorCode 枚举值都有 case（用 reflect 遍历） |
| 测试 7 | 所有 MT4 ErrorCode 枚举值都有 case |

### 2.8 价差矩阵计算

| 文件 | `internal/dashboard/matrix_test.go` |
|------|-------------------------------------|
| 测试 1 | 2 broker × 2 symbol → 4 格矩阵，值正确 |
| 测试 2 | 绿色判定：净利差 > 成本 |
| 测试 3 | 黄色判定：0 < 净利差 < 成本 |
| 测试 4 | 红色判定：净利差 < 0 |
| 测试 5 | 性能：15 broker × 30 symbol 矩阵计算 < 1ms |

### 2.9 PostgreSQL Store

| 文件 | `internal/store/pg_test.go` |
|------|---------------------------|
| 测试 1 | WriteTicks 批量写入 + 查询验证 |
| 测试 2 | CreateSignal + QuerySignals 按时间范围过滤 |
| 测试 3 | UpsertOrder：幂等更新 |
| 测试 4 | 分区自动创建（调用 create_next_month_partition） |
| 测试 5 | 连接池耗尽时 Acquire 超时 |

### 2.10 审计日志

| 文件 | `internal/audit/audit_test.go` |
|------|-------------------------------|
| 测试 1 | Log → 读回 → 内容一致 |
| 测试 2 | 并发写入不损坏文件 |
| 测试 3 | 不使用 encoding/json（grep 验证 import） |

---

## 3. 基准测试

| 文件 | `internal/bus/bench_test.go` |
|------|------------------------------|
| Bench 1 | `BenchmarkPublish`：1 个 subscriber，1M Publish，报告 ns/op 和 allocs/op |
| Bench 2 | `BenchmarkPublishMultiSub`：10 个 subscriber |

| 文件 | `internal/execute/bench_test.go` |
|------|----------------------------------|
| Bench 3 | `BenchmarkRevalidation`：3 腿预成交校验，报告 allocs/op（必须为 0） |

---

## 4. 集成测试

### 4.1 mtapi Demo 连接

| 文件 | `test/integration/mt5_connect_test.go` |
|------|----------------------------------------|
| 标签 | `//go:build integration` |
| 测试 1 | Connect → 获取 token → 验证非空 |
| 测试 2 | Subscribe EURUSD → OnQuote stream 收到 tick |
| 测试 3 | AccountSummary → 验证 FreeMargin > 0 |
| 测试 4 | OrderSend 0.01 lot → 验证 Ticket != 0 → OrderClose |
| 测试 5 | 断线模拟（关闭 conn）→ Reconnecting → 重连成功 |

### 4.2 DashboardService e2e

| 文件 | `test/integration/dashboard_test.go` |
|------|--------------------------------------|
| 测试 1 | SpreadMatrix stream 收到有效数据 |
| 测试 2 | PositionWatch stream 收到持仓 |
| 测试 3 | SubmitOrder → 验证 ClientID → ClosePosition |

---

## 5. 前端测试（desk — Svelte）

### 5.1 组件测试（Vitest + jsdom）

施工 agent 在 `frontend/` 目录下运行 `npx vitest run`。

| 文件 | `frontend/tests/components.test.js` |
|------|-------------------------------------|
| 测试 1 | `Card` 组件渲染 → 验证 `.card` CSS class 存在 |
| 测试 2 | `StatCard` 接收 props → 验证 value/label 文本内容 |
| 测试 3 | `Skeleton` 组件 → 验证 `.skeleton.animate` 有 `shimmer` animation |
| 测试 4 | `DataTable` 接收 rows → 验证 `<tr>` 数量匹配 |

| 文件 | `frontend/tests/stores.test.js` |
|------|---------------------------------|
| 测试 5 | `spreadMatrix` store 更新 → 订阅者收到新值 |
| 测试 6 | `positions` store 更新 → 订阅者收到新值 |

### 5.2 E2E 测试（Playwright + headless Chromium）

施工 agent 在 `frontend/` 目录下运行 `npx playwright test`。

| 文件 | `frontend/tests/visual.spec.js` |
|------|---------------------------------|
| 测试 7 | 5 个 Tab 全部渲染 |
| 测试 8 | 统计卡片区域有 4 个 stat-card |
| 测试 9 | 价差矩阵渲染 broker 卡片 + 品种单元格 |
| 测试 10 | `.card` 有 `backdrop-filter: blur()` 样式 |
| 测试 11 | 套利单元格 `color: rgb(52, 199, 89)`（绿色 #34c759） |
| 测试 12 | 亏损单元格 `color: rgb(255, 69, 58)`（红色 #ff453a） |
| 测试 13 | hover 时卡片产生 `translateY` transform |
| 测试 14 | 骨架屏有 `shimmer` 动画 |
| 测试 15 | 持仓表格渲染订单数据 |
| 测试 16 | 正 PnL 显示绿色 |
| 测试 17 | `wails.Call("SubmitOrder", ...)` 返回 ticket > 0 |
| 测试 18 | `wails.Call("SubmitOrder", {})` 抛出错误 |
| 测试 19 | 价差矩阵 Tab 截图 > 10KB（非空白） |
| 测试 20 | 持仓 Tab 截图 > 5KB（非空白） |

### 5.3 IPC 抽象层验收（施工 agent 自查）

```
[ ] grep -r "wails\." frontend/src/tabs/      → 返回空（组件不直接调 wails）
[ ] grep -r "wails\." frontend/src/components/ → 返回空
[ ] grep -r "wails\." frontend/src/lib/backend.js → 所有调用集中在此文件
[ ] 浏览器窗口 375px 宽度下所有 Tab 内容可见、无水平滚动条
```

### 5.4 CI 集成

```makefile
.PHONY: test-frontend

test-frontend:
	cd frontend && npx vitest run
	cd frontend && npx playwright test
```

## 6. 检查清单（施工 agent 自查）

```
[ ] 所有测试文件使用 go test -race 通过
[ ] 无任何 import "encoding/json"
[ ] 无任何 import "github.com/gorilla/websocket"
[ ] 无任何 import gin/echo/mux
[ ] 无任何 import "sync/atomic" 用于非简单类型
[ ] 热路径基准测试 allocs/op = 0
[ ] 错误码分类 switch 覆盖所有枚举值
[ ] 集成测试有 //go:build integration 标签
[ ] go vet 无警告
[ ] govulncheck 无已知漏洞
```
