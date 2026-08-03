# 项目约束文档

> 版本：v3.0  
> 日期：2026-08-03  
> 施工 agent：Claude Code（Sonnet/Haiku mode）  
> 违反任何一条约束的代码不得合并。

---

## 一、通信协议

### 1.1 允许

```
✅ gRPC (HTTP/2 + Protobuf) — 唯一通信协议
✅ protobuf 二进制序列化
✅ Core ↔ Desk：gRPC over local TCP (protobuf)
✅ Core ↔ MT4/MT5：gRPC over TLS → mt4grpc3.mtapi.io:443 / mt5grpc3.mtapi.io:443
✅ 铁律：gRPC Dial 目标 = mtapi 网关，ConnectRequest.Host = broker 真实地址（二者永远不同）
✅ 未来 Binance adapter：内部是 gRPC，Binance 侧由 adapter 封装
```

### 1.2 禁止

```
❌ REST API (HTTP/1.1 + JSON) — 任何场景
❌ JSON 序列化 — 任何场景（包括 encoding/json）
❌ WebSocket — 任何场景
❌ gRPC-Web — 不需要，Fyne 桌面应用直接调 gRPC
❌ 浏览器前端 — 任何场景
❌ Server-Sent Events
❌ GraphQL
❌ 任何文本格式的 RPC
```

---

## 二、存储

### 2.1 允许

```
✅ PostgreSQL 15+ — 时序数据、信号、订单、PnL
✅ 本地文件 — 配置文件 (protobuf text format)、审计日志 (protobuf)、熔断状态 (.kill_switch)
```

### 2.2 禁止

```
❌ Redis — gRPC stream 替代 pub/sub；内存替代缓存；不需要分布式锁
❌ SQLite — 时序查询性能不足；15 broker 并发写入时 WAL 单写者瓶颈
❌ 任何 NoSQL/MQ 中间件（Kafka/RabbitMQ/NATS）
```

---

## 三、前端

### 3.1 允许

```
✅ Fyne (Go GUI) — 桌面窗体应用
✅ Tab 切换：价差矩阵 / 持仓 / 交易 / 历史
✅ 直接从 Core 调 gRPC stub（同一个 Go 代码库）
```

### 3.2 禁止

```
❌ 浏览器 / Electron / WebView
❌ JavaScript / TypeScript
❌ TUI / 终端界面
❌ Wails / Qt / GTK — CGO 依赖，交叉编译问题
```

---

## 四、数据精度

### 4.1 三层模型

```
Hot Path:  float64 — 仅乘、除、比较、临时减
Warm Path: shopspring/decimal — 下单量、保证金
Cold Path: shopspring/decimal — PnL、报表、历史
```

### 4.2 存储精度

```
PG: NUMERIC(20,8) — 所有价格/金额列
Go: decimal.Decimal — Warm/Cold Path
时间: int64 ts_unix_ms（毫秒级 UTC Unix 时间戳）
```

### 4.3 禁止

```
❌ float32
❌ float64 在 Warm/Cold Path
❌ decimal.NewFromFloat() 直接调用（统一走 decimalutil.FromFloat64）
❌ JSON number 传输金额
```

---

## 五、并发

### 5.1 允许

```
✅ goroutine + channel
✅ buffered channel 信号量
✅ sync.RWMutex
✅ sync/atomic
✅ context.Context
```

### 5.2 禁止

```
❌ 第三方 goroutine pool (ants/conc)
❌ sync.Mutex 在热路径
❌ 裸 go 无界 goroutine
❌ sync.Map
```

---

## 六、文件与函数规模

**原则**：按语义域拆分优先，行数为软性参考。目标：AI agent 能一次读完并理解一个完整文件。

| Language | 软性参考 | 硬性红线 |
|----------|---------|---------|
| Go | 300 行/文件，50 行/函数 | 450 行/文件 |

- 拆分前先判断：是否有明确的功能边界（CRUD/生命周期/实体类型）？有 → 拆。没有 → 保持内聚。
- 硬性红线：Go > 450 行必须拆分（AI agent 阅读理解明显退化）。
- 自动生成代码（`proto/gen/`）、测试文件豁免。
- CI：`go run ./tools/check-file-lines --strict`（🔴 阻断提交）。

---

## 七、Push-First 架构

- **gRPC streaming（server-push）是默认数据分发模式。**
- ❌ 禁止 polling / `time.Ticker` 轮询 — 仅当数据源无 push 能力且对延迟不敏感时例外。
- ❌ 数据源有 stream 等效物时绝不 poll（如 MT5 `OnQuote` stream 优于轮询 `GetQuote`）。
- ✅ 引入新数据 feed 时第一问："能做成 stream 吗？" 能 → 做。

