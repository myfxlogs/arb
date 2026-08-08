# 17 · Audit 审计 + 归因记账

> 「准确无误」的闭环发动机：成交后用实际 swap/commission/滑点校准 Evaluator 的预估参数。
> 依据：`07-risk-audit.md §3/§4`、`00 公理③`（漏成本 = 假机会）。

---

## 0. 为什么必须是 Protobuf

审计日志是归因校准的**数据源**——滑点 P95、swap 偏差、commission 偏差全从它算。
JSON 的问题：
- 没有 schema 校验——字段拼错、类型漂移，写的时候不报错，读的时候才发现数据脏了
- 数字精度——所有 number 都是 float64，`0.1` 变成 `0.10000000000000001`
- 不严谨 = 校准不准 = 「准确无误」假

protobuf = 编译期 schema 保证 + 精确 decimal（string 字段）+ 原生二进制高性能。
人读用 `protoc --decode` 或 `tools/readaudit`，不牺牲严谨换便利。

---

## 1. Proto 定义

新增 `proto/audit/audit.proto`：

```protobuf
syntax = "proto3";

package arb.audit;
option go_package = "arb/proto/gen/audit;auditpb";

import "google/protobuf/timestamp.proto";

// AuditEvent is a single audit log entry. Written as a length-delimited
// protobuf message to a local file (constraints §二 2.1: 审计日志本地文件).
message AuditEvent {
  google.protobuf.Timestamp timestamp = 1;

  // Event classification
  EventType type = 2;

  // Opportunity lifecycle
  string opportunity_id = 3;

  // Snapshot of evaluator fields at event time (for FILLED events,
  // these are the pre-execution estimates; actuals are in order_result).
  string gross_profit_est = 4;     // decimal string
  string net_profit_est = 5;       // decimal string
  string net_bps_est = 6;          // decimal string
  int32 leg_count = 7;

  // Present only for FILLED/FAILED events
  OrderResult order_result = 8;

  // Free-form detail (broker name, reason, etc.)
  string detail = 9;
}

enum EventType {
  EVENT_TYPE_UNSPECIFIED = 0;
  EVENT_TYPE_DETECTED = 1;
  EVENT_TYPE_PUSHED = 2;
  EVENT_TYPE_CONFIRMED = 3;
  EVENT_TYPE_FILLED = 4;
  EVENT_TYPE_FAILED = 5;
  EVENT_TYPE_EXPIRED = 6;
}

// Actual execution result for a single leg.
message LegResult {
  string broker = 1;
  string broker_symbol = 2;
  string direction = 3;      // "BUY" / "SELL"
  string lots = 4;           // decimal string
  string est_price = 5;      // decimal string — evaluator estimate
  string actual_price = 6;   // decimal string — fill price
  double slippage_bps = 7;   // (actual - est) / est * 10000
  string swap = 8;           // decimal string — actual swap charged
  string commission = 9;     // decimal string — actual commission charged
  uint64 ticket = 10;        // broker order ticket
}

message OrderResult {
  repeated LegResult legs = 1;
  string total_swap = 2;           // decimal string
  string total_commission = 3;     // decimal string
  string total_slippage_bps = 4;   // decimal string — weighted avg
  string actual_net_profit = 5;    // decimal string — 扣完成本后的真实净盈利
}
```

---

## 2. 审计 Logger

### 包：`internal/audit/`

```go
type Logger struct {
    mu   sync.Mutex
    file *os.File
}

func NewLogger(path string) (*Logger, error)   // 创建/追加文件
func (l *Logger) Log(ev *auditpb.AuditEvent) error  // 同步写，不加 goroutine
func (l *Logger) Close() error
```

**写入格式**：length-delimited protobuf stream（标准做法）。
每个 event = 4 字节 varint 长度前缀 + proto.Marshal 的二进制 body。
兼容 `proto.Unmarshal` 逐条读回，也兼容 `protoc --decode` pipeline。

