# 跨平台跨经纪商套利系统 — 项目约束文档

> 版本：v5.0
> 日期：2026-08-07
> 施工 agent：Claude Code / Windsurf Cascade
> 违反任何一条约束的代码不得合并。
>
> **v4.0 重大变更**：desk 前端从 Fyne 迁移到 Wails v3 + Svelte。
> **v5.0 重大变更**：desk 客户端从 Wails v3 + Svelte/JS 迁移到 **.NET 8 WPF + C#**（多语言架构，D-005）。变更原因见 §三。

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
❌ gRPC-Web — 不需要，core↔desk 直接走原生 gRPC（grpc-dotnet），无 Web 栈
❌ Server-Sent Events
❌ GraphQL
❌ 任何文本格式的 RPC
❌ HTTP 服务器 — desk 不启动任何 HTTP listener
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

## 三、桌面应用（Desk）

### 3.1 架构

```
desk.exe (单个进程，.NET 8 WPF)
├── C# gRPC client (grpc-dotnet) ──── gRPC (network) ────→ core:50051
├── ViewModels (MVVM) — 业务逻辑（状态聚合、命令绑定）
└── WPF UI (XAML + 数据绑定)
    ├── INotifyPropertyChanged / ObservableCollection (实时刷新)
    ├── 5 个视图：价差矩阵 / 持仓 / 交易 / 历史 / 管理
    └── 图表控件（LiveCharts/OxyPlot/ScottPlot）
```

### 3.2 允许

```
✅ .NET 8 WPF + C# — Windows 桌面客户端（单进程，原生）
✅ grpc-dotnet (Grpc.Net.Client) — 连 core 的唯一网络通道
✅ WPF 数据绑定（INotifyPropertyChanged / ObservableCollection）
✅ 图表库：LiveChartsCore / OxyPlot / ScottPlot（任选其一）
✅ MVVM 模式（ViewModel + XAML View + ICommand）
✅ 5 个视图：价差矩阵 / 持仓 / 交易 / 历史 / 管理
✅ Proto 生成的 C# gRPC client（与 Go 共享同一份 .proto）
```

### 3.3 禁止

```
❌ 浏览器 / Electron / Web — 不捆绑 Chromium、不走 Web 栈
❌ Wails / Fyne / Avalonia — Go 桌面非原生（已废，见 D-005）
❌ React / Vue / JS / Svelte 前端 — desk 不再用 JS（已改 C#）
❌ TypeScript — desk 不走 Web 前端构建
❌ TUI / 终端界面
❌ desk 直连 broker — 所有 broker I/O 经 core（desk 只见 gRPC）
❌ desk 内启动 HTTP server / REST endpoint / WebSocket
❌ desk 直接访问 PostgreSQL — 历史等查询经 core gRPC unary
```

### 3.4 数据流

```
实时推送（gRPC server stream → WPF 数据绑定）：
  core gRPC stream (OpportunityStream / SpreadMatrix / PositionWatch)
    │ C# stream.Recv() async (await foreach)
    │ ViewModel 更新 ObservableCollection / 触发 PropertyChanged
    ▼
  WPF UI 自动刷新（数据绑定引擎）

用户操作（WPF 命令 → gRPC unary）：
  WPF Button → ICommand →
    client.ConfirmOpportunityAsync(req) / SubmitOrderAsync(req)  ← 网络 gRPC
    ▼
  core: DashboardService.{ConfirmOpportunity, SubmitOrder, ...}
```

### 3.5 为什么是 WPF 而非 Wails

D-005 第一性重审结论：**多语言架构（Go core + C# desk），各层最优**。
- **WPF 是 Windows 桌面标杆**：成熟的数据绑定、稳健的图表生态（LiveCharts/OxyPlot/ScottPlot）、长期稳定（与 .NET 同生命周期），实时数据刷新是 WPF 数据绑定的主场。
- **Wails v3 仍 beta**：本仓库已踩坑；Go 桌面框架（Wails/Fyne/Avalonia）非 Windows 原生，在数据绑定与图表生态上不及 WPF。
- **gRPC+protobuf 桥接成本极小**：Go core 与 C# desk 共享同一份 `.proto`，grpc-dotnet 性能不输 Go gRPC，多语言不会成为瓶颈。强行单语言会逼某层次优。
- 移动端第一版不做；后期如需，独立项目（与 desk 不共用 WPF）。

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

