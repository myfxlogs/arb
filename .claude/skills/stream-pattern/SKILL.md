---
name: stream-pattern
description: >
  ConnectRPC SSE 实时流更新模式（全栈）。涵盖后端 mtapi→broker→SSE 事件管道、
  前端共享订阅去重、节流批量写入 Zustand store 的完整链路。
  当需要实现或调试实时账户数据推送（profit/balance/equity/positions）时使用此 skill。
  子文档：前端流客户端(01)、后端事件管道(02)、节流批量模式(03)。
---

# SSE 实时流更新模式

> 最后验证: 2026-05-27, 对标当前代码库 streaming/SSE 实现。

## 参考文档

| 编号 | 主题 | 文件 | 说明 |
|------|------|------|------|
| 01 | 前端流客户端 | [01-frontend-stream-client.md](references/01-frontend-stream-client.md) | 共享订阅、事件路由、重连逻辑 |
| 02 | 后端事件管道 | [02-backend-event-pipeline.md](references/02-backend-event-pipeline.md) | mtapi→runner→broker→SSE 完整链路 |
| 03 | 节流批量模式 | [03-throttled-batch-pattern.md](references/03-throttled-batch-pattern.md) | 300ms 节流、批量 flush、去重策略 |
| 04 | klinecharts 主图叠加 | [04-klinecharts-overlay-pattern.md](references/04-klinecharts-overlay-pattern.md) | BIDASK 价格线叠到主图 candle_pane 的正确写法 |

## 架构总览

```
MT4/MT5 Broker
  │ mtapi gRPC (OnOrderProfit / OnOrderUpdate)
  ▼
mdgateway adapter (mt4/quotes.go, mt5/quotes.go)
  │ SubscribeProfit(ctx, handler)
  ▼
mdgateway runner (runner.go)
  │ OnAccountProfit(accountID, userID, *ProfitUpdate)
  ▼
MtHubService (mthub/service.go)
  │ PublishAccountProfit(*AccountProfitEvent)
  ▼
AccountProfitBroker (mthub/types.go)
  │ fan-out pub/sub, buffered chan(8)
  ▼
StreamServer (connect/system/stream_handler.go)
  │ ConnectRPC server-stream (SSE over HTTP/2)
  │   ├─ SubscribeProfitUpdates (per-account)
  │   └─ SubscribeUserSummary   (aggregated, pre-computed)
  ▼
Frontend stream.ts (client/stream.ts)
  │ Shared subscription dedup
  ▼
SSEQueryBridge (bridge/SSEQueryBridge.tsx)
  │ 300ms throttle → batch flush
  ▼
TanStack Query cache (single source of truth)
  │   ├─ queryKeys.accounts.list()      — per-account live data
  │   ├─ queryKeys.userSummary.all      — pre-computed aggregates
  │   └─ queryKeys.accounts.financials  — per-account financials
  ▼
React components (Dashboard / AccountDetail)
  │ useQuery() → render only, no client-side math
  ▼
UI
```

## 核心文件

| 层 | 文件 | 关键函数/类型 |
|----|------|-------------|
| 后端-网关适配器 | `backend/internal/mdgateway/adapter/mt5/quotes.go` | `SubscribeProfit()` |
| 后端-网关适配器 | `backend/internal/mdgateway/adapter/mt4/quotes.go` | `SubscribeProfit()` |
| 后端-运行器 | `backend/internal/mdgateway/runner.go:213` | `gw.SubscribeProfit()` |
| 后端-DTO | `backend/internal/mdgateway/adapter/mdtick/mdtick.go` | `ProfitUpdate`, `ProfitHandler` |
| 后端-服务层 | `backend/internal/mthub/service.go:297` | `PublishAccountProfit()` |
| 后端-经纪人 | `backend/internal/mthub/types.go:74` | `AccountProfitBroker` |
| 后端-SSE | `backend/internal/connect/system/stream_handler.go:460` | `SubscribeProfitUpdates()` |
| 前端-流 | `frontend/src/client/stream.ts` | `streamApi`, `subscribeShared()` |
| 前端-SSE桥接 | `frontend/src/bridge/SSEQueryBridge.tsx` | SSE→TanStack Query routing |
| 前端-事件映射 | `frontend/src/bridge/bridgeStreamEvents.ts` | `handleProfitUpdate`, `flushProfitUpdates` |
| 前端-汇总映射 | `frontend/src/bridge/bridgeUserSummary.ts` | `handleUserSummary` (pre-computed aggregates) |
| 前端-状态 | `frontend/src/stores/tradingStore.ts` | `setAccountInfoById()` (per-account, for detail pages) |
| Proto | `proto/ant/v1/stream_event_account.proto` | `ProfitUpdateEvent` |

