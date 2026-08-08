# 15 · Desk WPF 实施规格

> C# .NET 8 WPF 桌面应用的**实施级**规格。UX/布局/数据流已在 `10-desk-ui.md`。
> 本文件给 Windsurf 的项目结构、包依赖、MVVM 模式、grpc-dotnet 线程模型、文件清单。
> core 的 gRPC OpportunityStream + ConfirmOpportunity 已在 Phase D 就位，desk 直接消费。

---

## 0. 项目定位

desk = 独立的 .NET 8 WPF 项目（`desk/` 目录），通过 grpc-dotnet 连 core:50051。不直连 PG、不调 broker——所有数据经 core gRPC。

最小可用版（v0）：Opportunity 列表 + 确认按钮。其余视图逐步加。

---

## 1. 项目骨架

### NuGet 包（.csproj）
```xml
<PackageReference Include="Grpc.Net.Client" />
<PackageReference Include="Grpc.Tools" PrivateAssets="All" />
<PackageReference Include="Google.Protobuf" />
<PackageReference Include="CommunityToolkit.Mvvm" />  <!-- MVVM source generators -->
```

### Proto
从 `proto/dashboard/dashboard.proto` 生成 C# stub。`Grpc.Tools` 自动处理（`<Protobuf Include="..\proto\dashboard\dashboard.proto" GrpcServices="Client" />`）。

### 目录结构
```
desk/
  ArbDesk.csproj
  App.xaml / App.xaml.cs
  MainWindow.xaml / MainWindow.xaml.cs
  Proto/                              # Grpc.Tools 自动生成
  Services/
    DashboardClient.cs                # GrpcChannel + gRPC client 封装
  ViewModels/
    BaseViewModel.cs                  # INotifyPropertyChanged 基类（或 CommunityToolkit ObservableObject）
    OpportunityViewModel.cs           # 机会列表 + Confirm ICommand
    MatrixViewModel.cs
    PositionsViewModel.cs
    TradingViewModel.cs
    HistoryViewModel.cs
    AdminViewModel.cs
  Views/
    OpportunityView.xaml
    MatrixView.xaml
    PositionsView.xaml
    TradingView.xaml
    HistoryView.xaml
    AdminView.xaml
```

---

## 2. gRPC 客户端（`Services/DashboardClient.cs`）

```csharp
public class DashboardClient : IDisposable
{
    readonly GrpcChannel _channel;
    readonly DashboardService.DashboardServiceClient _client;

    public DashboardClient(string address = "http://localhost:50051")
    {
        _channel = GrpcChannel.ForAddress(address);
        _client = new DashboardService.DashboardServiceClient(_channel);
    }

    // Stream: core → desk
    public IAsyncEnumerable<OpportunityEvent> OpportunityStream() =>
        _client.OpportunityStream(new OpportunityStreamRequest()).ResponseStream.ReadAllAsync();

    public IAsyncEnumerable<SpreadMatrixReply> SpreadMatrix() =>
        _client.SpreadMatrix(new SpreadMatrixRequest()).ResponseStream.ReadAllAsync();

    public IAsyncEnumerable<PositionWatchReply> PositionWatch() =>
        _client.PositionWatch(new PositionWatchRequest()).ResponseStream.ReadAllAsync();

    // Unary: desk → core
    public async Task<ConfirmReply> ConfirmOpportunityAsync(string id) =>
        await _client.ConfirmOpportunityAsync(new ConfirmRequest { OpportunityId = id });

    // ... SubmitOrder, ClosePosition, GetSignalHistory, Kill, ToggleStrategy 等
}
```

- `GrpcChannel.ForAddress` 默认 HTTP/2（core gRPC server 已支持）
- 生命周期：App.xaml.cs 创建，DI 或 static 单例传入 ViewModel
- 线程：`ResponseStream.ReadAllAsync()` 返回 `IAsyncEnumerable`，在 `await foreach` 消费

---

## 3. MVVM 模式

### OpportunityViewModel（核心，先做）

