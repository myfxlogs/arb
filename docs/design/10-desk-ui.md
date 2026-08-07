# 10 · desk UI（arb-cockpit WPF 详细设计）

> desk 桌面客户端的实现级设计。本文是 `04 §4`（人机交互）+ `06`（gRPC 接口）+ D-005（WPF）的填充，落地到 MVVM 分层、视图、ViewModel、数据绑定、线程模型。
> 依据：D-005（多语言架构，desk = .NET 8 WPF + C#）、`04`（机会闭环 + 确认按钮）、`06`（OpportunityStream / ConfirmOpportunity）、constraints §三（WPF 唯一允许）。
> 实现者（Windsurf）照本文落地；命名对齐 constraints §九（C# PascalCase，私有字段 `_camelCase`）。

---

## 1. 定位与栈

**arb-cockpit** 是运营人员的「持续监控面板 + 决策工作台」（CLAUDE.md：持仓 Tab 不是看一眼就关）。它不是薄壳，而是 `core` 全部能力的可视化与确认入口。

| 维度 | 选型 | 依据 |
|---|---|---|
| 语言/框架 | C# / .NET 8 WPF | D-005 |
| 网络通道 | grpc-dotnet（`Grpc.Net.Client`）连 `core:50051` | constraints §一/§三 |
| UI 模式 | MVVM（XAML + 数据绑定 + `ICommand`） | constraints §三 3.2 |
| 图表 | LiveChartsCore / OxyPlot / ScottPlot（择一，推荐 LiveChartsCore） | constraints §三 3.2 |
| 进程 | 单进程，**不启动任何 HTTP/WebSocket listener** | constraints §三 3.3 |

**唯一网络出口是 gRPC**：desk 不直连 broker、不直读 PostgreSQL，所有数据经 `core` 的 `DashboardService`（06）。

---

## 2. MVVM 分层

```
desk/ (arb-cockpit)
├── App.xaml / App.xaml.cs          启动 + 全局资源 + 路由
├── MainWindow.xaml(.cs)            顶层 Tab 容器 + 全局状态栏（Kill Switch/连接状态）
│
├── ViewModels/                     ★业务逻辑（状态聚合、命令绑定、gRPC 调用编排）
│   ├── MainViewModel.cs            顶层：持有各 Tab ViewModel + 全局命令（Kill/Resume）
│   ├── OpportunityViewModel.cs     ★机会卡片（单条）— 见 §4
│   ├── OpportunityListViewModel.cs ★机会列表（主视图）— 见 §4
│   ├── MatrixViewModel.cs          价差矩阵
│   ├── PositionsViewModel.cs       持仓 + 浮动 PnL（长期监控面板，CLAUDE.md）
│   ├── TradingViewModel.cs         手动交易（SubmitOrder / ClosePosition / CancelOrder）
│   ├── HistoryViewModel.cs         历史（Signal / Order / DailySummary）
│   └── AdminViewModel.cs           管理（Broker 增删 / 策略开关 / 日志尾巴）
│
├── Views/                          XAML 视图（纯展示，无业务逻辑）
│   ├── OpportunityListView.xaml    ★主视图（默认 Tab）
│   ├── MatrixView.xaml
│   ├── PositionsView.xaml
│   ├── TradingView.xaml
│   ├── HistoryView.xaml
│   └── AdminView.xaml
│
├── Services/                       gRPC client 封装（§5）+ 调度
│   ├── GrpcClientFactory.cs        Channel 单例 + client stub 工厂
│   ├── OpportunityService.cs       OpportunityStream 订阅 + ConfirmOpportunity
│   ├── MarketService.cs            SpreadMatrix / PositionWatch 订阅
│   ├── TradingService.cs           SubmitOrder / ClosePosition / CancelOrder unary
│   ├── HistoryService.cs           历史 unary 查询
│   ├── AdminService.cs             Broker 管理 / ToggleStrategy / Kill / Resume
│   └── UIDispatcher.cs             后台 Task → UI 线程推送（§7）
│
├── Proto/                          由 dashboard.proto 经 Grpc.Tools 生成的 C# stub
└── Models/                         轻量 UI 模型（枚举/值类型转换 helper）
```