---

## 八、依赖

### 8.1 允许

| 包 | 用途 |
|----|------|
| `shopspring/decimal` | Warm/Cold Path 精度 |
| `google.golang.org/grpc` | 唯一通信 |
| `google.golang.org/protobuf` | 序列化 |
| `fyne.io/fyne/v2` | 桌面 UI |
| `github.com/jackc/pgx/v5` | PostgreSQL driver |
| `golang.org/x/time/rate` | 自适应限流 |
| `log/slog` | 结构化日志 |

### 8.2 禁止

```
❌ encoding/json
❌ gorilla/websocket
❌ go-resty/resty
❌ gin/echo/gorilla/mux（任何 HTTP 框架）
❌ go-redis/redis
❌ mattn/go-sqlite3、modernc.org/sqlite
```

---

## 九、代码组织

```
cmd/
  core/main.go        # 守护进程入口
  desk/main.go        # 桌面应用入口
internal/
  adapter/            # PlatformAdapter + MT4/MT5 实现
  bus/                # QuoteBus
  engine/             # 策略引擎
  execute/            # 执行管线 + 幂等去重
  risk/               # 资金门禁 + 熔断 + Kill Switch
  audit/              # 审计日志
  decimalutil/        # float64↔decimal 统一转换
  errclass/           # 错误码分类
  dashboard/          # DashboardService gRPC server
  store/              # PostgreSQL 读写
desk/
  matrix/             # 价差矩阵 Tab
  positions/          # 持仓 Tab
  trading/            # 交易 Tab
  history/            # 历史查询 Tab
proto/
  config/             # 配置 schema
  dashboard/          # DashboardService schema
migrations/           # PG DDL
```

命名：
```
✅ 接口：名词（PlatformAdapter, QuoteBus）
✅ 方法：动词（Connect, Publish, Evaluate）
✅ 文件：小写下划线（quote_bus.go）
❌ 包名包含 util/common/misc/helper
❌ 接口名 I 前缀
```

---

## 十、编译与部署

```
✅ Go 1.22+
✅ Linux amd64
✅ CGO_ENABLED=0（core）；无 CGO（desk 除外，Fyne 需要）
✅ Docker 容器化（core）
✅ Fyne 桌面应用本地编译安装（desk）
✅ TLS 1.3
✅ GOTRACEBACK=0
✅ ulimit -c 0
✅ GOGC=50
✅ go test -race 强制
✅ CI 跑 govulncheck
```

---

## 十一、语言 + 零容忍禁止

### 11.1 语言

```
✅ 全项目唯一语言：Go
❌ 禁止 TypeScript/JavaScript/Python/任何第二语言
```

### 11.2 零容忍

```
❌ 硬编码凭证 / .env 提交到 git
❌ //nolint / // @ts-ignore / // #nosec（修代码，不压制 linter）
❌ 因困难而妥协最优解 — 遇到阻碍必须回到根因找到正确修复方式；
    快捷方式（回退代替重构、标记 legacy 代替移除、沉默代替修复）视为违规
❌ Cross-scope changes（一个 task 只改一个范围）
❌ Symbol 后缀剥离（raw broker symbol = canonical）
```

---

## 十二、Command Output Discipline

施工 agent 是 Claude Code（Sonnet/Haiku）。以下规则减少 token 消耗：

| 操作 | ✅ 首选 | ❌ 禁止 |
|------|--------|--------|
| 读文件 | Read 工具 | `cat` / `head` / `tail` |
| 搜索文本 | Grep 工具 | `grep -rn` |
| 查找文件 | Glob 工具 | `find` |
| 统计 | — | `wc -l` |
| 列目录 | — | `ls -la` |

- 内置工具（Read/Grep/Glob）零 token 开销，结果格式化。
- 仅在需要复杂管道或非文件操作时使用 Bash。
- 裸 `grep`/`find`/`cat`/`wc`/`ls` 禁止在 Bash 中直接使用。

---

## 十三、Before Commit

施工 agent 每次提交前必须通过：

```bash
go build ./...                              # 编译必须通过
go test -race -count=1 ./...                # 全量 race 检测
go vet ./...                                # 静态分析
go run ./tools/check-file-lines --strict    # 文件规模检查（🔴 阻断）
govulncheck ./...                           # 已知漏洞
```
