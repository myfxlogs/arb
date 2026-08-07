# 06 · gRPC 接口

> `DashboardService` 的完整 RPC 设计：现有保留 + 机会闭环新增。
> 依据 `04`（OpportunityStream/ConfirmOpportunity）+ constraints（gRPC 唯一网络协议、Push-First）+ 现有 `dashboard` proto。

---

## 1. 设计原则
- **gRPC + Protobuf 是 core↔desk 唯一网络协议**（constraints §一）。desk（C# WPF）通过 grpc-dotnet 直连 core。
- **Push-First**：实时数据用 server stream；用户动作用 unary。
- desk 不直接访问 broker / PostgreSQL，所有数据经 core gRPC。

---

## 2. DashboardService RPC 清单

### 现有（保留）
| RPC | 类型 | 用途 |
|---|---|---|
| `SpreadMatrix` | server stream | 价差矩阵实时推送 |
| `PositionWatch` | server stream | 持仓 + 浮动 PnL 推送 |
| `SubmitOrder` / `ClosePosition` / `CancelOrder` | unary | 手动交易通道（04 §4，与机会流程并存） |
| `GetSignalHistory` / `GetOrderHistory` / `GetDailySummary` | unary | 历史查询 |
| `GetAccountSnapshots` | unary | 各 broker 账户状态 |
| `BrokerSearch` / broker 管理 | unary | 添加/管理 broker（desk 管理 Tab） |
| `Kill` / `ToggleStrategy` / `Resume` | unary | 控制（Kill Switch / 策略开关 / 恢复） |

### 新增（机会闭环，04）
| RPC | 类型 | 用途 |
|---|---|---|
| `OpportunityStream` | server stream | core → desk 推送机会 + 状态变更 |
| `ConfirmOpportunity` | unary | desk → core，你确认执行 |

---

## 3. 新增 message 定义（proto 草案）

> 本节是初稿速览；**字段级完整定义见 §5「完整 proto 定义（落地版）」**，冲突以 §5 为准。

```protobuf
message Opportunity {            // 见 02 §5
  string id = 1;
  OppType type = 2;              // CROSS_EXCHANGE / CARRY / TRIANGULAR
  repeated Leg legs = 3;
  int64 quote_time_unix_ms = 4;
  // 成本拆解（decimal 存字符串，warm path）
  string gross_profit = 5;
  string spread_cost = 6;
  string commission_cost = 7;
  string slippage_cost = 8;
  string swap_cost = 9;
  string net_profit = 10;
  string net_bps = 11;
  int64 expires_at_unix_ms = 12;
  bool executable = 13;
  double confidence = 14;
  OppStatus status = 15;
}

message Leg {
  string broker = 1;
  string broker_symbol = 2;      // 原始符号，下单用
  string canonical_symbol = 3;   // 逻辑符号，展示/比较用
  BuySell direction = 4;
  string lots = 5;
  string estimate_price = 6;
}

message OpportunityEvent {
  Opportunity opp = 1;
  enum Action { PUSHED = 0; FILLED = 1; FAILED = 2; EXPIRED = 3; }  // 状态机见 04 §2
  Action action = 2;
}

message ConfirmRequest  { string opportunity_id = 1; }
message ConfirmReply    { bool accepted = 1; string reason = 2; }
```

---

## 4. desk 桥接（C# WPF + grpc-dotnet，Windsurf 落地）

```
core OpportunityStream ──gRPC──► desk C# grpc-dotnet client
                                     │ await foreach (event in call.ResponseStream.ReadAllAsync())
                                     │ ViewModel 更新 ObservableCollection / 触发 PropertyChanged
                                     ▼
                                 WPF UI 自动刷新（数据绑定）

desk 确认: WPF Button → ICommand →
              client.ConfirmOpportunityAsync(new ConfirmRequest { Id = id })
              │ grpc-dotnet unary ──► core 触发 pipeline (04 §3)
```

---

## 5. 完整 proto 定义（落地版）

> 本节是 `DashboardService` 的**完整 RPC 清单 + 机会闭环 message 字段级定义**，作为 Windsurf 落地 proto 的权威依据。
> 现有 message（`SpreadMatrixReply` / `PositionWatchReply` / `ManualOrderRequest` 等）的**全字段不在此重复**——见现有 `proto/dashboard/dashboard.proto`。
> decimal 用 `string`（warm/cold path，constraints §四），时间用 `int64 unix_ms`（constraints §四 4.2）。

### 5.1 完整 RPC 清单