## 前端关键约定

1. **共享订阅**：同一 `accountId` 的多个调用者共享一个 SSE 连接，通过 `subscribeShared()` 的 `Map<string, SharedStreamState>` 去重。
2. **节流非防抖**：300ms 节流窗口。新事件不重置计时器，而是挂在已排程的 flush 上。防止多账户场景下防抖永远不触发。
3. **批量写入**：每次 flush 遍历 `pendingProfitUpdates` Map，一次同步批量写入所有待更新账户到 Zustand store。
4. **只补不增**：profit.orders 只更新已存在于 `positionsMap` 的持仓行，不尽信 profit 事件的 orders 列表（MT5 可能省略）。
5. **控制台静默**：profitUpdate 不再 log 到 console，仅通过 `console.debug` 在开发环境可用。

## 前端关键约定

1. **共享订阅**：同一 `accountId` 的多个调用者共享一个 SSE 连接，通过 `subscribeShared()` 的 `Map<string, SharedStreamState>` 去重。
2. **节流非防抖**：300ms 节流窗口。新事件不重置计时器，而是挂在已排程的 flush 上。防止多账户场景下防抖永远不触发。
3. **批量写入 TanStack Query**：每次 flush 遍历 `pendingProfit` Map，一次同步批量写入所有待更新账户到 TanStack Query 缓存（NOT Zustand）。
4. **唯一数据源**：SSE 数据写入 TanStack Query 后，所有组件通过 `useQuery(key)` 读取。不要双写 Zustand+TQ。
5. **前端只渲染，不做计算**：`totalEquity`、`totalProfit`、`connectedCount` 等聚合值来自后端 `SubscribeUserSummary` SSE，前端只 `useQuery(userSummary.all)` 渲染。
6. **只补不增**：profit.orders 只更新已存在于 `positionsMap` 的持仓行，不尽信 profit 事件的 orders 列表（MT5 可能省略）。
7. **控制台静默**：profitUpdate 不再 log 到 console，仅通过 `console.debug` 在开发环境可用。

## ⚠️ 账户详情净值曲线为空 (2026-05-31 修复)

**症状**：账号详情页净值/余额/收益曲线永远为空。后端 `trade_records` 和 `account_balance_history` 有数据，但 API 返回 `equityCurve: []`。

**根因**：`AnalyticsCache.Get()` 在 Redis 未命中时返回 `(nil, nil)`。Handler 只检查 `err == nil`——对 `nil` 响应也为 true——直接返回 `connect.NewResponse(nil)`（空响应），从未执行到实际 SQL 查询。

**修复**：`analytics_handler.go:64`：加 `cached != nil` 检查。cache miss 必须 fall through 到实际计算。

```go
// Before (broken):
if cached, err := s.cache.Get(ctx, req.Msg.AccountId); err == nil {
    return connect.NewResponse(cached), nil  // nil cached → empty response!
}

// After (fixed):
if cached, err := s.cache.Get(ctx, req.Msg.AccountId); err == nil && cached != nil {
    return connect.NewResponse(cached), nil
}
```

**教训**：cache-aside 模式中，cache miss 返回 `(nil, nil)` 是合法约定，但调用方必须同时检查 `err == nil && value != nil`，否则 cache miss 被当作 cache hit（空值命中）。

## ⚠️ Dashboard 数据流陷阱 (2026-05-31 修复)

**症状**：仪表盘 N 个交易账户，只有 1 个显示实时数据，其余显示 0。账户详情页全部正常。

**根因**：SSE 桥接 (`bridgeStreamEvents.ts`) 把利润数据写入 TanStack Query，但 Dashboard 从 Zustand `tradingStore.accountInfoMap` 读取。两个 store 之间没有桥接——`accountInfoMap` 永远为空。

为什么恰好显示 1 个账户有数据？因为用户点进过那个账户的详情页，`setAccountInfo`（依赖 `currentAccountId`）恰好给那个账户写了单条数据。这是偶然正确。

**错误修法**：在 SSE 桥接里同时写 TanStack Query 和 Zustand（双写反模式）。

**正确修法**：
1. `useAccount().fetchAccounts()` RPC 回调后用 `queryClient.setQueryData(accounts.list(), accounts)` 播种 TanStack Query
2. SSE 桥接只写 TanStack Query（已经做了）
3. Dashboard 用 `useQuery(accounts.list())` 读取 —— TanStack Query 是唯一数据源
4. 仪表盘卡片的 `totalEquity`/`totalProfit` 用 `useQuery(userSummary.all)` —— 后端 `SubscribeUserSummary` SSE 预计算聚合值
5. 前端不写 for 循环做 Σ sum 聚合 —— 后端 `GetUserAccountsSummary()` 负责

