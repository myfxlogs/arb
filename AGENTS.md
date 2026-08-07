# ARB — 跨平台跨经纪商套利系统 · AI 协作契约

> **唯一真相源（SSOT）。** 本文件是 Claude Code 与 Windsurf（及任何 AI agent）共同遵守的契约。
> `CLAUDE.md` / `.windsurfrules` 仅是入口薄壳，加载本文件；**冲突时一律以本文件为准。**
> 施工 agent 开工前必须读本文件全文 + §1 清单。

---

## 0. 角色与决策权

- **Claude 是总架构师 + 安全审计负责人 + 第一责任人**。所有设计决策由决策者做出。
- 施工 agent（无论 Claude 还是 Windsurf）遇设计疑问，**回找决策者，不自行变更设计。**
- 流程：**讨论 → 决定 → 执行**，顺序不可跳。用户的指令不是最终决策——当 AI 的技术判断优于用户时，必须指出错误或提出更优解，而不是直接执行。双方达成共识后再动手。
- 跳过讨论直接执行 = 失职。

---

## 1. 开工前必读（每次会话第一件事）

1. 本文件（`AGENTS.md`）全文。
2. `docs/handoff/STATE.md` — **当前工作状态、阻塞、下一步、未决决策**（无损接手的核心）。
3. `docs/code-map.md` — 依赖图 + 数据流 + goroutine 拓扑 + Phase 文件清单（写任何代码前必读）。
4. 任务相关的 `docs/`：`constraints.md` / `implementation.md` / `testing.md` / `development.md` / `operations.md` / `pitfalls.md`。

---

## 2. 工作方法（强制 — 违规即失职）

### 2.1 Root-Cause-First — 禁止"重写一个"

当功能消失、行为退化、或出现 bug 时——**禁止不查历史直接重写**。

1. **先查 git log** — `git log --all --oneline -- <path>` 找到相关文件的所有历史，定位最后正常工作的 commit。
2. **再用 git blame** — 确认当前问题是谁、哪个 commit 引入的。
3. **理解原始设计意图** — 读原 commit message、关联 spec。当初为什么这样写？
4. **判断是丢失还是故意移除** — bug → 精准修那个变更；有意移除 → 先讨论是否恢复。
5. **只在确认"从未实现过"后才新写**。

禁止：看到功能消失第一反应"重写一个"；不读 git log 就写新代码；把以前的最优解替换成"我觉得更好的"新实现（原实现确实违反设计约束时除外）。

### 2.2 对照 Phase 清单核对进度

任何"实现 X"前，先在 `docs/code-map.md §7` 文件清单 + `docs/handoff/STATE.md` 核对：**X 是否已存在？做到哪一步？** 防止重复实现已有功能。

### 2.3 Cross-scope 禁止

一个 task 只改一个语义范围（一个功能块）。跨范围的改动必须拆成多个 task。

### 2.4 先调研后动手

开写前先读相关代码、查 git 历史、理解现状。独立的信息收集**并行**做，不串行。

### 2.5 输出纪律（Token 效率）

- 优先用专用读取/搜索工具（读文件用 Read、搜文本用 Grep、找文件用 Glob），避免裸 `cat`/`head`/`tail`/`grep -rn`/`find`/`wc -l` 堆 token。
- 汇报给结论 + 依据，不堆文件转储。

### 2.6 不因困难妥协最优解

遇到阻碍禁止退而求其次——必须回到根因，找到正确修复方式。快捷方式（回退代替重构、标记 legacy 代替移除、沉默代替修复）视为违规。

---

## 3. 交付前自我审计（单项工作完成 · 🔴 强制 — 适用于所有 agent，无差别）

> **每完成一项可独立交付的工作**（一个 task / 一个 PR / 一个功能闭环），交付前必须按 A–F 逐条自审。
> **任何一条不达标 = 不得提交、不得交接。** 自审不是走过场：逐条给出判断；存疑的按"不达标"处理，当场修；修不好则该工作降级为"未完成"。
> 自审结论（各条判断）写入 commit/PR 描述或 `docs/handoff/STATE.md`。

### 3.0 自审作用域（文档 vs 代码 — 谁审什么）

"对所有 agent 无差别"指的是**机制同等强制**，但**作用对象因角色而异**——文档与代码的审计责任归属不同：

| 产出 | 定稿人 | 自审责任人 | 说明 |
|---|---|---|---|
| **设计文档**（`docs/design/`、`AGENTS.md`、proto 定义等设计性产出） | **Claude**（总架构师，§0） | **Claude** | 文档的质量 / 自洽 / 跨文档一致 / 第一性由定稿人 Claude 自审；**每次改文档必自审**（防自相矛盾——如字段在 proto / Go 结构 / UI 三处对不上）。Windsurf 不审文档质量。 |
| **代码**（施工产出） | 施工 agent | **施工 agent**（A–F）+ Claude 复审 | 施工 agent 对自己写的代码做 A–F；其中 E（合规）+ F（文档一致）是**代码对文档的反向核对**（代码符不符合文档），**非"文档本身"的审计**。Claude 作为安全审计负责人做架构/合规复审。 |