**分层规则**（constraints §三 3.2 MVVM 模式）：
- **View**（XAML）：只绑定 + 触发命令，不含业务逻辑、不直接调 gRPC。
- **ViewModel**：持有 UI 状态（`ObservableCollection` / `INotifyPropertyChanged`），编排 `Services` 的 gRPC 调用，把结果投到 UI 线程。
- **Services**：封装 grpc-dotnet client，**只懂 gRPC**，不懂 UI；返回 proto message 或映射后的 UI model。
- **Proto**：与 Go `core` 共享同一份 `dashboard.proto`（Grpc.Tools 生成 C# 桩）。

---

## 3. 视图（6 个 Tab）

constraints §三 3.2 列的 5 视图（价差矩阵 / 持仓 / 交易 / 历史 / 管理）+ **机会列表（新增主视图）**（04 §4「机会列表（新增视图）」）= 6 个 Tab。机会列表是「**主视图 + 默认 Tab**」，因为它是 D-003「人确认」的核心落点。

| Tab | ViewModel | 数据源 RPC | 刷新 | 用途 |
|---|---|---|---|---|
| **机会列表** ★主 | OpportunityListViewModel | `OpportunityStream`（push）+ `ConfirmOpportunity`（unary） | 实时 push | 看机会 → 你确认 → 看状态变更（Filled/Failed/Expired） |
| 价差矩阵 | MatrixViewModel | `SpreadMatrix`（server stream） | push（refresh_interval_ms） | 全市场 bid/ask + 跨所价差热力 |
| 持仓 | PositionsViewModel | `PositionWatch`（server stream） | push | 长期持仓 + 浮动 PnL + swap 累积（持续监控） |
| 交易 | TradingViewModel | `SubmitOrder`/`ClosePosition`/`CancelOrder`（unary） | 命令触发 | 手动通道（调试/救急，与机会流程并存，04 §4） |
| 历史 | HistoryViewModel | `GetSignalHistory`/`GetOrderHistory`/`GetDailySummary`/`GetAccountSnapshots`（unary） | 查询触发 | 合规追溯 + 归因复盘 |
| 管理 | AdminViewModel | `SearchBroker`/`AddBroker`/`RemoveBroker`/`ToggleStrategy`/`Kill`/`Resume`/`TailLogs`（unary） | 命令触发 | broker 增删、策略开关、Kill Switch、日志 |

**全局状态栏**（`MainWindow` 顶部，常驻）：
- Kill Switch 按钮（`ICommand` → `KillAsync`），红色确认弹窗。
- core 连接状态（gRPC channel `state`，`Connected`/`Connecting`/`TransientFailure`）。
- 全局 blind 指示（09 §10：mtapi 断 → core 拒新仓；desk 经 `PositionWatch`/`OpportunityStream` 事件感知）。
- 日亏损熔断指示（07 §1：日亏 ≤ 3% 暂停）。

---

## 4. OpportunityViewModel（机会卡片 + 确认命令）

机会列表的主元素是 `OpportunityViewModel`（一条机会 = 一张卡片）。字段对齐 `02 §5` Opportunity + `06` proto。