### 8.1 允许（core / Go）

| 包 | 用途 |
|----|------|
| `shopspring/decimal` | Warm/Cold Path 精度 |
| `google.golang.org/grpc` | 唯一通信 |
| `google.golang.org/protobuf` | 序列化 |
| `github.com/jackc/pgx/v5` | PostgreSQL driver |
| `golang.org/x/time/rate` | 自适应限流 |
| `log/slog` | 结构化日志 |

### 8.2 允许（desk / C# NuGet，D-005）

| 包 | 用途 |
|----|------|
| `Grpc.Net.Client` / `Grpc.Tools` / `Google.Protobuf` | grpc-dotnet 连 core |
| .NET 8 WPF SDK | Windows 桌面壳 |
| `LiveChartsCore` / `OxyPlot` / `ScottPlot` | 图表（择一） |

### 8.3 禁止

```
❌ encoding/json
❌ gorilla/websocket
❌ go-resty/resty
❌ gin/echo/gorilla/mux（任何 HTTP 框架）
❌ go-redis/redis
❌ mattn/go-sqlite3、modernc.org/sqlite
❌ fyne.io/fyne/v2（Go 桌面，已废）
❌ github.com/wailsapp/wails/v3（Go 桌面，已废，见 D-005）
```

---

## 九、代码组织

```
cmd/
  core/main.go        # 守护进程入口（Go）
internal/             # Go 包（core 侧）
  adapter/            # PlatformAdapter + MT4/MT5 实现
  bus/                # QuoteBus
  engine/             # 策略引擎（detector + evaluator）
  execute/            # 执行管线 + 幂等去重
  risk/               # 资金门禁 + 熔断 + Kill Switch
  audit/              # 审计日志
  decimalutil/        # float64↔decimal 统一转换
  errclass/           # 错误码分类
  dashboard/          # DashboardService gRPC server
  store/              # PostgreSQL 读写
desk/                 # C# .NET 8 WPF 项目（取代旧 Wails，D-005）
  Desk.csproj         # 项目文件（.NET 8，NET 8 WPF SDK）
  App.xaml / App.xaml.cs
  MainWindow.xaml / MainWindow.xaml.cs
  ViewModels/         # MVVM（Matrix / Positions / Trading / History / Admin）
  Views/              # XAML 视图（5 个）
  Services/           # gRPC client（封装 grpc-dotnet 连 core）
  Proto/              # 由 .proto 生成的 C# gRPC client 桩
# frontend/（旧 Svelte）作废，删除
proto/
  config/             # 配置 schema（Go/C# 共享）
  dashboard/          # DashboardService schema（Go/C# 共享）
migrations/           # PG DDL
```

命名：
```
✅ Go 接口：名词（PlatformAdapter, QuoteBus）
✅ Go 方法：动词（Connect, Publish, Evaluate）
✅ Go 文件：小写下划线（quote_bus.go）
✅ C# 类型 / XAML：PascalCase（MatrixViewModel, MatrixView.xaml）
✅ C# 私有字段：_camelCase
❌ 包名包含 util/common/misc/helper
❌ Go 接口名 I 前缀（C# 接口保留 I 前缀，遵循 .NET 惯例）
```

---

## 十、编译与部署

```
✅ core：Go 1.22+，Linux amd64，CGO_ENABLED=0，Docker 容器化
✅ desk：.NET 8 SDK + WPF（Windows amd64，dotnet build → desk.exe）
✅ desk 不再依赖 Node.js / npm（旧 Svelte 链路随 D-005 废除）
✅ Go 与 C# 共享同一份 proto（buf generate 出 Go stub，Grpc.Tools 出 C# stub）
✅ TLS 1.3
✅ GOTRACEBACK=0
✅ ulimit -c 0
✅ GOGC=50
✅ go test -race 强制（core）
✅ CI 跑 govulncheck（core）
```

---

## 十一、语言 + 零容忍禁止

### 11.1 语言

```
✅ core 后端：Go（core 唯一语言）
✅ desk 客户端：C#（.NET 8 WPF；多语言架构，见 D-005）
❌ 禁止 TypeScript（desk 不走 Web 前端）
❌ 禁止 Python / 任何其他语言
❌ desk 不再用 JS/Svelte（已改 C#）
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
