# 实践记忆 — Claude ↔ Windsurf 共享

> **开工前必读。** 这里沉淀了历次审查中反复出现的问题——不是规则（规则在 AGENTS.md），
> 而是"在我们这个项目里，代码怎么写才不会被打回来"。
> Claude 每次复审后更新此文件；Windsurf 每次开工前通读。

---

## 1. 高频打回模式（按频次排序）

### 1.1 写了但没接上（wired-but-not-connected）

**症状**：参数/字段在 A 处收集了，在 B 处没传过去，静默丢弃。
**实例**：F4 — OrderRequest 加了 StopLoss/TakeProfit，handler 也收了 req.StopLoss/req.TakeProfit，
但 `adapter.OrderRequest` 结构体没这两个字段 → SL/TP 被静默丢弃。
**自查**：加一个字段时，**顺着数据流从头到尾追一遍**——proto → Go struct → adapter → RPC request。
每层都到了才算接上。

### 1.2 零值静默（silent-zero）

**症状**：字段有值可填，但没填，proto/UI 显示零值/epoch/"0.0"。
**实例**：F2 — SignalItem 的 TimestampUnixMs/GrossBps/NetBps 在 handler 里没赋值，C# UI 全显示 0。
**自查**：每新增一个 proto 字段或 Go struct 字段，grep 所有构造该结构体的地方，确认新字段已赋值。

### 1.3 结构体 ≠ 数据库 schema（struct-schema-mismatch）

**症状**：Go struct 字段名/类型和 SQL 列名/类型不对齐，SQL 执行时报错或读到零值。
**实例**：F1 — SignalRecord 有 PnL/Status 字段但 DB 里根本没这两列；DB 有 gross_bps/net_bps 但 struct 里没有。
**自查**：任何涉及 DB 读写的新 struct，**打开 migrations/ 目录对应的 DDL 文件，逐列对齐**。

### 1.4 上下文断裂（context-break）

**症状**：用 `context.Background()` 而不是从父 goroutine 派生 ctx——shutdown 时无法取消在途操作。
**实例**：Phase F — `ConfirmOpportunity` 里 `go executeConfirmed(context.Background(), opp)`，
engine shutdown 后 pipeline 可能还在跑。
**自查**：任何 `go` 启动的 goroutine，ctx 从哪来？如果是 Background()，问自己：shutdown 时这个 goroutine 该停吗？

### 1.5 测试只覆盖快乐路径（happy-path-only）

**症状**：有正常流程测试，缺错误路径、边界条件、状态机拒绝分支。
**实例**：Phase F engine_test.go — 有 Confirm/PipelineError/NotFound/NotPushed，但没有 `Executable=false`。
**自查**：每个 if 分支、每个 error return、每个状态机拒绝条件——至少一个测试用例。

---

## 2. 代码风格约定（不写进 AGENTS.md 的工程直觉）

### Go

- **decimal 构造**：测试代码也优先 `decimal.RequireFromString("0.1")`，不用 `decimal.NewFromFloat(0.1)`。
  测试写多了会漏到生产代码。
- **锁**：有 early return 的函数用显式 `mu.Unlock()`，不用 `defer`（Phase F engine.go 是正确示范）。
- **纯函数优先**：业务逻辑包里不调 I/O、不调外部服务。I/O 在 adapter/store 层。
- **测试 helper 命名**：`make*`（构造对象）、`new*`（构造+注册）、`wait*`（阻塞等待）。
- **注释**：为什么，不是做什么。英文。
- **文件行数**：新文件尽量 < 200 行；超过 300 行考虑拆文件。硬红线 450。

### SQL / DDL

- `NUMERIC(20,8)` 给金额字段（不要 `FLOAT`/`DOUBLE`）。
- 表名复数（`opportunities`、`audit_events`）与 001_init.sql 风格一致。
- 每个迁移用 `BEGIN; ... COMMIT;` 包起来。

### Proto

- 时间字段用 `int64 unix_ms`，不用 `google.protobuf.Timestamp`。
- Decimal 字段用 `string`（`decimal.String()` 往返）——这条也是 protobuf 和 SQL 的共同约定。
- **审计日志 = protobuf 长度前缀格式**（不是 JSON Lines）。17-audit.md 2026-08-08 已修正（D-015）。
  人读用 `protoc --decode`，不牺牲严谨换便利。
- 枚举从 0 开始，0 = 未指定/默认值。

---

## 3. 本项目的"味道"（smells）

这些不是 bug，但说明代码可能需要重构：

- **函数 > 50 行** — 检查是否该拆。
- **文件 > 300 行** — check-lines.sh 会 warn，考虑拆。
- **import 了不该 import 的包** — 对照 code-map.md 依赖层级图。
- **新类型/枚举和已有的重复** — grep 一下再新建。
- **`// TODO` 或 `// FIXME`** — 零容忍，要么做要么删（AGENTS §C）。

---

## 4. 动手前的思考清单（写第一行代码之前）

> 这是 Claude 思考方法的外化——不是规则，是问自己的问题。
> 顺序有意为之：先理解、再判断、后动手。

```
[ ] Q1: 这个东西已经存在了吗？
    → grep 相关符号 + 读 code-map.md §7 文件清单 + 查 STATE.md。
    如果已存在：为什么需要重写而不是扩展？（AGENTS §2.2）

[ ] Q2: 上次正常工作的版本是哪个？
    → git log --oneline --all -- <相关路径>。
    如果是修 bug：是哪次 commit 引入的？（AGENTS §2.1）

[ ] Q3: 这个任务跨越了几个语义范围？
    → 如果 > 1 个：拆成多个 task，先做完一个再做下一个。（AGENTS §2.3）

[ ] Q4: 问题的本质是什么？
    → 用一句话说清楚。如果一句话说不清，说明还没想透。
    然后问：最直接的解法是什么？不要套模板——从问题本质推导解法。（AGENTS §B）

[ ] Q5: 我 import 的包在哪个层级？
    → 对照 code-map.md 依赖层级图。被依赖的层不能 import 依赖它的层。
    新增依赖方向 = 架构变更 = 需要 Claude 确认。

[ ] Q6: 有没有更简单的做法？
    → 少一个文件？少一层抽象？复用已有的而不是新建？
    如果存在更简单但你没选的方案——说明为什么。
```

## 5. 提交前自检清单（代码写完后）

```
[ ] 每个新增字段：数据流从头到尾追过？每一层都赋值了？（防 1.1）
[ ] 每个新增 proto 字段：所有构造此 message 的地方都填了？（防 1.2）
[ ] 涉及 DB 的 struct：和 migrations/ DDL 逐列对齐？（防 1.3）
[ ] 每个 go routine：ctx 从正确的父 context 派生？（防 1.4）
[ ] 每个 if/error/状态拒绝：至少一个测试？（防 1.5）
[ ] grep TODO/FIXME/nolint：零命中？
[ ] go build && go vet && go test -race && ./scripts/check-lines.sh 全过？
```

---

> 最后更新：2026-08-08（从 Phase A/E/F 审查结论提炼）