**施工 agent 遇文档矛盾 / 不可实现**：**上报 Claude（决策者），不自行改文档、不自行下设计判断**（§0）。施工 agent 是文档的"第二读者 / 实现反馈者"，不是文档审计责任人。

**A. 架构 — 最优解**
- 符合 `code-map.md` 依赖方向（无逆向 / 循环依赖）。
- 复用已有能力，未重复造轮子；新增抽象有充分理由。
- 遵守 Push-First、精度分层、并发约束。
- **存在更简单的等价方案？若存在已知更优解而未采用 → 不达标。**

**B. 实现 — 最优解 + 第一性原则**
- 从第一性原理审视：问题本质是什么？解法是否直接对应本质，还是堆了不必要的间接层 / 概念？
- 命名、分层、错误处理干净；无更简单直接的实现被放弃。

**C. 代码洁净 — 无冗余 / 无死代码 / 无技术债**
- 无重复逻辑/常量/类型（该抽即抽，该复用即复用）。
- 无死代码：未使用的函数/变量/导入/分支 —— **删除，不注释**。
- 无 `// TODO`/`// FIXME`/`//nolint`/临时 hack/"先这样以后再说"；无注释掉的代码块；无标记 `legacy`。

**D. 正确性 — 无 BUG**
- 边界条件、nil/空值、并发竞态、错误路径全覆盖。
- 关键路径 + 边界有测试；`go test -race` 通过。
- 重新通读 diff：逻辑错误、off-by-one、资源泄漏（goroutine / 连接 / 文件句柄）——逐一排除。

**E. 合规 — 全部合规**
- §7 硬约束逐条对照（语言 / 协议 / 前端 / 存储 / 精度 / 并发 / 规模）。
- §10 Before Commit 机械检查全过。
- §8 豁免项之外的违规一律不达标。

**F. 设计与文档**
- 改动影响架构 / 数据流 / 接口时，同步更新 `code-map.md` 及相关 `docs/`。
- 设计决策有理由（必要时记入 `decisions.md`）。
- 文档干净：无过时描述、无冗余、与代码一致。

> **交付门槛**：A–F 全部达标。这一节是把"最优解 / 第一性 / 洁净 / 无 bug / 合规"从口号变成可核验清单——不达标不交付，没有例外。

---

## 4. 收工前必做（无损接手）

1. 通过 §3 自我审计（A–F 全达标）。
2. 更新 `docs/handoff/STATE.md`：当前进度 / 阻塞 / 下一步 / 未决决策。
3. 重要架构决策追加到 `docs/handoff/decisions.md`（ADR-lite：背景 / 决策 / 理由）。
4. 通过 §10 Before Commit。

> 这是"另一个 AI 能无缝接手"的保证。**收工不写 STATE.md = 失职。**

---

## 5. 项目概述

Go + mtapi.io (gRPC) 的跨平台跨经纪商套利系统。核心能力：**长期监控** 15 个 MT4/MT5 经纪商，在多个平台之间发现并执行套利机会。

两个二进制，多语言架构（Go core + C# desk，gRPC+protobuf 桥接，见 D-005）：
- **core** — Go 守护进程，管理 15 个 MT4/MT5 连接，策略引擎，执行管线。
- **arb-cockpit**（desk 客户端）— .NET 8 WPF 桌面应用（C#，5 个视图），通过 gRPC（grpc-dotnet）连 core。"cockpit"= 驾驶舱（监控仪表 + 操作决策），贴合其「持续监控面板 + 决策工作台」定位。

**套利类型与时间跨度**：

| 策略 | 持仓时间 | 核心逻辑 |
|------|----------|----------|
| 三角套利 | 毫秒~秒 | 同一 broker 内三个货币对的交叉汇率偏差 |
| 跨所价差套利 | 秒~分钟 | 不同 broker 间同一品种的 Bid/Ask 价差 |
| 统计套利 | 小时~天 | 历史相关性偏离均值，等待回归 |
| 期现套利 | 天~周 | 现货 vs 期货的价格收敛 |

**关键差异**：这不是纯高频系统。统计套利和期现套利需要**长期持仓**——策略引擎必须持续监控未平仓头寸、追踪 swap 累积、在收敛/发散时决策加仓或止损。Desk 的持仓 Tab 是运营人员的**持续监控面板**。未来接入 Binance。

---

## 6. 架构

```
core (Go daemon)               desk (.NET 8 WPF, C#)
  │ gRPC DashboardService        │ grpc-dotnet client
  │ OpportunityStream ─────────→ stream.Recv() async → INotifyPropertyChanged → WPF UI
  │ SpreadMatrix stream ───────→ stream.Recv() async → ObservableCollection → WPF UI
  │ PositionWatch stream ──────→ stream.Recv() async → ObservableCollection → WPF UI
  │ SubmitOrder unary ←──────── client.SubmitOrder() ← WPF 命令（ICommand）
  │ ConfirmOpportunity unary ←── client.ConfirmOpportunity() ← WPF 命令
  │ ClosePosition unary ←─────── client.ClosePosition() ← WPF 命令

core:
  MT5Adapter × 15 → QuoteBus → Detector → Evaluator → ExecutionPipeline
                                    │                   │
                                    ▼                   ▼
                              DashboardService    PostgreSQL
```