**数据流**：
```
RPC fetchAccounts → seeds TanStack Query
SSE ProfitUpdate   → patches TanStack Query   }  单源
Dashboard          ← useQuery(TanStack Query)   }  只读
StatCards          ← useQuery(userSummary)       }  后端聚合
```

## ⚠️ 账户删除 — TanStack Query 乐观更新模式 (2026-05-31 确立)

**最终方案**：`useMutation` + `onMutate` 乐观更新双 store + `cancelQueries` 防竞争。

```typescript
useMutation({
  mutationFn: ({ id, password }) => accountApi.delete(id, password),
  onMutate: async (vars) => {
    // 1. 取消进行中的 list 请求，防止覆盖乐观更新
    await queryClient.cancelQueries({ queryKey: queryKeys.accounts.list() });
    // 2. 快照双 store 用于 rollback
    const prevTq = queryClient.getQueryData(queryKeys.accounts.list());
    const prevZustand = useAccountStore.getState().accounts;
    // 3. 同时从两个 store 乐观删除
    queryClient.setQueryData(queryKeys.accounts.list(),
      (old) => (old ?? []).filter((a) => a.id !== vars.id));
    useAccountStore.getState().removeAccount(vars.id);
    return { prevTq, prevZustand };
  },
  onError: (_err, _vars, ctx) => {
    // 原子回滚两个 store
    if (ctx?.prevTq) queryClient.setQueryData(queryKeys.accounts.list(), ctx.prevTq);
    if (ctx?.prevZustand) useAccountStore.getState().setAccounts(ctx.prevZustand);
  },
  onSettled: () => {
    // 仅 invalidate list 与服务器同步。
    // ⚠️ 不要在 onSettled 中 removeQueries per-account 缓存！
    // removeQueries 会触发 still-mounted 的 observer 重新 fetch，
    // 导致 404/403 错误。让组件 unmount 时自然 GC。
    queryClient.invalidateQueries({ queryKey: queryKeys.accounts.list() });
  },
});
```

**组件侧**：仅 `await mutateAsync` → `navigate('/')`，不操作任何 store。

**为什么这是最优**：
1. `onMutate` 单入口乐观更新两个 store
2. `cancelQueries` 防止进行中的请求覆盖乐观更新
3. `onSettled` 只 invalidate list，不移除 per-account 缓存
4. 组件解耦 — 不接触 store，只 await + navigate

**踩过的坑（6 次迭代）**：
1. ❌ `removeQueries` 后不 `cancelQueries` → 活跃 observer 重新 fetch 404
2. ❌ `navigate` 在 mutate 之前 → 时序不可靠
3. ❌ `removeQueries` 在 `onSettled` 中 → 组件仍 mounted 触发 refetch
4. ❌ 双 store 分别处理 → rollback 不原子
5. ❌ 组件直接操作 Zustand + fire-and-forget → rollback 逻辑分散
6. ✅ `onMutate` + `cancelQueries` + 双 store 快照回滚 + `onSettled` 仅 invalidate list

## 踩坑记录

详见子文档中的"关键踩坑"章节：

| 问题 | 文档 | 影响 |
|------|------|------|
| `@connectrpc/connect` v2 Timestamp 反序列化为对象 | [01](references/01-frontend-stream-client.md) | 表格显示 `[object Object]` |
| RPC vs SSE 路径 openTime 格式不一致 | [01](references/01-frontend-stream-client.md) | 三种格式混入 store |
| `formatTimestamp` `!0` 吞掉零值 | [01](references/01-frontend-stream-client.md) | openTime 显示为空 |
| 表格抖动：columns 重建 + tableLayout | [01](references/01-frontend-stream-client.md) | 实时数据更新时列宽跳动 |
| MT5 OrderUpdateSummary 无 GetCredit | [mt-gateway 05](../mt-gateway/references/05-account.md) | MT5 credit 始终为 0 |
| MT5 Order 有 GetOpenTimestampUTC 需 fallback | [mt-gateway 05](../mt-gateway/references/05-account.md) | MT5 openTime 缺失 |
| Proto Timestamp 零值 omitempty 省略 | [mt-gateway 05](../mt-gateway/references/05-account.md) | 前端收不到字段 |
| Credit 全链路缺失 | [mt-accounts pitfalls](../mt-accounts/references/pitfalls.md) | DTO→SQL→Scan→Handler→Proto |
| Analytics cache miss 返回空响应 | 本文档 §净值曲线为空 | `(nil, nil)` 被当作 cache hit |
