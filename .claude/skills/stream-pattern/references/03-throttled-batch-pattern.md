# 03 - 节流批量模式

## 文件: `frontend/src/providers/ConnectProvider.tsx`

### 核心状态

```ts
const profitUpdateTimeoutRef = useRef<number | null>(null);
const profitLastFlushAtRef = useRef<number>(0);
const pendingProfitUpdates = useRef<Map<string, ProfitUpdate>>(new Map());
```

### 节流算法 (THROTTLE, 非 DEBOUNCE)

```ts
const THROTTLE_MS = 300;
const now = Date.now();
const elapsed = now - profitLastFlushAtRef.current;

if (profitUpdateTimeoutRef.current) {
  // 已有排程的 flush, 不重置计时器 (这是节流, 不是防抖)
  return;
}

const delay = elapsed >= THROTTLE_MS ? 0 : THROTTLE_MS - elapsed;

profitUpdateTimeoutRef.current = window.setTimeout(() => {
  profitLastFlushAtRef.current = Date.now();
  profitUpdateTimeoutRef.current = null;

  // 批量 flush 所有待处理账户
  const updates = pendingProfitUpdates.current;
  for (const [accId, profitData] of updates.entries()) {
    tradingStore.setAccountInfoById(accId, { balance, equity, profit, ... });
    accountStore.patchAccountFinancials(accId, patch);
    tradingStore.touchStreamProfitAt(accId);

    // 专利持仓: 只更新已存在的行, 不新增
    for (const o of profitData.orders) {
      const old = existingRows.find(p => p.ticket === ticket);
      if (!old) continue;  // 跳过不存在的持仓
      tradingStore.updatePosition(accId, ticket, posPatch);
    }
  }

  pendingProfitUpdates.current.clear();
}, delay);
```

### 为什么是节流不是防抖

之前的实现用防抖 (每次事件重置计时器):
- 问题: 2+ 账户同时推送 profit 事件, 频率超过计时器时长
- 结果: `clearTimeout` 永远在重置, flush 永不触发
- Account List 数据不更新

改为节流后:
- 新事件不重置计时器
- 每个 300ms 窗口最多触发一次 flush
- 所有在窗口内到达的事件批量处理

### Zustand Store 更新

`tradingStore.setAccountInfoById()` (`frontend/src/stores/tradingStore.ts:202`):
```ts
setAccountInfoById: (accountId, info) => set((state) => {
  const existingInfo = state.accountInfoMap.get(accountId);
  const newInfo = { ...(existingInfo || defaultAccountInfo), ...info };
  const newMap = new Map(state.accountInfoMap);
  newMap.set(accountId, newInfo);

  // 若当前查看的账户就是被更新的账户, 同步更新 accountInfo
  if (state.currentAccountId === accountId) {
    return { accountInfo: newInfo, accountInfoMap: newMap, ... };
  }
  return { accountInfoMap: newMap, ... };
}),
```

### 消费者侧

AccountDetail 页 (`frontend/src/pages/accounts/AccountDetail.tsx`):
```ts
// 从 Zustand store 读取, profit 更新时自动触发 re-render
const accountInfo = useTradingStore(state =>
  state.accountInfoMap.get(accountId)
);
```

### 性能特性

- **O(1) 查找**: `accountInfoMap` 是 `Map<string, AccountInfo>`
- **批量更新**: 多个账户在同一个 flush 中更新, 减少 React re-render 次数
- **非阻塞 pub**: 后端 broker 用 `default:` 丢弃慢消费者的积压事件
- **缓冲 channel**: broker channel 容量 8, 短时突发可容忍
