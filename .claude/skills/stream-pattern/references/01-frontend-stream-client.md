# 01 - 前端流客户端

## 文件: `frontend/src/client/stream.ts`

### 共享订阅模式

核心数据结构:
```ts
type SharedStreamState<T> = {
  abortController: AbortController;
  listeners: Map<string, Listener<T>>;
  started: boolean;
};

const sharedProfitStreams = new Map<string, SharedStreamState<ProfitUpdate>>();
const sharedOrderStreams = new Map<string, SharedStreamState<OrderUpdate>>();
```

`subscribeShared()` 实现订阅去重:
- 同一 `accountId` 的多次调用共享一个底层 SSE 连接
- 每个 listener 有唯一 ID，支持独立取消
- 最后一个 listener 取消时中止 AbortController 清理资源

```ts
function subscribeShared<T>(store, key, start, listener) {
  let state = store.get(key);
  if (!state) {
    state = { abortController: new AbortController(), listeners: new Map(), started: false };
    store.set(key, state);
  }
  const id = Math.random().toString(36).slice(2);
  state.listeners.set(id, listener);
  startSharedStream(state, start, key, store);
  return () => {
    // 取消订阅: 删除 listener, 若为最后一个则 abort + delete
  };
}
```

### 三种订阅 API

| API | RPC | 用途 |
|-----|-----|------|
| `subscribeEvents(accountIds, callbacks)` | `SubscribeEvents` | 主事件流: order/profit/status/positionSnapshot |
| `subscribeProfitUpdates(accountId, cb)` | `SubscribeProfitUpdates` | 单账户 profit 流 (共享订阅) |
| `subscribeUserSummary(cb)` | `SubscribeUserSummary` | 用户级聚合摘要 |

### 事件路由 (subscribeEvents)

```ts
switch (e.payload.case) {
  case 'orderUpdate':   → callbacks.onOrder?.(toCamelCase(e.payload.value))
  case 'profitUpdate':  → callbacks.onProfit?.(toCamelCase(e.payload.value))
  case 'accountStatus': → callbacks.onStatus?.(toCamelCase(e.payload.value))
  case 'positionSnapshot': → 解析 positions 数组 → callbacks.onPositionSnapshot?.(accountId, orders)
}
```

### 重连逻辑

指数退避: `delay = min(1000 * 2^retryCount, 30000)` ms
传输失败上限: 连续 12 次 transport failure 后停止重连 (避免浏览器报错刷屏)
非 AbortError 才重连

## 关键踩坑

### @connectrpc/connect v2: Proto Timestamp 反序列化

`@connectrpc/connect` **v2.x** 将 `google.protobuf.Timestamp` 反序列化为 `{seconds: bigint, nanos: number}` 对象，**不是** ISO 字符串。

| connect 版本 | Timestamp JSON 表示 |
|-------------|-------------------|
| v1.x | `"2026-05-29T10:30:00Z"` (字符串) |
| **v2.x** | `{seconds: 1779000000n, nanos: 0}` (对象) |

**影响**：`fromProtoOrders()` 用 `...o` 透传 Timestamp 对象到表格，`String(v)` 输出 `[object Object]`，用户看到乱码/"缺失"。

**修复**：在 `fromProtoOrders` 中将 Timestamp 对象转为 unix 秒（number）：
```ts
const toUnixSeconds = (ts: unknown): number => {
  if (ts == null) return 0;
  if (typeof ts === 'number') return ts;
  const t = ts as Record<string, unknown>;
  if (t.seconds != null) {
    const secs = typeof t.seconds === 'bigint' ? Number(t.seconds) : Number(t.seconds);
    return secs + Number(t.nanos || 0) / 1_000_000_000;
  }
  return 0;
};
```

### RPC 路径 vs SSE 流路径数据格式不一致

| 数据路径 | `openTime` 格式 | 来源 |
|---------|----------------|------|
| RPC (`OpenedOrders`) | Proto Timestamp → `{seconds, nanos}` 对象 | `toProtoOrders()` → ConnectRPC JSON |
| SSE `orderUpdate` | number (unix 秒) | `orderRecordToUpdateEvent()` → `rec.OpenTime.Unix()` |
| SSE `positionSnapshot` | number (unix 秒) | `PositionSnapshotItem.OpenTime` |

三种格式混入 Zustand store，消费端必须用 `typeof` 分支处理。推荐在 `fromProtoOrders` 统一转为 unix 秒，在渲染层用 `formatTimestamp()` 统一格式化。

### formatTimestamp 零值陷阱

```ts
// ❌ 错误：!0 === true，吞掉有效的 0 值
if (!ts) return '';

// ✅ 正确：显式检查 null/undefined/空字符串，number 单独处理
if (ts == null || ts === '') return '';
if (typeof ts === 'number' && ts <= 0) return '';
```

### 表格抖动三件套

1. `columns` 用 `useMemo(() => [...], [t])` 稳定引用
2. `Table` 设置 `tableLayout="fixed"` + `scroll={{ x: 总宽度 }}`
3. CSS 兜底：`.ant-table-container > table { table-layout: fixed !important; }`
