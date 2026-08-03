---
name: debugging-symptom-to-root
description: |
  全栈根因调试方法论。从 UI 症状出发，沿数据管线逐层追踪：前端 filter/map → API/SSE 响应
  → 后端 handler → gateway/RPC → DB 查询 → 数据源。覆盖两条核心管线：
  (1) 曲线图（Equity/Balance/Profit）— 跨边界日期/Decimal/Enum 格式问题；
  (2) 持仓/订单列表 — 实时 SSE 流断连、会话过期、状态不一致。
  Triggers on: "equity图表", "balance曲线", "profit曲线", "净値曲线", "资金曲线",
  "收益曲线", "暂无净値曲线数据", "No equity curve data", "曲线为空", "图表不显示",
  "chart empty", "图表没有数据", "EquityPoint", "AccountAnalytics", "AccountDetail",
  "曲线图不显示", "曲线没有数据点", "持仓为0", "订单为空", "持仓列表空", "positions empty",
  "position list empty", "持仓没有", "positionSnapshot", "position_snapshot", "OpenedOrders"
---

# Equity/Balance/Profit 曲线全栈根因调试

三条金融曲线（Equity、Balance、Profit）横跨 DB → Go → Proto → JSON → React → Recharts 六层边界，
任一层的格式偏差都可能导致图表完全空白。本方法论专治此类跨边界数据格式问题。

## The 6-Layer Pipeline

```
ClickHouse/PostgreSQL  (DATE / TIMESTAMP / DOUBLE PRECISION)
  → Go time.Time / Format("layout") / JSONB unmarshal
    → Proto message (Timestamp / string / double) 
      → ConnectRPC JSON serialization
        → TypeScript type → useMemo filter/sort → Recharts <AreaChart>
```

## The 5-Step Trace

### Step 1: 确认曲线类型和空状态来源

三种曲线对应三个 dataKey，先确认是哪个：

| 图表 | dataKey | i18n empty key |
|------|---------|---------------|
| Equity | `equity` | `accounts.analytics.empty.equityCurve` |
| Balance | `balance` | — 同区域切换 Segmented |
| Profit | `profit` | — 同区域切换 Segmented |

读取组件的 render gate：
```tsx
// AccountAnalyticsSection.tsx 或 AccountDetail.tsx
{chartData.length > 0 ? (
  <AreaChart><Area dataKey="equity" /></AreaChart>
) : (
  <Empty text={t('accounts.analytics.empty.equityCurve')} />
)}
```

### Step 2: 沿管线逐层追踪数据流

Equity/Balance/Profit 曲线数据流（AccountDetail 为例）：

```
AccountAnalyticsSection (render gate)
  ← equityChartData: {date, equity, balance, profit}[]  (useMemo filter)
    ← analytics?.equityCurve: EquityPoint[]  (API response)
      ← analyticsApi.getAccountAnalytics()  (ConnectRPC client)
        ← AnalyticsServer.GetAccountAnalytics  (backend handler)
          ← AnalyticsRepository.GetEquityCurve  (DB query)
            ← trade_records / ClickHouse (source)
```

对每一层问三个问题：
1. 数据在这一层长什么样？（打印或检查字段类型）
2. 什么条件会触发空/错误状态？
3. 上一层的输出是否满足这一层的输入预期？

### Step 3: 定位曲线 filter 的"全杀"条件

AccountDetail 中 equity/balance/profit 三条曲线共享同一个时间过滤器：

```tsx
// AccountDetail.tsx useMemo
const filteredEquityCurve = (() => {
  if (chartPeriod === 'all' || equityCurve.length === 0) return equityCurve;
  const cutoff = new Date();  // ← 当前时间
  if (chartPeriod === 'month') cutoff.setDate(cutoff.getDate() - 30);
  // ...
  return equityCurve.filter((p) => new Date(p.date) >= cutoff);
  //                                ↑ 如果 p.date 格式导致 Invalid Date
  //                                  Invalid Date 的所有比较都返回 false
  //                                  全部点被过滤 → 空数组 → "暂无净値曲线数据"
})();
```

**关键认知：** 时间过滤器是曲线图最脆弱的环节。只要后端返回的 `date` 字段不是 ISO 8601
（`YYYY-MM-DD` 或 RFC 3339），V8 的 `new Date()` 就会返回 `Invalid Date`，导致整条曲线消失。

### Step 4: 跨边界格式不匹配 —— 曲线最常见根因

六层边界中，每层都可能引入格式偏差导致曲线空白：

| 层 | 边界 | 曲线常见问题 |
|----|------|-------------|
| DB | PG/ClickHouse → Go | `DATE` 类型 scan 到 `time.Time` 后 Format 选错 layout |
| Go | Go → Proto | `time.Time.Format("01/02")` 无年份 → 前端无法解析 |
| Proto | Proto → JSON | `google.protobuf.Timestamp` 序列化为 RFC 3339 vs 自定义 string |
| JSON | JSON → JS | `new Date("03/07")` → V8 Invalid Date；`new Date("2026-05-28")` ✅ |
| JS | JS → Recharts | filter 把 Invalid Date 全部判 false → data 空数组 |
| Recharts | Recharts → 渲染 | 空数组 → Area 不绘制 → 空白区域 + Empty 文案 |