| RPC | 类型 | 用途 | 来源 |
|---|---|---|---|
| `SpreadMatrix` | server stream | 价差矩阵实时推送 | 现有 |
| `PositionWatch` | server stream | 持仓 + 浮动 PnL 推送（长期监控） | 现有 |
| `OpportunityStream` | **server stream** | **core → desk：机会推送 + 状态变更（PUSHED/FILLED/FAILED/EXPIRED）** | **★新增（04 §3）** |
| `ConfirmOpportunity` | **unary** | **desk → core：你确认执行 → 触发 pipeline（04 §3）** | **★新增（04 §3）** |
| `SubmitOrder` | unary | 手动下单（与机会流程并存） | 现有 |
| `ClosePosition` | unary | 手动平仓 | 现有 |
| `CancelOrder` | unary | 取消挂单 | 现有 |
| `GetSignalHistory` | unary | 历史信号查询 | 现有 |
| `GetOrderHistory` | unary | 历史订单查询 | 现有 |
| `GetDailySummary` | unary | 日资金汇总 | 现有 |
| `GetAccountSnapshots` | unary | 各 broker 账户快照 | 现有 |
| `BrokerSearch` / `AddBroker` / `RemoveBroker` / `GetBrokerOrderHistory` / `GetBrokerSymbols` | unary | broker 管理（desk 管理 Tab） | 现有 |
| `SubscribeSymbols` / `UnsubscribeSymbols` / `ListSubscribedSymbols` | unary | 品种订阅管理 | 现有 |
| `GetStrategyStatus` / `ToggleStrategy` / `ResumeStrategy` / `ResetGlobalCircuitBreaker` | unary | 策略开关 / 熔断恢复（04 §4） | 现有 |
| `Kill` | unary | Kill Switch（04 §4，全局紧急停止） | 现有 |
| `Resume` | unary | 解除 Kill Switch | 现有 |
| `GetKillSwitchStatus` | unary | Kill Switch 当前状态 | 现有 |
| `TailLogs` | server stream | core 日志尾巴（desk 运维） | 现有 |

> service 定义形态：`rpc OpportunityStream(Empty) returns (stream OpportunityEvent);` + `rpc ConfirmOpportunity(ConfirmRequest) returns (ConfirmReply);` 追加到现有 `service DashboardService`（04 §5）。其余 RPC 签名**不变**（见现有 `dashboard.proto`）。

### 5.2 机会闭环 message（字段级定义）

```protobuf
// ===== 枚举 =====

enum OppType {
  OPP_TYPE_UNSPECIFIED = 0;
  OPP_TYPE_CROSS_EXCHANGE = 1;   // 跨所价差（03 §2.1，最优先）
  OPP_TYPE_CARRY           = 2;  // 对冲套息（03 §2.2）
  OPP_TYPE_TRIANGULAR      = 3;  // 三角（03 §2.3）
}

enum OppStatus {
  OPP_STATUS_UNSPECIFIED = 0;
  OPP_STATUS_PUSHED      = 1;   // Evaluator 判 Executable=true，已推 desk
  OPP_STATUS_CONFIRMED   = 2;   // 你点了确认（04 §2）
  OPP_STATUS_EXECUTING   = 3;   // 进入 pipeline
  OPP_STATUS_FILLED      = 4;   // 全部腿成交
  OPP_STATUS_FAILED      = 5;   // 部分腿失败，已对冲（04 §3）
  OPP_STATUS_EXPIRED     = 6;   // 报价过期 / 你忽略
}

enum BuySell {
  BUY_SELL_UNSPECIFIED = 0;
  BUY_SELL_BUY  = 1;
  BUY_SELL_SELL = 2;
}

// 腿经济角色（02 §5）—— Carry 才有意义，其他策略 UNSPECIFIED 用 direction 区分
enum LegRole {
  LEG_ROLE_UNSPECIFIED = 0;  // CrossExchange / Triangular：两腿经济对称
  LEG_ROLE_INCOME      = 1;  // Carry 收息腿（正 swap）
  LEG_ROLE_HEDGE       = 2;  // Carry 对冲腿（负 swap 成本）
}

// 机会事件动作（OpportunityEvent.action）
enum OpportunityAction {
  OPP_ACTION_UNSPECIFIED = 0;
  OPP_ACTION_PUSHED  = 1;   // 新机会推送
  OPP_ACTION_UPDATED = 2;   // 状态/字段更新（如倒计时/重新评估）
  OPP_ACTION_FILLED  = 3;   // 成交
  OPP_ACTION_FAILED  = 4;   // 失败
  OPP_ACTION_EXPIRED = 5;   // 过期
}

// ===== 核心 message =====

// Opportunity 见 02 §5。decimal 字段用 string（warm path），时间用 int64 unix_ms。
message Opportunity {
  string id = 1;
  OppType type = 2;
  repeated Leg legs = 3;
  int64 quote_time_unix_ms = 4;        // 价格采样时刻（公理④新鲜度基准）

  // —— 成本拆解（warm path, decimal，constraints §四）——
  string gross_profit     = 5;         // 毛利差（本币换算 USD 后，02 §3）
  string spread_cost      = 6;         // 点差成本
  string commission_cost  = 7;         // 手续费
  string slippage_cost    = 8;         // 滑点预估（归因 P95，07 §4）
  string swap_cost        = 9;         // swap 预估（按预期持仓时长）
  string net_profit       = 10;        // = GrossProfit − 上述全部（02 §4.1）
  string net_bps          = 11;        // 统一绝对度量（跨机会排序）

  // —— Carry 专用：长期持仓年化度量（02 §5.1）——
  string net_swap_per_day = 16;        // Carry：净日 swap（USD，+ = 收入）；其他策略不用
  int32  hold_days_hint   = 17;        // Carry：预期持仓天数（年化换算分母）
  string annualized_net_bps = 18;      // Carry：组合年化（02 §5.1，desk 主度量列）

  // —— 准确性（公理④）——
  int64 expires_at_unix_ms = 12;       // 报价有效期（Evaluator 设，02 §6）
  bool   executable        = 13;       // 可执行性预检结果
  double confidence        = 14;       // P1 占位（归因校准，02 §6）

  OppStatus status = 15;               // 状态机（04 §2）
}

message Leg {
  string broker = 1;                   // "ICMarketsSC-Demo"
  string broker_symbol = 2;            // 原始符号，下单透传（不归一化，02 §2）
  string canonical_symbol = 3;         // 逻辑符号，展示/比较用
  BuySell direction = 4;
  string lots = 5;                     // decimal string（warm path，对冲手数 02 §3.1 归一化）
  string estimate_price = 6;           // 估价（采样 bid/ask，decimal string）
  LegRole role = 7;                    // 经济角色（Carry 收息/对冲，02 §5）
  string daily_swap = 8;               // 该腿日 swap（Carry，decimal string，02 §4.2）
  string annualized_bps = 9;           // 该腿年化（Carry，decimal string，02 §5.1）
}

// OpportunityStream 推送事件（一次机会的完整快照 + 动作）
message OpportunityEvent {
  string id = 1;                       // = Opportunity.id
  Opportunity opp = 2;                 // 全量快照（每次推全量，desk 直接覆盖）
  OpportunityAction action = 3;        // PUSHED / UPDATED / FILLED / FAILED / EXPIRED
  int64  timestamp_unix_ms = 4;        // 事件产生时刻
  string reason = 5;                   // FAILED/EXPIRED 时填原因（如 revalidate 价偏/盲区）
}

// 你确认执行（desk → core unary，触发 pipeline，04 §3）
message ConfirmRequest {
  string opportunity_id = 1;
}
message ConfirmReply {
  bool   accepted = 1;                 // true = 已进入 pipeline；false = 被拒（机会已 Expire/状态非 Pushed）
  string reason   = 2;                 // 被拒原因
}

// OpportunityStream 入参（占位，预留过滤参数）
message OpportunityStreamRequest {
  // 预留：类型过滤、最小 net_bps 过滤（第一版空 = 全推）
}
```