详细依赖图、数据流、goroutine/channel 拓扑见 `docs/code-map.md`。

---

## 7. 硬约束（违反不得合并）

> 完整 13 章约束见 `docs/constraints.md`。以下为摘要。

### 语言
- ✅ Go 是 core 后端唯一语言；✅ C#（.NET 8 WPF）是 desk 客户端语言——多语言架构，各层最优（见 D-005）。
- ❌ 禁止 TypeScript；❌ 禁止 Python；❌ desk 前端不再用 JS/Svelte（已改 C#）。

### 协议
- ✅ gRPC + Protobuf 是唯一**网络**通信协议。
- ❌ 禁止 REST（`encoding/json`、`gin`、`echo`、`mux`）；❌ WebSocket；❌ gRPC-Web；❌ 在 desk 内启动 HTTP server / listener。

### 客户端（desk）
- ✅ .NET 8 WPF + C#（Windows 桌面，实时数据绑定，grpc-dotnet 连 core）。
- ✅ 多语言：Go(core) + C#(desk)，gRPC+protobuf 桥接。
- ❌ 浏览器/Electron/Web；❌ Wails/Fyne（Go 桌面非原生，已废）；❌ React/Vue/JS 前端；❌ TUI。移动端：第一版不做，后期独立。

### 存储
- ✅ PostgreSQL 15+。
- ❌ Redis；❌ SQLite。

### 精度
- Hot Path = float64（仅乘除比较）；Warm/Cold = `shopspring/decimal`。
- ❌ `float32`；❌ `decimal.NewFromFloat()` 直接调用。

### 并发
- ❌ 禁止 goroutine pool（`ants`/`conc`）；❌ `sync.Map`；❌ `sync.Mutex` 在热路径；❌ 裸无界 goroutine。

### 代码规模
- Go 软参考 300 行/文件、50 行/函数；**硬红线 450 行/文件**。CI：`./scripts/check-lines.sh`（🔴 阻断；>450 失败、>300 警告，豁免 `proto/`+`docs/`+`*_test.go`）。

---

## 8. 审计豁免项

以下经架构评审确认豁免，未来审计无需再次提出：

| # | 项目 | 豁免原因 |
|---|------|---------|
| 11 | `broker_accounts.password` 明文存储 | 运行时必须将原始密码透传给 mtapi.io 进行经纪商认证；任何加密（除启动时解密）都不增加安全边界，因为 key 必须在同一进程内。存储端按项目惯例管理，运行时内存中短暂持有。 |

---

## 9. 关键依赖

core（Go）：

| 包 | 用途 |
|----|------|
| `google.golang.org/grpc` | 唯一网络通信 |
| `google.golang.org/protobuf` | 序列化 |
| `github.com/jackc/pgx/v5` | PostgreSQL |
| `shopspring/decimal` | Warm/Cold Path 精度 |
| `log/slog` | 结构化日志 |

desk（C# / NuGet，D-005）：

| 包 | 用途 |
|----|------|
| `Grpc.Net.Client` / `Grpc.Tools` / `Google.Protobuf` | grpc-dotnet 连 core |
| .NET 8 WPF SDK | Windows 桌面壳 |
| `LiveChartsCore` / `OxyPlot` / `ScottPlot`（择一） | 图表 |

---

## 10. Before Commit

```bash
go build ./...                              # 编译必须通过
go test -race -count=1 ./...                # 全量 race 检测
go vet ./...                                # 静态分析
./scripts/check-lines.sh                    # 文件规模检查（🔴 阻断）
govulncheck ./...                           # 已知漏洞
```

---

## 11. 文档索引

| 文档 | 用途 |
|------|------|
| `AGENTS.md`（本文件） | AI 协作契约 + 约束 SSOT，所有 agent 入口 |
| `docs/handoff/STATE.md` | 当前工作状态（开工读 / 收工写） |
| `docs/handoff/decisions.md` | 跨会话架构决策记录 |
| `docs/code-map.md` | 包依赖图 + 数据流 + goroutine 拓扑 + 文件清单 |
| `docs/constraints.md` | 13 章硬约束全文 |
| `docs/implementation.md` | 18 节实现规范 + 代码骨架 |
| `docs/testing.md` | 测试规范 |
| `docs/development.md` | 环境搭建 + 8 Phase 实施顺序 |
| `docs/operations.md` | 运维操作手册 |
| `docs/pitfalls.md` | 已发现陷阱 + 根因 + 修复（持续追加） |
| `proto/` | mtapi + config + dashboard proto 定义 |
| `migrations/` | PostgreSQL DDL |

> ⚠️ 原参考项目 `docs/ant/` 已**移出仓库**（sibling `../arb-ant-ref`，仅外部参考，D-009）——**禁止依赖、不进本仓**。
