# 14 · Dashboard 机会闭环接线

> Engine 已有 Snapshot→Detect→Evaluate→push 管道 + sub/pub。
> 本阶段把 Engine 的 `Subscribe()` 接到 gRPC `OpportunityStream`，实现 `ConfirmOpportunity` unary，
> 闭环「发现→评估→推送→确认→执行」中最后两个环节。
> 依据：`06-interfaces.md §5.2`（proto 字段级定义）、`04-human-in-loop.md §2`（状态机）、现有 `engine/engine.go`（sub/pub 已就位）。

---

## 0. 定位

```
Engine.sub/pub ──► dashboard.OpportunityStream (gRPC server stream) ──► desk WPF
                       ▲
desk WPF ──► ConfirmOpportunity (gRPC unary) ──► Engine.ConfirmOpportunity()
```

本阶段只接线，不改 Engine 逻辑，不改 Detector/Evaluator。

---

## 1. Proto 变更（`proto/dashboard/dashboard.proto`）

`06 §5.2` 的 message 和 enum 全部搬进 `dashboard.proto`：

```
enum OppType { ... }          // CROSS_EXCHANGE=1 / CARRY / TRIANGULAR
enum OppStatus { ... }        // PUSHED=1 / CONFIRMED / EXECUTING / FILLED / FAILED / EXPIRED
enum BuySell { ... }          // BUY=1 / SELL
enum LegRole { ... }          // UNSPECIFIED=0 / INCOME / HEDGE
enum OpportunityAction { ... } // PUSHED=1 / UPDATED / FILLED / FAILED / EXPIRED

message Opportunity { ... }   // 06 §5.2 完整定义，decimal→string，时间→int64 unix_ms
message Leg { ... }
message OpportunityEvent { ... }
message ConfirmRequest { opportunity_id }
message ConfirmReply { accepted, reason }
```

新增 RPC（追加到 `service DashboardService`）：

```
rpc OpportunityStream(OpportunityStreamRequest) returns (stream OpportunityEvent);
rpc ConfirmOpportunity(ConfirmRequest) returns (ConfirmReply);
```

`buf generate` 后 Go/C# 两端重新生成。

---

## 2. Dashboard Go 实现

### opportunity.go（~100 行）

```go
// OpportunityStream 订阅 engine，每收到事件 → stream.Send(event)
func (s *Server) OpportunityStream(req *dashpb.OpportunityStreamRequest, stream dashpb.DashboardService_OpportunityStreamServer) error {
    ch, cancel := s.engine.Subscribe()
    defer cancel()
    for {
        select {
        case <-stream.Context().Done():
            return nil
        case ev := <-ch:
            pbEv := toProtoEvent(ev)
            if err := stream.Send(pbEv); err != nil {
                return err
            }
        }
    }
}

// ConfirmOpportunity 确认执行
func (s *Server) ConfirmOpportunity(ctx context.Context, req *dashpb.ConfirmRequest) (*dashpb.ConfirmReply, error) {
    opp, reason := s.engine.ConfirmOpportunity(req.OpportunityId)
    if opp == nil {
        return &dashpb.ConfirmReply{Accepted: false, Reason: reason}, nil
    }
    return &dashpb.ConfirmReply{Accepted: true}, nil
}
```

### 类型映射辅助（~60 行）

`toProtoEvent(engine.OpportunityEvent) *dashpb.OpportunityEvent`：
- `evaluator.Opportunity` → `dashpb.Opportunity`（decimal→string，time→unix_ms，枚举→proto enum）
- `evaluator.OppLeg` → `dashpb.Leg`

放在 `internal/dashboard/opportunity.go`。

### Server 结构变更

`Deps` 加 `Engine *engine.Engine` 字段（main.go 已传，见 `dashboard.NewServer(dashboard.Deps{..., Engine: eng})`）。

---

## 3. 文件布局

```
internal/dashboard/
  opportunity.go     ~160  — OpportunityStream + ConfirmOpportunity handler + 类型映射
```

（其余 dashboard 文件不动）

---

## 4. 测试

```go
func TestOpportunityStream_Push(t *testing.T) {
    // 构造 engine → subscribe → gRPC stream recv
    // 验证收到 PUSHED 事件，字段映射正确（decimal→string, time→unix_ms）
}

func TestConfirmOpportunity_Success(t *testing.T) {
    // engine.ConfirmOpportunity 成功 → accepted=true
}

func TestConfirmOpportunity_NotFound(t *testing.T) {
    // 不存在 → accepted=false, reason 非空
}
```

Engine 依赖 mock：用 `bus.New()` + fake listing cache 构造最小 Engine，或直接测 handler 层（mock engine Subscribe）。

---

## 5. 回溯

- proto 字段 → 06 §5.2
- 状态机 Pushed→Confirmed→... → 04 §2
- Engine sub/pub → `internal/engine/engine.go`
- Evaluator 产出 Opportunity → 12
- 固定：不碰 Detector/Evaluator/MT4，只接线。