**日期格式兼容性速查：**

| 格式 | `new Date()` 结果 |
|------|------------------|
| `"2026-05-28"` (ISO 8601) | ✅ 所有引擎正确 |
| `"2026-05-28T10:30:00Z"` (RFC 3339) | ✅ 所有引擎正确 |
| `"03/07"` (MM/DD，无年份) | ❌ V8/Chromium Invalid Date |
| `"03/07/2026"` (MM/DD/YYYY) | ⚠️ 引擎差异，不可靠 |
| `"01/02"` (Go 的 `Format("01/02")`) | ❌ V8 Invalid Date |

**凡曲线 filter 中涉及 `new Date(p.date) >= cutoff`，第一个检查点就是抓取前端收到的原始 date 值。**

### Step 5: 验证修复

修复后验证清单：
1. Backend `EquityPoint.Date` 是 ISO 8601（`time.Time.Format("2006-01-02")`）
2. 浏览器 console 中 `new Date(point.date)` 返回有效 Date
3. 三条曲线（equity / balance / profit）在 day/week/month/all 四个 period 下都有数据点
4. 月初/月末/跨年边界条件下 filter 不丢数据

## 快速诊断 checklist

遇到曲线图为空但 DB 有交易记录时：

```
□ 1. 确认是三条曲线都空还是仅某一条空
□ 2. 读 AccountDetail useMemo 中的 filter 条件，确认是哪个变量为空
□ 3. Network tab 查找 GetAccountAnalytics 响应，检查 equityCurve[0].date 格式
□ 4. Console: new Date("响应中的date值") 是否返回 Invalid Date
□ 5. 后端: 读 analytics_repository_equity.go 的 Format("layout") 
□ 6. 后端: 确认用的是 Format("2006-01-02") 而非 Format("01/02")
□ 7. 如果换了 chartPeriod 后有数据，说明是某段时间窗口的日期格式问题
□ 8. 检查 proto 定义中 EquityPoint.date 是 string（自定义格式）还是 Timestamp
```

## 经典案例：MM/DD 无年份 → 曲线全空

```
根因链:
  Go: dd.Date.Format("01/02")  → 输出 "03/07"（无年份）
  → Proto JSON: {"date": "03/07", "equity": 10500.0}
    → JS: new Date("03/07")  → Invalid Date (V8)
      → useMemo filter: p.date >= cutoff  → false (InvalidDate < anyDate)
        → filteredEquityCurve: []
          → {filteredEquityCurve.length > 0 ?} → false
            → <Empty text="暂无净値曲线数据" />

修复:
  Go: dd.Date.Format("2006-01-02")  → 输出 "2026-03-07" ✅
```

## 反模式

❌ 不要在前端加 `try/catch` 或 `|| []` fallback 来掩盖数据格式问题
❌ 不要在 filter 中加 `isNaN(new Date(x).getTime())` 来跳过无效日期
✅ 应该在数据源头（backend handler）返回正确的格式
✅ 前端防御性代码只能作为临时 workaround，必须标注 TODO 指向真正的根因

---

# 持仓/订单列表空 → SSE 流断连类根因调试

持仓列表为空与曲线图不同：曲线是历史数据的 RPC 拉取，**持仓是实时 SSE 流推送**。
空列表不一定是"真的没有持仓"，更可能是推送链断了。

## The SSE Position Pipeline (8 Layers)

```
MT 经纪商服务器 (raw positions)
  → mtapi.io gRPC proxy (OnOrderUpdate 双向流)
    → Gateway.adapter/mt5/quotes.go (orderUpdateRecvLoop)
      → main.go OnOrderUpdate callback (构建 PositionSnapshot → Publish)
        → mthub.PositionSnapshotBroker (内存 pub/sub, per accountID)
          → stream_handler.go SubscribeEvents (SSE event loop)
            → 前端 stream.ts (ConnectRPC SSE client)
              → ConnectProvider.tsx onPositionSnapshot (map → setPositions)
                → tradingStore.ts positionsMap (Zustand)
                  → AccountDetail.tsx positions → useMemo → realPositions/pendingOrders
```

## 关键区别：两条路径，两条都可能断

| 路径 | 触发时机 | 如果失败 |
|------|---------|---------|
| **Initial Snapshot** | SSE 连接建立时 `sendInitialSnapshot()` | `OpenedOrders` RPC → hub → gateway → mtapi |
| **Live Push** | 每次 `OnOrderUpdate` 从 broker 推送 | `PositionSnapshotBroker.Publish` → SSE event loop |

**只要 Initial Snapshot 成功，前端至少有一次全量数据。** Live Push 断了只会让数据不更新，不会导致空列表。
**两条都断 → 前端看到空列表。**

## 快速诊断 checklist

遇到持仓/订单列表为空但余额卡片有数据时：

