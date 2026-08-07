# 04 · 人机闭环

> 定义 `Opportunity` 的生命周期主干：**发现 → 评估 → 推送 → 你确认 → 执行 → 归因**。
> 依据 D-003（混合模式）+ `00 §3`（准确性两层）+ `02`（Opportunity 对象）。
> 这是系统骨架；`03`（detector 细节）、`06`（接口 proto）是本文档的填充。

---

## 1. 核心理念：为什么是「人确认」而非全自动

`00 §3` 已论证："准确无误"分两层——**发现时准确**（可做到）与**执行后准确**（跨 broker 真正同时成交物理上不可能 100%）。

**人工确认夹在两层之间**：系统在发现层保证机会为真（Evaluator），你在执行前再看一眼当前盘口决定要不要下手——这正是执行层不确定性里能多争取的那一层确定性。这是 D-003「混合模式」的物理依据，不是 UX 偏好。

---

## 2. Opportunity 生命周期（状态机）

```
  Detector            Evaluator          推送 desk        你点确认
 ──►Candidate──►[评估]──►Pushed──►Confirmed──►Executing──►Filled
                     │                  │                └──►Failed
                     │(不可执行/          │(忽略/过期)
                     │ 净利不足)          └──►Expired
                     ▼
                    丢弃                        (成交后)归因回填
```

Detector 产出 `Candidate`（无状态，仅价差）；下表为 **Opportunity 状态**（从 `Pushed` 起）：

| 状态 | 含义 | 谁触发转换 |
|---|---|---|
| `Pushed` | Evaluator 判定 Executable=true，已推送到 desk | Evaluator |
| `Confirmed` | 你点了确认 | 你（ConfirmOpportunity） |
| `Executing` | 进入执行管线 | core（确认后自动） |
| `Filled` | 全部腿成交 | pipeline |
| `Failed` | 部分腿失败，已对冲 | pipeline |
| `Expired` | 报价过期/你忽略 | core（ExpiresAt 超）/ 你 |

> 同一机会在任何状态都可被 Kill Switch（全局）强制终止 → 撤单/平仓。

---

## 3. 端到端流程

```
[Detector]  发现候选（跨所价差/三角/套息，仅价差）
     │
     ▼
[Evaluator] 评估（02 §6）：扣全成本算 NetProfit → 可执行性预检 → 设 ExpiresAt
     │  Executable=true 的 → Opportunity(Pushed)
     ▼
[OpportunityStream]  core ──gRPC server stream──► desk
     │
     ▼
[desk 机会列表]  你看到：类型/腿/NetBps+USD/成本拆解/倒计时/置信度
     │  你审，点「确认执行」
     ▼
[ConfirmOpportunity]  desk ──gRPC unary──► core
     │
     ▼
[ExecutionPipeline]  仅确认后触发：
     │   1. revalidate：重新拉各腿最新报价，价偏 > MaxSlippage → 放弃(Expired)
     │   2. capital gate：资金门禁
     │   3. 并发下单所有腿（all-or-nothing）
     │   4. 任一腿失败 → 已成交腿反向对冲（hedge）
     ▼
[归因]  实际成交价/swap/commission 回填 Opportunity（预估 vs 实际偏差）
     │  → 校准 Evaluator 的滑点/swap/commission 预估（讨论五自适应）
     ▼
  Filled / Failed
```

---

## 4. 人机交互（desk 侧，UI 由 Windsurf 实现）

**机会列表（新增视图，或 Matrix 视图的可执行机会区）**：
- 每个机会卡片：类型 / 腿（broker·品种·方向·量）/ **NetProfit（bps + USD）** / 成本拆解（点差/手续费/滑点/swap）/ 有效期倒计时 / 置信度。
- **「确认执行」按钮** → C# grpc-dotnet `client.ConfirmOpportunityAsync(new ConfirmRequest { Id = id })`（WPF 命令绑定）。
- 忽略 → 机会自然 Expire（ExpiresAt 到期）。

**已有控制权保留**：
- **Kill Switch**：全局紧急停止（撤单+平仓）。
- **策略开关 ToggleStrategy**：启用/禁用某类 detector（如只跑跨所，关掉三角）。
- **熔断恢复**：熔断后须人工确认才恢复（`evaluation-framework.md:942`）。

**手动交易通道（保留）**：`SubmitOrder` 是「不经过机会流程」的直接手动下单（调试/救急用），与机会流程并存、不冲突。

---

## 5. gRPC 接口设计（proto，Windsurf 落地）

> 仅定义 message/service 形态；proto 文件 + Go 实现由 Windsurf。

```
service DashboardService {
  // 已有：SpreadMatrix, PositionWatch, SubmitOrder, ClosePosition, ...

  // 新增：机会推送（core → desk，server stream）
  rpc OpportunityStream(Empty) returns (stream OpportunityEvent);

  // 新增：你确认机会（desk → core，unary）
  rpc ConfirmOpportunity(ConfirmRequest) returns (ConfirmReply);
}

message OpportunityEvent {
  string id;
  Opportunity opp;        // 见 02 §5
  enum Action { PUSHED, FILLED, FAILED, EXPIRED } action;
}
```

机会状态变更（Filled/Failed/Expired）也经同一 stream 推回 desk，让你的列表实时更新。

---

## 6. 准确性两层落地（呼应 00 §3）

| 层 | 保证手段 | 落在 |
|---|---|---|
| **发现时准确** | Evaluator：全成本净盈利 + 报价新鲜度 + 可执行性预检 | 02 §6 |
| **确认时再判** | 你执行前看当前盘口/倒计时，人工把关 | 本文档 §4 |
| **执行兜底** | revalidate（价偏放弃）+ all-or-nothing + 失败对冲 | 现有 pipeline.go（保留） |

---

## 7. 与现有代码的关系（给 Windsurf 的改动指引）

| 现有 | 处置 |
|---|---|
| `execute/pipeline.go`（4 阶段，代码注释为 5 个 Phase 标签 1/1.5/2/3/4，功能上 4 阶段） | **保留** revalidate/gate/并发下单/对冲逻辑；**改为仅被 `ConfirmOpportunity` 触发**，不再被策略引擎自动调用（D-003）。删掉 `_ = pipeline` 的悬空。 |
| `dashboard/handlers.go` 的 `SubmitOrder/ClosePosition` | 保留（手动通道）。 |
| `dashboard/server.go` | 新增 `OpportunityStream`（推送）+ `ConfirmOpportunity`（触发 pipeline）handler；新增机会仓库（内存 + PG 持久化）。 |
| `cmd/core/main.go` | 串联 Detector → Evaluator → 机会仓库 → OpportunityStream；pipeline 接 ConfirmOpportunity。 |
| desk / 前端 | C# WPF 全新（不复用旧 Wails）：新增机会列表视图 + 确认按钮（ICommand → grpc-dotnet unary）。 |

---

## 8. 回溯

- 人工确认位 / 非全自动 → D-003、`00 §3`
- Opportunity 对象 / 状态 → `02 §5`
- Evaluator（发现层准确）→ `02 §6`
- 执行 all-or-nothing + 对冲 → 现有 `execute/pipeline.go`
- detector 细节 → `03-strategies.md`
- gRPC proto 细节 → `06-interfaces.md`
