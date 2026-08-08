# 16 · 执行接线 + Notional 替换

> Engine.ConfirmOpportunity 目前只改状态。本阶段把它接到 ExecutionPipeline，
> 让用户确认的机会真正下单。同时用 Evaluator 的真实 `NotionalUSD` 替换
> `execute/pipeline.go:43` 的硬编码 `×100000`。
> 依据：`04-human-in-loop.md §3`（确认→执行流程）、`07-risk-audit.md §1`（风控三项）。

---

## 1. Engine.ConfirmOpportunity → ExecutionPipeline

```go
// engine/engine.go — ConfirmOpportunity 扩展
func (e *Engine) ConfirmOpportunity(id string) (*evaluator.Opportunity, string) {
    // ... existing state check (Pushed, Executable) ...
    opp.Status = evaluator.OppStatusConfirmed

    // 异步触发执行（不阻塞 gRPC unary 返回）
    go e.executeConfirmed(ctx, opp)

    return opp, ""
}

func (e *Engine) executeConfirmed(ctx context.Context, opp *evaluator.Opportunity) {
    // 1. 转换为 pipeline.ArbitrageOpportunity
    pipeOpp := toPipelineOpp(opp)
    // 2. 走现有 pipeline.Execute（4 阶段：revalidate→gate→concurrent submit→hedge）
    err := e.deps.Pipeline.Execute(ctx, pipeOpp)
    // 3. 结果回填
    if err != nil {
        opp.Status = evaluator.OppStatusFailed
    } else {
        opp.Status = evaluator.OppStatusFilled
    }
    // 4. 广播状态变更
    e.broadcast(OpportunityEvent{Opp: opp, Action: "FILLED"/"FAILED"})
}
```

- gRPC ConfirmOpportunity 立即返回 `{Accepted: true}`，不等待执行完成
- 执行结果异步广播回 desk（`Action: FILLED` / `FAILED`）
- 失败时 Engine 广播事件，desk 看到 Status 变化

---

## 2. Notional 替换

`execute/pipeline.go:43` 当前：
```go
func (o ArbitrageOpportunity) Notional() float64 {
    total := 0.0
    for _, leg := range o.Legs {
        total += leg.Price * leg.Volume * 100000 // hardcoded
    }
    return total
}
```

替换为使用 Evaluator 算好的真实 `NotionalUSD`：
```go
type ArbitrageOpportunity struct {
    Legs       []Leg
    Params     StrategyParams
    Notional   float64  // ★ 新增：真实名义本金（USD），由 Evaluator 算
}

func (o ArbitrageOpportunity) Notional() float64 { return o.Notional }
```

`Engine.executeConfirmed` 中的 `toPipelineOpp` 把 `opp.NotionalUSD` 映射到 `pipeOpp.Notional`。

---

## 3. Engine.Deps 扩展

```go
type Deps struct {
    // ... existing ...
    Pipeline *execute.ExecutionPipeline  // ★ 新增
}
```

main.go 传入已建好的 pipeline（现有代码已有）。

---

## 4. 风控接入

`Pipeline.Execute` 已有 Phase 1.5 `CapitalGate.Allow()`。当前 `CapitalGate` 用 `opp.Notional()` 算敞口，替换后直接使用 `opp.NotionalUSD`（float64 返回）——Natural 函数原型匹配。

风控三项（07 §1）中的「并发未平仓 ≤5」和「单平台资金 ≤40%」在 `CapitalGate.Allow()` 扩展实现——本阶段不重写 CapitalGate，参数化对接。

---

## 5. 文件变更

```
internal/engine/engine.go      # +Pipeline 字段, ConfirmOpportunity 加异步执行, toPipelineOpp
internal/execute/pipeline.go   # ArbitrageOpportunity +NotionalUSD, Notional() 改读字段
cmd/core/main.go               # engine.New 传 pipeline
```

---

## 6. 测试

```go
func TestConfirm_RunsPipeline(t *testing.T) {
    // 构造 engine + mock pipeline
    // Confirm → 断言 pipeline.Execute 被调用, opp 状态 → Filled
}
func TestConfirm_PipelineError(t *testing.T) {
    // mock pipeline 返回 err → opp 状态 → Failed, broadcast FAILED
}
func TestNotional_FromEvaluator(t *testing.T) {
    // ArbitrageOpportunity.NotionalUSD = 108000
    // Notional() → 108000（不再 ×100000）
}
```

---

## 7. 回溯

- confirm→execute 流程 → `04-human-in-loop.md §3`
- 风控三项 → `07-risk-audit.md §1`
- Evaluator.NotionalUSD → `12-evaluator.md`（Evaluator 产出）
- pipeline 四阶段 → existing `execute/pipeline.go`