```
□ 1. 后端日志搜 "sendInitialSnapshot: OpenedOrders failed"
     → 有 → 这是 Initial Snapshot 路径断了，继续步骤 2
     → 无 → Initial Snapshot 成功，问题可能在 Live Push 或前端，跳到步骤 6
□ 2. 看错误类型:
     "mthub: session not found" → Hub 里没有该账户的 Gateway 会话
     "context canceled" → SSE 客户端断连导致 RPC 被取消（用户刷新页面）
     "Client with id = mdgw-* not found" → mtapi proxy 会话过期
□ 3. 检查 DB account_status:
     SELECT id, login, account_status FROM mt_accounts WHERE id = '<accountID>';
     connected 但步骤 2 报 session not found → DB 状态不一致（Bug #1）
□ 4. 搜 "mdgateway: dead account" 日志
     → 有 → Gateway 健康监控检测到 >15 分钟无 tick
     → OnAccountDisconnect 是否更新了 DB status（修后应该有 WARN 日志）
□ 5. 搜 "mdgateway: gateway active" 确认 Gateway 是否已重连
     → 已 active → 刷新前端页面，SSE 重新连接后 sendInitialSnapshot 应该成功
     → 未 active → 检查 mtapi 连接是否可达、经纪商是否在线
□ 6. 前端 Network tab 找 subscribeEvents 的 SSE 响应
     检查是否有 type: "position_snapshot" 事件及其 positions 数组
□ 7. Console: useTradingStore.getState().positions 检查 Zustand 状态
```

## 常见根因链

### 根因 A: 周末 broker 会话过期 → DB 状态 stuck（已修复）

```
周末经纪商维护/空闲超时
  → mtapi proxy 清理 session ("Client not found")
    → orderUpdateRecvLoop ensureConnected 重连 backoff 到 5min 上限
      → healthMonitor 检测 >15min 无 tick → dead account
        → OnAccountDisconnect 调用但【不更新 DB account_status】(Bug #1, 已修)
          → DB 仍显示 connected
            → SSE sendInitialSnapshot 看到 connected → 调 OpenedOrders
              → hub.Get 返回 nil → ErrSessionNotFound
                → 静默 continue，不发 position_snapshot (Bug #2, 已修)
                  → 前端 sees 空列表
```

修复（a61fc1e）:
1. `OnAccountDisconnect` 现在会 `UPDATE mt_accounts SET account_status='disconnected'`
2. `sendInitialSnapshot` `OpenedOrders` 失败时会打 WARN 日志
3. `OnBrokerInfo` goroutine 同样加错误日志

### 根因 B: 启动竞态 — SSE 请求比 Gateway 就绪先到

```
backend 重启
  → loadAccountConfigs 串行启动 Gateway (MT5 ~4s, MT4 ~6s)
  → SSE SubscribeEvents 同时到达 → sendInitialSnapshot
  → "session not found" → position_snapshot 跳过
  → 10s 后 Gateway 就绪，但 SSE 已过了 initial snapshot 阶段
  → OnBrokerInfo 发布的 snapshot 可能被 SSE 循环消费（Subscribe 在 sendInitialSnapshot 之后）
```

**现状：OnBrokerInfo → snapshotBroker.Publish → SSE event loop 可以补偿。**
但如果 SSE Subscribe 太慢（在 OnBrokerInfo goroutine 之后），snapshot 会因 broker 不 replay 而丢失。

### 根因 C: 用户刷新页面打断 initial snapshot

```
用户打开页面 → SSE 连接 → sendInitialSnapshot 开始
  → OpenedOrders RPC (5s timeout, 绑 SSE ctx)
    → 用户刷新页面 → SSE ctx 取消
      → RPC 被取消: "context canceled"
        → 新的 SSE 连接 → 又重新开始...
```

**5s 超时在周末可能不够**（经纪商服务器响应慢），可考虑增加到 10s。

## 涉及的关键文件

| 文件 | 关键行 | 角色 |
|------|--------|------|
| `stream_handler.go` | 139-162 | Initial position snapshot |
| `stream_handler.go` | 248-324 | Live position_snapshot from broker |
| `main.go` | 351-455 | OnOrderUpdate → snapshotBroker.Publish |
| `main.go` | 457-468 | OnAccountDisconnect → DB update |
| `main.go` | 469-498 | OnBrokerInfo → initial snapshot goroutine |
| `runner.go` | 178-238 | healthMonitor (30s tick → dead detection) |
| `runner.go` | 400-460 | NATS account event subscriber (reconnect) |
| `mt5/connection.go` | 19-34 | Gateway struct (per-account gRPC conn) |
| `mt5/quotes.go` | 263-404 | orderUpdateRecvLoop (auto-reconnect stream) |
| `mt5/orders.go` | 157-215 | FetchOpenedOrders (RPC to mtapi) |
| `mthub/types.go` | 18-23 | Hub (session/executor registry) |
| `mthub/types.go` | 201-239 | PositionSnapshotBroker (pub/sub) |
| `mthub/service.go` | 280-284 | MtHubService.OpenedOrders |