```csharp
public partial class OpportunityViewModel : ObservableObject
{
    readonly DashboardClient _client;

    public ObservableCollection<Opportunity> Opportunities { get; } = new();

    [RelayCommand]
    async Task Confirm(Opportunity opp)
    {
        var reply = await _client.ConfirmOpportunityAsync(opp.Id);
        // reply.Accepted → 更新 UI 状态
    }

    // 启动时订阅流
    async Task StartStream(CancellationToken ct)
    {
        await foreach (var ev in _client.OpportunityStream().WithCancellation(ct))
        {
            App.Current.Dispatcher.Invoke(() =>
            {
                switch (ev.Action)
                {
                    case OpportunityAction.Pushed:
                    case OpportunityAction.Updated:
                        Opportunities.Insert(0, ev.Opp); // 新机会置顶
                        break;
                    case OpportunityAction.Expired:
                    case OpportunityAction.Failed:
                        Remove(ev.Id);
                        break;
                    case OpportunityAction.Filled:
                        UpdateStatus(ev.Id, "Filled");
                        break;
                }
            });
        }
    }
}
```

- `CommunityToolkit.Mvvm` 的 `[ObservableProperty]` / `[RelayCommand]` source generator —— 不用手写 INotifyPropertyChanged boilerplate
- gRPC 流消费在后台 Task；UI 更新通过 `App.Current.Dispatcher.Invoke`
- decimal 字段：`decimal.Parse(opp.NetProfit)` → 格式化展示

### v1：MatrixViewModel + MatrixView

```csharp
public partial class MatrixViewModel : ObservableObject
{
    readonly DashboardClient _client;
    CancellationTokenSource _cts = new();

    public ObservableCollection<BrokerCell> Rows { get; } = new();

    public async Task StartStream(CancellationToken ct)
    {
        await foreach (var m in _client.SpreadMatrix(ct))
        {
            Application.Current.Dispatcher.Invoke(() =>
            {
                Rows.Clear();
                foreach (var row in m.Rows)
                    Rows.Add(new BrokerCell(row));
            });
        }
    }
}

public class BrokerCell
{
    public string Broker { get; }
    public string Symbol { get; }
    public double Bid { get; }
    public double Ask { get; }
    public double Spread { get; }
    public bool Arbitrageable { get; }  // from proto spread_cell
    public BrokerCell(SpreadMatrixReply.Types.BrokerRow row) { /* map fields */ }
}
```

MatrixView.xaml：`DataGrid` 绑 `Rows`，列 = Broker / Symbol / Bid / Ask / Spread，`Arbitrageable`=true 行高亮。

### v1：PositionsViewModel + PositionsView

```csharp
public partial class PositionsViewModel : ObservableObject
{
    readonly DashboardClient _client;

    public ObservableCollection<PositionRow> Positions { get; } = new();

    public async Task StartStream(CancellationToken ct)
    {
        await foreach (var p in _client.PositionWatch(ct))
        {
            Application.Current.Dispatcher.Invoke(() =>
            {
                Positions.Clear();
                foreach (var b in p.Brokers)
                foreach (var pos in b.Positions)
                    Positions.Add(new PositionRow(b.Broker, pos));
            });
        }
    }
}

public class PositionRow
{
    public string Broker { get; }
    public string Symbol { get; }
    public string Direction { get; }
    public double Volume { get; }
    public double OpenPrice { get; }
    public double CurrentPrice { get; }
    public double PnL { get; }
    public double Swap { get; }
    public PositionRow(string broker, Position pos) { /* map fields */ }
}
```

PositionsView.xaml：`DataGrid` 绑 `Positions`，列 = Broker / Symbol / Direction / Volume / OpenPrice / CurrentPrice / PnL / Swap。

### v2：Trading + History + Admin

**TradingViewModel**：
```csharp
public partial class TradingViewModel : ObservableObject
{
    [ObservableProperty] string _broker, _symbol, _lots;
    [ObservableProperty] BuySell _direction;

    [RelayCommand]
    async Task SubmitOrder() {
        var reply = await _client.SubmitOrderAsync(new ManualOrderRequest {
            Broker = Broker, Symbol = Symbol, Lots = Lots, Direction = Direction, Slippage = 5
        });
        LastResult = reply.Accepted ? "Order placed" : $"Rejected: {reply.Error}";
    }

    [RelayCommand]
    async Task ClosePosition(PositionRow row) {
        await _client.ClosePositionAsync(new ClosePositionRequest {
            Broker = row.Broker, Ticket = row.Ticket, Volume = row.Volume
        });
    }
}
```
TradingView.xaml：Broker/Symbol/Lots/Direction 输入框 + Submit/Close 按钮 + 结果文本。