### 5.3 字段类型约定（constraints §四）

| 类型 | proto 表示 | 说明 |
|---|---|---|
| 金额/价格/bps（warm-cold path decimal） | `string` | desk 侧 `decimal.Parse`；**不用 `double`** 传金额 |
| 时间 | `int64 *_unix_ms` | 毫秒级 UTC Unix 时间戳（constraints §四 4.2） |
| 枚举 | proto `enum` | 每个枚举 `*_UNSPECIFIED = 0` 作哨兵 |
| 布尔可执行性 | `bool` | `executable` |
| 置信度（无精度要求） | `double` | 仅展示用，非计价 |

### 5.4 现有 message 指引

下列 message **全字段定义见现有 `proto/dashboard/dashboard.proto`**，本文不重复：
- `SpreadMatrixRequest` / `SpreadMatrixReply`（含 `BrokerRow` / `SpreadCell`）
- `PositionWatchRequest` / `PositionWatchReply`（含 `BrokerPosition` / `Position`）
- `ManualOrderRequest` / `ManualOrderReply`、`ClosePositionRequest/Reply`、`CancelOrderRequest/Reply`
- `SignalHistoryRequest/Reply`、`OrderHistoryRequest/Reply`、`DailySummaryRequest/Reply`
- `AccountSnapshotRequest/Reply`
- `KillRequest/Reply`、`ResumeRequest/Reply`、`KillSwitchStatusRequest/Reply`
- `SubscribeSymbolsRequest/Reply` 等、`StrategyStatusRequest/Reply`、`ToggleStrategyRequest/Reply` 等
- `SearchBrokerRequest/Reply` 等、`BrokerSymbolsRequest/Reply`、`TailLogsRequest/Reply`

> 现有 `SignalHistoryReply.SignalItem.legs_json`（JSON string）与新 `Opportunity.legs`（结构化 `repeated Leg`）形态不同——历史信号查询保留现有 JSONB 透传；新机会闭环走结构化 Leg。两者并存（历史只读 vs 实时结构化）。

---

## 6. 回溯
- OpportunityStream / ConfirmOpportunity / Opportunity 状态机 → 04 §3、§5
- Opportunity / Leg 字段（成本拆解 / ExpiresAt / Confidence / 腿角色 LegRole / Carry 年化）→ 02 §5、§5.1、§6
- decimal as string / int64 unix_ms → constraints §四
- Push-First（stream）/ grpc-dotnet unary → constraints §一、§三
- 现有 RPC / message → 现有 `proto/dashboard/dashboard.proto`