```go
func (l *Logger) Log(ev *auditpb.AuditEvent) error {
    l.mu.Lock()
    defer l.mu.Unlock()
    b, err := proto.Marshal(ev)
    if err != nil {
        return err
    }
    // Length-delimited format: varint(len) + bytes
    buf := make([]byte, binary.MaxVarintLen64)
    n := binary.PutUvarint(buf, uint64(len(b)))
    if _, err := l.file.Write(buf[:n]); err != nil {
        return err
    }
    _, err = l.file.Write(b)
    return err
}
```

---

## 3. 人读工具

### `tools/readaudit/main.go`

```go
// readaudit reads audit.pb (length-delimited protobuf) and prints as
// protobuf text format to stdout. Usage: readaudit audit.pb | grep OPP_ID
```

或者直接用 protoc：
```bash
cat audit.pb | protoc --decode=arb.audit.AuditEvent proto/audit/audit.proto
```

不需要单独工具。`protoc --decode` 已是标准方案。

---

## 4. Engine 埋点

5 处 Log 调用，与 JSON 版本相同：

| 位置 | 事件 | 携带字段 |
|------|------|---------|
| `scanOnce` 产出 Candidate | `DETECTED` | OppID + 预估 net_bps/gross |
| `scanOnce` push 时 | `PUSHED` | OppID + 完整预估快照 |
| `ConfirmOpportunity` | `CONFIRMED` | OppID |
| `executeConfirmed` 成功 | `FILLED` | OppID + OrderResult（含 4 项实际成本） |
| `executeConfirmed` 失败 | `FAILED` | OppID + OrderResult + detail（失败原因） |

---

## 5. 归因查询（Phase F 数据驱动，本阶段建表 + 写行）

DDL 已就位：`migrations/003_opportunity.sql`（opportunities + audit_events 表）。

归因用 SQL 直接从 PG 查（不是从 audit.pb 文件）：
```sql
-- 实际滑点 P95 → 校准 Evaluator.Cfg.SlippageBps
SELECT percentile_cont(0.95) WITHIN GROUP (ORDER BY deviation_slippage_bps)
FROM opportunities WHERE status = 'Filled';

-- swap 预估 vs 实际偏差
SELECT avg(deviation_swap) FROM opportunities WHERE status = 'Filled';
```

audit.pb 文件 = 不可篡改的完整事件流（append-only，不留空洞）；
PG opportunities 表 = 结构化查询 + 归因统计。

---

## 6. 文件布局

```
proto/audit/
  audit.proto                    # AuditEvent + LegResult + OrderResult + EventType

internal/audit/
  audit.go        ~100行 — Logger (New/Log/Close，length-delimited protobuf)
  audit_test.go   ~60行  — 写 2 event → 读回 → 断言字段

internal/engine/engine.go  # +audit.Logger, 5 处 Log 调用
internal/store/             # +opportunities.go (WriteOpportunity / UpdateOpportunityFilled)
cmd/core/main.go            # 创建 audit.Logger, 传入 engine.Deps

tools/readaudit/
  main.go          ~30行  — 可选的便利工具（protoc --decode 已是标准方案，非必须）
```

---

## 7. 测试

```go
func TestAuditLog_WriteRead(t *testing.T) {
    dir := t.TempDir()
    l, _ := audit.NewLogger(filepath.Join(dir, "audit.pb"))
    ev := &auditpb.AuditEvent{
        Type: auditpb.EventType_EVENT_TYPE_DETECTED,
        OpportunityId: "opp-1",
        NetBpsEst: "3.5",
    }
    l.Log(ev)
    l.Close()

    // Read back via protobuf
    f, _ := os.Open(filepath.Join(dir, "audit.pb"))
    // ... read length-delimited, proto.Unmarshal, assert fields
}
```

---

## 8. 回溯

- 归因 = 准确无误闭环 → `07 §4`、`00 公理③`
- 审计日志固定使用 protobuf（不是 JSON Lines）→ `constraints §二 2.1`
- 同步写（不加 goroutine）→ `code-map §4`
- Evaluator 参数 P1 初值 → `07 §1`