**HistoryViewModel**：
```csharp
public partial class HistoryViewModel : ObservableObject
{
    public ObservableCollection<SignalRow> Signals { get; } = new();

    [RelayCommand]
    async Task Load() {
        var reply = await _client.GetSignalHistoryAsync(new SignalHistoryRequest());
        Application.Current.Dispatcher.Invoke(() => {
            Signals.Clear();
            foreach (var s in reply.Signals) Signals.Add(new SignalRow(s));
        });
    }
}
```
HistoryView.xaml：`DataGrid` 绑 `Signals`，列 = Time / Strategy / Legs / PnL / Status。`LoadCommand` 按钮触发。

**AdminViewModel**：
```csharp
public partial class AdminViewModel : ObservableObject
{
    [ObservableProperty] bool _isKilled;
    [ObservableProperty] string _strategyStatus;

    [RelayCommand] async Task Kill() { await _client.KillAsync(new KillRequest()); IsKilled = true; }
    [RelayCommand] async Task Resume() { await _client.ResumeAsync(new ResumeRequest()); IsKilled = false; }
    [RelayCommand] async Task ToggleStrategy(string name) { await _client.ToggleStrategyAsync(...); }
}
```
AdminView.xaml：Kill 按钮（红色）+ Resume + 策略开关列表。

---

## 4. View 绑定

### OpportunityView.xaml（Master-Detail 表格，10 §4）

```xml
<DataGrid ItemsSource="{Binding Opportunities}" AutoGenerateColumns="False"
          SelectedItem="{Binding SelectedOpportunity}">
    <DataGrid.Columns>
        <DataGridTextColumn Header="Type" Binding="{Binding Type}" />
        <DataGridTextColumn Header="Legs" Binding="{Binding LegsSummary}" />
        <DataGridTextColumn Header="NetBps" Binding="{Binding NetBps}" />
        <DataGridTextColumn Header="NetProfit" Binding="{Binding NetProfit}" />
        <DataGridTextColumn Header="Expires" Binding="{Binding ExpiresAtUnixMs, Converter=...}" />
        <DataGridTemplateColumn Header="">
            <Button Command="{Binding DataContext.ConfirmCommand, RelativeSource=...}"
                    CommandParameter="{Binding}" Content="确认" />
        </DataGridTemplateColumn>
    </DataGrid.Columns>
</DataGrid>
<!-- 详情面板：选中行展开，显示全成本拆解 + 腿详情 -->
```

- 风险提示列（10 §4.1）：`Executable`=false → 行灰显 + tooltip 显示 `RejectReason`
- 筛选排序栏（10 §4.4）：`CollectionViewSource` 绑 `Type`/`NetBps`/`ExpiresAt`
- decimal→display：XAML converter 或 ViewModel 的 string 属性

---

## 5. 线程模型

```
UI Thread (STA)
  │ Dispatcher.Invoke ─── 更新 ObservableCollection / PropertyChanged
  │
Background Task
  │ await foreach (gRPC stream)
  │   └── Dispatcher.Invoke(() => { ... })
  │
Confirm 按钮
  │ ICommand → async Confirm()
  │   └── await client.ConfirmOpportunityAsync(id)
```

- gRPC `await foreach` 在后台 Task 运行（不阻塞 UI）
- 所有 UI 更新通过 `Dispatcher.Invoke`
- `DataGrid` + `ObservableCollection` 天然支持实时更新（新增/删除行自动反映）

---

## 6. 实施顺序

| 阶段 | 内容 | 可验收标准 |
|---|---|---|
| v0 | .csproj + DashboardClient + OpportunityViewModel + OpportunityView | 启动 → 连 core → 看到机会列表 → 点确认按钮 → core 返回 accepted |
| v1 | MatrixViewModel/View + PositionsViewModel/View | 看到价差矩阵实时刷新 + 持仓列表 |
| v2 | Trading/History/Admin ViewModel/View | 手动下单/平仓 + 查历史 + Kill Switch |

每个阶段独立 PR、独立可跑。

---

## 7. 非本阶段

- desk 不直连 PG、不调 broker（数据全从 core gRPC，constraints §三）
- 图表（`10 §3`）用 LiveChartsCore/OxyPlot —— v1 阶段再选型集成
- UI 自动化测试（`11-testing.md:157`）—— 后期可加

## 8. 回溯

- UX/布局 → `10-desk-ui.md`
- gRPC 接口 → `06-interfaces.md §5.2`
- proto 字段 → `proto/dashboard/dashboard.proto`（Phase D 已同步）
- core OpportunityStream/ConfirmOpportunity → Phase D `internal/dashboard/opportunity.go`
- 约束：desk C# WPF 是唯一前端（D-005），gRPC 是唯一通信协议（constraints §三）
