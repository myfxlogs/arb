# 开发环境与工作流

> 施工 agent 的唯一操作手册。本文档定义如何搭建环境、构建、运行、提交。

---

## 1. 环境要求

```
Go 1.22+
Node.js 22+          (desk 前端构建，仅构建时)
PostgreSQL 15+
Buf CLI 1.x          (proto 代码生成)
grpcurl              (调试用)
Docker + Docker Compose (开发环境)
```

### 1.1 快速启动

```bash
# 启动 PostgreSQL（Docker）
docker run -d --name arb-pg \
  -e POSTGRES_USER=arb \
  -e POSTGRES_PASSWORD=arb \
  -e POSTGRES_DB=arb \
  -p 5432:5432 \
  postgres:16-alpine

# 运行迁移
psql "postgres://arb:arb@localhost:5432/arb?sslmode=disable" \
  -f migrations/001_init.sql

# 生成 proto 代码
buf generate

# 下载 Go 依赖
go mod tidy

# 下载前端依赖
cd frontend && npm install && cd ..

# 运行测试
go test -race ./...

# 启动 core（守护进程）
go run ./cmd/core -config=config/default.textproto

# 构建并启动 desk（桌面应用，在另一个终端）
cd frontend && npm run build && cd ..
go run ./cmd/desk
```

---

## 2. 项目初始化

施工 agent 第一步：创建 `go.mod`。

```bash
go mod init arb
```

`go.mod` 需要包含的依赖（施工 agent 执行 `go mod tidy` 后自动填充）：

```
module arb

go 1.22.0

require (
    github.com/wailsapp/wails/v3 v3.x
    github.com/jackc/pgx/v5 v5.x
    github.com/shopspring/decimal v1.x
    google.golang.org/grpc v1.x
    google.golang.org/protobuf v1.x
    golang.org/x/time v0.x
)
```

---

## 3. config/default.textproto

施工 agent：创建此文件作为默认配置模板。

```textproto
# ARB 系统配置
# 施工 agent：复制此文件并填入真实的 broker 凭证

brokers: [
  {
    name: "OctaFX-Demo"
    platform: PLATFORM_TYPE_MT5
    host: "78.140.180.198"
    port: 443
    user: 0                         # ← 替换为真实账号
    password: ""                    # ← 替换为真实密码
  }
]

strategies: [
  {
    type: STRATEGY_TYPE_TRIANGULAR
    enabled: true
    max_slippage_bps: 0.5
    order_timeout: { seconds: 0 nanos: 500000000 }    # 500ms
    subscribed_symbols: ["EURUSD", "GBPUSD", "EURGBP"]
  },
  {
    type: STRATEGY_TYPE_CROSS_EXCHANGE
    enabled: false
    max_slippage_bps: 1.0
    order_timeout: { seconds: 0 nanos: 500000000 }
    subscribed_symbols: []                             # 全部可用
  }
]

risk: {
  max_notional_per_trade: 10000     # $10,000 单笔上限
  max_consecutive_losses: 5
  max_window_loss: 500              # $500
  daily_loss_limit: 5000            # $5,000
  max_drawdown_pct: 20              # 20%
  max_concurrent_orders: 5
  rate_limit_initial: 10            # 10 req/s
}

database: {
  dsn: "postgres://arb:arb@localhost:5432/arb?sslmode=disable"
}

dashboard: {
  listen_address: "127.0.0.1:50051"
  matrix_refresh_ms: 100
}
```

---

## 4. 配置文件加载

施工 agent 实现：

```go
package config

import (
    "os"
    "google.golang.org/protobuf/encoding/prototext"
    configpb "arb/proto/config"
)

func Load(path string) (*configpb.SystemConfig, error) {
    b, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    cfg := &configpb.SystemConfig{}
    if err := prototext.Unmarshal(b, cfg); err != nil {
        return nil, err
    }
    return cfg, nil
}
```

**注意**：使用 `prototext.Unmarshal`（protobuf text format），不使用 `encoding/json`。

---

## 5. 凭证管理

施工 agent：凭证不放在 config 文件中（config 可能进 git）。通过环境变量覆盖：

```go
// internal/adapter/credentials.go
func LoadCredentials(cfg *configpb.BrokerConfig) (user int64, password string) {
    envUser := os.Getenv(fmt.Sprintf("ARB_%s_USER", strings.ToUpper(cfg.Name)))
    envPass := os.Getenv(fmt.Sprintf("ARB_%s_PASSWORD", strings.ToUpper(cfg.Name)))

    user = cfg.User
    password = cfg.Password

    if envUser != "" { user, _ = strconv.ParseInt(envUser, 10, 64) }
    if envPass != "" { password = envPass }

    return
}
```

