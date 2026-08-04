# ARB — 跨平台跨经纪商套利系统

## 项目概述

Go + mtapi.io (gRPC) 的跨平台跨经纪商套利系统。核心能力：**长期监控** 15 个 MT4/MT5 经纪商，在多个平台之间发现并执行套利机会。

一个代码库，两个二进制：
- **core** — 守护进程，管理 15 个 MT4/MT5 连接，策略引擎，执行管线
- **desk** — Wails v3 桌面应用（Go 后端 + Svelte 前端，5 个 Tab），通过 gRPC 连接 core

**套利类型与时间跨度**：

| 策略 | 持仓时间 | 核心逻辑 |
|------|----------|----------|
| 三角套利 | 毫秒~秒 | 同一 broker 内三个货币对的交叉汇率偏差 |
| 跨所价差套利 | 秒~分钟 | 不同 broker 间同一品种的 Bid/Ask 价差 |
| 统计套利 | 小时~天 | 历史相关性偏离均值，等待回归 |
| 期现套利 | 天~周 | 现货 vs 期货的价格收敛 |

**关键差异**：这不是纯高频系统。统计套利和期现套利需要**长期持仓**——策略引擎必须持续监控未平仓头寸、追踪 swap 累积、在收敛/发散时决策加仓或止损。Desk 的持仓 Tab 不是"看一眼就关"，而是运营人员的**持续监控面板**。

未来接入 Binance。

## 架构

```
core (daemon)                  desk (Wails v3)
  │ gRPC DashboardService        │
  │ SpreadMatrix stream ────────→ Go 后端 → Wails Events → Svelte 前端
  │ PositionWatch stream ──────→ Go 后端 → Wails Events → Svelte 前端
  │ SubmitOrder unary ←───────── Go 后端 ← Wails Call ← Svelte 前端
  │ ClosePosition unary ←─────── Go 后端 ← Wails Call ← Svelte 前端
  
core:
  MT5Adapter × 15 → QuoteBus → Strategy Engine → ExecutionPipeline
                                    │                   │
                                    ▼                   ▼
                              DashboardService    PostgreSQL
```

## 硬约束（违反不得合并）

### 语言
- ✅ Go 是后端唯一语言
- ✅ Svelte 5 (JavaScript) 是前端唯一语言
- ❌ 禁止 TypeScript
- ❌ 禁止 Python

### 协议
- ✅ gRPC + Protobuf 是唯一**网络**通信协议
- ❌ 禁止 REST（`encoding/json`、`gin`、`echo`、`mux`）
- ❌ 禁止 WebSocket
- ❌ 禁止 gRPC-Web（不需要，desk 内部是 Wails IPC 桥接，非网络协议）
- ❌ 禁止在 desk 内启动 HTTP server / listener

### 前端
- ✅ Wails v3 (Go + WebView2) — 桌面壳
- ✅ Svelte 5 + HTML/CSS — 前端渲染
- ✅ Wails IPC — 前端↔Go 通信（进程内桥接，不走网络栈）
- ❌ 禁止浏览器/Electron（捆绑 Chromium，臃肿）
- ❌ 禁止 React/Vue（运行时框架，virtual DOM 对实时数据不友好）
- ❌ 禁止 TUI

### 存储
- ✅ PostgreSQL 15+
- ❌ 禁止 Redis（gRPC stream = pub/sub；内存 = 缓存）
- ❌ 禁止 SQLite

### 精度
- Hot Path = float64（仅乘除比较）；Warm/Cold = `shopspring/decimal`
- ❌ `float32`；❌ `decimal.NewFromFloat()` 直接调用

### 并发
- ❌ 禁止 goroutine pool（`ants`/`conc`）；❌ `sync.Map`；❌ `sync.Mutex` 在热路径

## 关键依赖

| 包 | 用途 |
|----|------|
| `google.golang.org/grpc` | 唯一网络通信 |
| `google.golang.org/protobuf` | 序列化 |
| `github.com/wailsapp/wails/v3` | 桌面壳（Go + WebView2） |
| `github.com/jackc/pgx/v5` | PostgreSQL |
| `shopspring/decimal` | Warm/Cold Path |
| `log/slog` | 结构化日志 |

## 文档索引

| 文档 | 用途 | 谁读 |
|------|------|------|
| `CLAUDE.md` | 本文件，AI agent 入口 | 所有 agent |
| `docs/code-map.md` | 包依赖图 + 数据流 + goroutine 拓扑 + 文件清单 | 施工 agent（开工前必读） |
| `docs/evaluation-framework.md` | 73 项设计决策，全部 P0 定稿 | 架构师 + 施工 agent |
| `docs/constraints.md` | 9 章硬约束，违反不得合并 | 施工 agent |
| `docs/implementation.md` | 18 节实现规范 + 完整代码骨架 | 施工 agent |
| `docs/testing.md` | 测试规范，精确到每个 test case | 施工 agent |
| `docs/development.md` | 环境搭建 + 8 Phase 实施顺序 + 禁止事项 | 施工 agent |
| `docs/operations.md` | 运维操作手册：如何添加/移除 broker/品种/策略 | 施工 agent + 用户 |
| `docs/pitfalls.md` | 已发现陷阱 + 根因 + 修复，持续追加 | 施工 agent |
| `docs/api/` | mtapi proto 定义 | 施工 agent (adapter 实现) |
| `proto/config/config.proto` | 配置 schema (protobuf text format) | 施工 agent |
| `proto/dashboard/dashboard.proto` | DashboardService 完整定义 | 施工 agent (core+desk) |
| `migrations/001_init.sql` | PostgreSQL 初始 DDL | 施工 agent |
| `buf.yaml` + `buf.gen.yaml` | Proto 代码生成配置 | 施工 agent |

## 角色

Claude 是总架构师 + 安全审计负责人 + 第一责任人。
所有设计决策由 Claude 做出。施工 agent 遇设计疑问，回找 Claude，不自行变更。

## Root-Cause-First — 禁止"重写一个"

当功能消失、行为退化、或出现 bug 时——**禁止不查历史直接重写**。

1. **先查 git log** — `git log --all --oneline -- <path>` 找到相关文件的所有历史，定位最后正常工作的 commit
2. **再用 git blame** — 确认当前问题是谁、哪个 commit 引入的
3. **理解原始设计意图** — 读原 commit message、关联 spec。当初为什么这样写？
4. **判断是丢失还是故意移除** — bug → 精准修那个变更；有意移除 → 先讨论是否恢复
5. **只在确认"从未实现过"后才新写**

**禁止**：
- ❌ 看到功能消失，第一反应"重写一个"
- ❌ 不读 git log 就开始写新代码
- ❌ 把以前的最优解替换成"我觉得更好的"新实现（但原实现确实违反设计约束时除外——此时已经理解了原设计，可以推翻）