```csharp
public sealed class OpportunityViewModel : INotifyPropertyChanged
{
    // —— 身份 / 类型 ——
    public string Id { get; }                    // Opportunity.id
    public OppType Type { get; }                 // CROSS_EXCHANGE / CARRY / TRIANGULAR
    public IReadOnlyList<LegViewModel> Legs { get; }  // 每腿展示

    // —— 净盈利（统一度量，02 §3）——
    public decimal NetProfitUsd { get; }         // proto net_profit → decimal.Parse(string)
    public decimal NetBps { get; }               // proto net_bps
    public string NetSummary => $"{NetBps:F1} bp · ${NetProfitUsd:F2}";

    // —— 成本拆解（02 §4，公理③）——
    public decimal GrossProfit { get; }
    public decimal SpreadCost { get; }
    public decimal CommissionCost { get; }
    public decimal SlippageCost { get; }
    public decimal SwapCost { get; }

    // —— 时间（公理④）——
    public DateTimeOffset QuoteTime { get; }
    public DateTimeOffset ExpiresAt { get; }
    public TimeSpan Remaining => ExpiresAt - DateTimeOffset.Now;   // 倒计时
    public double Confidence { get; }            // 0..1

    // —— 状态机（04 §2）——
    public OppStatus Status { get; private set; } // Pushed/Confirmed/Executing/Filled/Failed/Expired
    public bool CanConfirm => Status == OppStatus.Pushed && Remaining > TimeSpan.Zero;

    // —— 命令 ——
    public ICommand ConfirmCommand { get; }      // → ConfirmOpportunityAsync（§5）
    public ICommand DismissCommand { get; }      // 本地忽略（不调 core；机会自然 Expire）

    public event PropertyChangedEventHandler? PropertyChanged;
    // OnStatusChanged / 倒计时 Tick → PropertyChanged
}

public sealed class LegViewModel
{
    public string Broker { get; }                // "ICMarketsSC-Demo"
    public string BrokerSymbol { get; }          // 原始符号（下单用）
    public string CanonicalSymbol { get; }       // 逻辑符号（展示）
    public BuySell Direction { get; }            // Buy/Sell
    public decimal Lots { get; }
    public decimal EstimatePrice { get; }
    public string Summary => $"{Broker} · {BrokerSymbol} {Direction} {Lots} @ {EstimatePrice}";
}
```

**机会列表 ViewModel**（主视图）：

```csharp
public sealed class OpportunityListViewModel : INotifyPropertyChanged
{
    public ObservableCollection<OpportunityViewModel> Opportunities { get; } = new();
    public OpportunityViewModel? Selected { get; set; }   // 详情面板绑定
    // 过滤：只看 Pushed / 含全部状态；按 NetBps 排序
}
```

**卡片展示要素**（04 §4 + 02 §5）：
- 顶栏：类型徽标（CrossExchange/Carry/Triangular）+ 状态徽标 + 倒计时进度条。
- 中段：腿列表（`LegViewModel.Summary` 逐腿）。
- 净盈利大字：`NetSummary`（`3.2 bp · $42.50`）+ 置信度小字。
- 成本拆解（可展开）：点差/手续费/滑点/swap 四行。
- 操作区：「确认执行」按钮（`CanConfirm` 为 `true` 时可用）+ 「忽略」按钮。

**确认按钮**（04 §4 的核心）：
```csharp
ConfirmCommand = new AsyncRelayCommand(ConfirmAsync, () => CanConfirm);

private async Task ConfirmAsync()
{
    var reply = await _opportunityService.ConfirmAsync(Id);
    if (!reply.Accepted)
        await _dialog.Warn($"未接受：{reply.Reason}");   // 如已被 Expire/价格漂移
}
```
- `ICommand` 绑定到 XAML `<Button Command="{Binding ConfirmCommand}">`。
- 确认后按钮置灰（`CanConfirm` 变 `false`），等 `OpportunityStream` 推状态变更（→ Confirmed/Executing/Filled/Failed）。

---

## 5. gRPC client（grpc-dotnet）

`Services/` 封装 grpc-dotnet client。**所有 RPC 形态对齐 `06` + 现有 `dashboard.proto`**。

### 5.1 Channel + client 工厂
```csharp
// GrpcClientFactory.cs
public sealed class GrpcClientFactory
{
    private readonly Channel _channel;
    public GrpcClientFactory(string coreAddress)   // "static (127.0.0.1:50051)" 或 core host
    {
        _channel = Channel.ForAddress(coreAddress);
        Dashboard = _channel.CreateGrpcService<DashboardService.DashboardServiceClient>();
    }
    public DashboardService.DashboardServiceClient Dashboard { get; }
}
```
- 用 `Grpc.Core` 自动生成的 client stub（Grpc.Tools 从 `dashboard.proto` 产出）。
- TLS 1.3（constraints §十）；本地同机可走明文 TCP，跨机走 TLS。