`.env` 文件用于本地开发（必须在 `.gitignore` 中）：

```
ARB_OCTAFX_DEMO_USER=62333850
ARB_OCTAFX_DEMO_PASSWORD=yourpassword
```

---

## 6. Git 工作流

```
分支：main（主分支）
提交粒度：每个完整的模块一次提交
提交信息格式：module: description

例如：
  adapter: implement MT5Adapter Connect and QuoteStream
  bus: implement QuoteBus with drain-then-replace
  execute: implement 4-phase execution pipeline
  dashboard: implement SpreadMatrix stream server
  desk: implement Wails Go backend with gRPC stream bridge
  frontend: implement Svelte SpreadMatrix tab with liquid glass cards
  store: implement PostgreSQL tick writer with COPY
```

**.gitignore**：
```
.env
.kill_switch
bin/
proto/gen/
*.test
```

---

## 7. Docker Compose（开发环境）

```yaml
# docker-compose.yml
version: "3.9"
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: arb
      POSTGRES_PASSWORD: arb
      POSTGRES_DB: arb
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
      - ./migrations:/docker-entrypoint-initdb.d

volumes:
  pgdata:
```

---

## 8. 实施顺序

施工 agent 严格按此顺序：

```
Phase 1: 基础
  1. go.mod + go.sum
  2. proto 代码生成 (buf generate)
  3. decimalutil (零外部依赖)
  4. QuoteBus (零外部依赖)
  5. 核心类型定义

Phase 2: Adapter
  6. PlatformAdapter 接口
  7. MT5Adapter Connect + QuoteStream
  8. MT4Adapter Connect + QuoteStream
  9. 重连状态机
  10. 凭证加载

Phase 3: 执行
  11. 错误码分类
  12. 幂等去重缓存
  13. 执行管线（4-phase）
  14. OrderExecutor（channel 信号量）

Phase 4: 风控
  15. 资金门禁
  16. 自适应限流
  17. 策略熔断 + 全局熔断
  18. Kill Switch

Phase 5: 存储
  19. PostgreSQL 连接池
  20. Tick 批量写入 (COPY)
  21. Signals/Orders/Daily CRUD

Phase 6: Dashboard
  22. DashboardService gRPC server
  23. 价差矩阵计算
  24. 持仓聚合

Phase 7: 桌面
  25. Wails 应用骨架 + Go 绑定函数 (desk/app.go)
  26. Svelte 项目初始化 + 液态玻璃主题 (frontend/)
  27. 价差矩阵 Tab (Go 数据层 + Svelte 渲染)
  28. 持仓 Tab (Go 数据层 + Svelte 渲染)
  29. 交易 Tab (Go 数据层 + Svelte 渲染)
  30. 历史查询 Tab (Go 数据层 + Svelte 渲染)
  31. 管理 Tab (Go 数据层 + Svelte 渲染)

Phase 8: 入口 + 集成
  32. cmd/core/main.go
  33. cmd/desk/main.go
  34. 配置文件加载
  35. 审计日志
  36. 集成测试 + 基准测试
  37. Dockerfile + Makefile
```

---

## 9. 禁止事项

```
[ ] 禁止跳过任何 Phase，严格按序实施
[ ] 禁止使用 encoding/json
[ ] 禁止使用任何 HTTP/WebSocket 库
[ ] 禁止创建 REST API endpoint
[ ] 禁止在 desk 内启动 HTTP server
[ ] 禁止前端直接发起网络请求（全部通过 Wails IPC → Go 后端）
[ ] 禁止引入 TypeScript（Svelte 用 JS）
[ ] 禁止在 Hot Path 上进行堆分配
[ ] 禁止使用 sync.Mutex 在热路径
[ ] 禁止使用 goroutine pool
[ ] 禁止直接调用 decimal.NewFromFloat
[ ] 禁止在 Hot Path 使用 decimal 类型
[ ] 禁止裸 grep/find/cat/wc/ls — 用 Read/Grep/Glob 工具
[ ] 配置文件绝不提交到 git
```

## 10. Before Commit

每次提交前必须通过的机械检查清单。施工 agent 自己跑，不依赖 CI 反馈。

```bash
go build ./...                              # 编译
go test -race -count=1 ./...                # 全量 race
go vet ./...                                # 静态分析
go run ./tools/check-file-lines --strict    # 文件规模（🔴 Go>450 行阻断）
govulncheck ./...                           # 已知漏洞
```

**施工 agent 注意**：`check-file-lines` 工具需要自己实现。逻辑很简单：
- 遍历所有 `.go` 文件（跳过 `proto/gen/`、`_test.go`）
- 任一行数 > 450 → 输出文件名 + 行数 + 返回非零 exit code
- 目标：保持每个文件在 AI agent 能一次读完的范围内