### 5.2 OpportunityService（push stream + confirm unary）
```csharp
// OpportunityService.cs
public sealed class OpportunityService
{
    private readonly DashboardService.DashboardServiceClient _client;
    private readonly UIDispatcher _dispatcher;
    public event Action<OpportunityEvent>? OnEvent;   // UI 订阅

    public async Task SubscribeAsync(CancellationToken ct)
    {
        using var call = _client.OpportunityStream(new Empty());
        await foreach (var evt in call.ResponseStream.ReadAllAsync(ct))
            _dispatcher.Post(() => OnEvent?.Invoke(evt));   // → UI 线程（§7）
    }

    public Task<ConfirmReply> ConfirmAsync(string opportunityId) =>
        _client.ConfirmOpportunityAsync(
            new ConfirmRequest { OpportunityId = opportunityId }).ResponseAsync;
}
```

### 5.3 MarketService（SpreadMatrix / PositionWatch）
同 §5.2 模式：`await foreach` 消费 server stream，每条消息 `Dispatcher.Post` 到 UI 线程刷新 `ObservableCollection`。

### 5.4 TradingService / HistoryService / AdminService
unary 调用，`await client.SubmitOrderAsync(req)` 直接返回；UI 用 `AsyncRelayCommand` 绑定。

---

## 6. 数据绑定（实时刷新）

WPF 数据绑定引擎是 desk 实时刷新的主场（D-005）：

| 机制 | 用途 |
|---|---|
| `INotifyPropertyChanged` | 单对象属性变更（OpportunityViewModel 的 Status/Remaining/CanConfirm） |
| `ObservableCollection<T>` | 列表增删（OpportunityListViewModel.Opportunities；PositionsViewModel.Positions） |
| `OneWay` 绑定 | push 数据 → UI（stream 推送，UI 只读） |
| `TwoWay` 绑定 | 表单（手动下单 lots/symbol 输入） |
| `ICommand` / `AsyncRelayCommand` | 按钮触发 gRPC unary（Confirm/Submit/Kill/Toggle） |
| `CollectionViewSource` | 排序/过滤（机会按 NetBps 排序、按 Status 过滤） |

**实时刷新链**（constraints §三 3.4）：
```
core gRPC stream ──► await foreach (ResponseStream.ReadAllAsync)
                  ──► UIDispatcher.Post(...)              [切到 UI 线程]
                  ──► ObservableCollection.Add/Remove 或 PropertyChanged
                  ──► WPF binding engine 自动刷新 View
```

**倒计时**（公理④，机会卡片需要秒级倒计时）：
- 单个 `DispatcherTimer`（1Hz）驱动所有可见卡片的 `Remaining` 触发 `PropertyChanged`，不为每张卡片开 timer。

---

## 7. 线程模型（后台 Task → UI 线程）

**铁律**：gRPC stream 的 `await foreach` 在后台 `Task` 跑，**绝不**在 UI 线程。所有 UI 状态变更必须经 `Dispatcher` 切到 UI 线程（WPF `DependencyObject` 只能在创建它的线程访问）。

```csharp
// UIDispatcher.cs
public sealed class UIDispatcher
{
    private readonly Dispatcher _ui = Dispatcher.CurrentDispatcher;
    public void Post(Action a) => _ui.BeginInvoke(a);   // 后台 → UI
    public T Invoke<T>(Func<T> f) => _ui.Invoke(f);
}
```

每个 stream 订阅 = 一个后台 `Task`（C# `Task.Run` + `CancellationToken`）：
```
OpportunityListViewModel.OnLoaded  → Task.Run(() => _opportunityService.SubscribeAsync(ct))
                                  → stream 在后台 await foreach
                                  → 每条事件 Post 到 UI 线程 → ObservableCollection.Add
MainWindow.OnClosed               → ct.Cancel() → 各 stream Task 退出
```

**断线重连**：gRPC channel `StateChanged` 事件 → 重连 stream Task；断线期间全局状态栏显示 `Disconnected`，UI 保留旧数据（不清空），重连后 stream 推新状态覆盖。core 侧断线行为见 09 §10（desk 断 → core 独立继续；desk 重连重订）。

**避免常见陷阱**：
- ❌ 后台 stream 直接 `ObservableCollection.Add` → `InvalidOperationException`（跨线程）。
- ❌ 在 UI 线程跑 `ResponseStream.ReadAllAsync` 阻塞 → UI 冻结。
- ✅ 大量事件批量 Post（每 100ms 合并一批）防 UI 抖动（高频 push 时）。

---

## 8. 图表（价差矩阵热力 / 曲线）

constraints §三 3.2 允许 LiveChartsCore / OxyPlot / ScottPlot。**推荐 LiveChartsCore**（WPF 数据绑定友好、实时序列示例成熟）。

| 图表 | 数据 | 类型 | 视图 |
|---|---|---|---|
| 价差矩阵热力 | `SpreadMatrixReply` 各 broker×品种 `spread_to_best_bid/ask_bps` | HeatMap | MatrixView |
| 单品种跨所价差曲线 | 历史 ticks（core 查询 or 本地短窗缓存） | 实时 LineSeries | MatrixView 详情 |
| 持仓浮动 PnL 曲线 | `PositionWatchReply` 时序 | LineSeries | PositionsView |
| 账户净值曲线 | `GetDailySummary` + `GetAccountSnapshots` | LineSeries | HistoryView |
| 归因偏差分布 | 历史 `opportunities.deviation_*` | Histogram | HistoryView（运维复盘） |

图表数据源同样是 ViewModel 的 `ObservableCollection<Point>`，binding 到 LiveCharts `ISeries`，push 到达 → `Post` 到 UI → `Add` → 曲线自动延伸。

---

## 9. 错误处理与可用性

| 场景 | 处理 |
|---|---|
| gRPC unary 返回 error | `AsyncRelayCommand` 捕获 → 弹窗（不崩 UI） |
| stream 断开 | channel `StateChanged` → 状态栏标红 + 自动重连（指数退避，core 侧 09 §10） |
| 确认时机会已 Expire | `ConfirmReply.Accepted=false` → 弹窗提示 reason，卡片状态由 stream 推回 Expired |
| Kill Switch 触发 | 全局状态栏红色横幅 + 所有 Confirm 按钮禁用 + 策略状态置灰 |
| core 重启对账中（09 §10） | OpportunityStream 推 `blind` 指示 → desk 暂停新机会展示 + 横幅提示 |

---

## 10. 实现指引（Windsurf）

| 组件 | 动作 |
|---|---|
| 项目脚手架 | `dotnet new wpf -n arb-cockpit -f net8.0`；加 `Grpc.Net.Client`/`Grpc.Tools`/`Google.Protobuf`/`LiveChartsCore.SkiaSharpView.WPF` NuGet。 |
| Proto stub | `dashboard.proto` 进 `Proto/`，`<Protobuf Include="Proto/dashboard.proto" />`（Grpc.Tools 生成 C# stub，与 Go 共享同一份 proto）。 |
| MVVM 基类 | `ViewModelBase`（`INotifyPropertyChanged`）+ `AsyncRelayCommand`（标准实现，无第三方 MVVM 框架强制，轻量）。 |
| 机会列表 | `OpportunityListView.xaml`（`ItemsControl` + 卡片 `DataTemplate`）+ `OpportunityListViewModel`（订阅 `OpportunityStream`）。 |
| 全局状态 | `MainWindow` 顶部 `ContentControl` 绑定 `MainViewModel.GlobalStatus`。 |
| 配置 | `core` 地址走配置文件（appsettings.json 或命令行）；不启动 HTTP listener。 |

---

## 11. 回溯

- WPF / C# / grpc-dotnet / MVVM / 6 视图 → D-005、constraints §三
- 机会列表主视图 + 确认按钮（ICommand → ConfirmOpportunity）→ 04 §4、§5
- OpportunityViewModel 字段（NetBps/USD/成本拆解/倒计时/置信度）→ 02 §5
- OpportunityStream（await foreach）+ ConfirmOpportunity（unary）→ 06
- 实时数据绑定（ObservableCollection / INotifyPropertyChanged）→ constraints §三 3.2/3.4
- 后台 Task → Dispatcher → UI 线程 → WPF 单线程 UI 模型（第一性）
- 持仓 Tab 长期监控 → CLAUDE.md（套息天~周，非看一眼就关）
