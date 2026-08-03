# klinecharts 主图叠加指标模式 (BIDASK)

> 记录于 2026-06-21：经过多次试错后，恢复了原始正确写法。此文档防止以后再次出错。

## 关键约定

在 klinecharts v9.8.12 中，将自定义 indicator 叠加到主图蜡烛 pane 的方法：

```typescript
chart.createIndicator('BIDASK', true, { id: 'candle_pane' });
//                           ^^^^          ^^^^^^^^^^^^
//                           isStack=true  PaneOptions.id 必须设为主图 pane ID
```

**三个参数缺一不可**：

| 参数 | 值 | 说明 |
|------|----|------|
| `name` | `'BIDASK'` | 与 `registerIndicator` 中的 `name` 一致 |
| `isStack` | `true` | 叠加模式，不创建独立 y 轴 |
| `PaneOptions.id` | `'candle_pane'` | **关键**：klinecharts 内部常量 `PaneIdConstants.CANDLE`，指定复用已存在的蜡烛主 pane |

## ⚠️ 踩坑记录

### 错误 1：id 不匹配 → 创建独立副图 pane

```typescript
// ❌ 错误：自定义 id 'bidask_overlay' 不在 klinecharts 已知 pane 列表中
// → 创建了一个新的独立 sub-pane，B/A 跑到副图上
chart.createIndicator('BIDASK', true, { id: 'bidask_overlay' });
```

### 错误 2：创建顺序假设

```typescript
// ❌ 错误假设：isStack=true 叠到上一个 pane，所以先创建就能叠到主图
// klinecharts 不会把 indicator 叠到 candle_pane 上（candle_pane 不是 indicator pane）
chart.createIndicator('BIDASK', true, { id: 'candle_pane' }); // ← 必须加 id
chart.createIndicator('VOL', false, { id: 'volume_pane' });
```

### 错误 3：改用 registerOverlay + createOverlay

```typescript
// ❌ 错误方向：Overlay 的 Coordinate 只有 {x, y}，没有 price 字段
// 无法从坐标反推价格位置，需要 chart.convertToPixel 间接转换
// 增加复杂度且未解决问题
registerOverlay({ name: 'bidask', createPointFigures: ... });
chart.createOverlay({ name: 'bidask' });
```

### 错误 4：移除 figures → indicator 不渲染

```typescript
// ❌ 错误：没有 figures，klinecharts 的 calc/draw 生命周期可能被跳过
// BIDASK indicator 需要 figures 来驱动 draw() 调用
const BIDASK_INDICATOR = {
  name: 'BIDASK',
  figures: [],    // ← 空数组导致 draw() 不被调用的风险
  calc: () => [],
  draw: ...,
};
```

## 正确实现

### BidAskIndicator.ts — 注册 indicator

```typescript
import { registerIndicator, type KLineData } from 'klinecharts';
import type { IndicatorCreate } from 'klinecharts';

const BIDASK_INDICATOR: IndicatorCreate = {
  name: 'BIDASK',
  shortName: 'B/A',
  precision: 5,
  shouldOhlc: false,
  figures: [                                          // ← 必须有 figures
    { key: 'bid', title: 'Bid', type: 'line' },
    { key: 'ask', title: 'Ask', type: 'line' },
  ],
  styles: {
    lines: [
      { color: '#ef5350', size: 1.5, ... },
      { color: '#26a69a', size: 1.5, ... },
    ],
  },
  calc: (list: KLineData[]) => {
    // 返回每根 bar 的 bid/ask 值，驱动 klinecharts 调用 draw()
  },
  draw: ({ ctx, bounding, yAxis, kLineDataList }) => {
    // yAxis.convertToPixel(price) — 精确的价格→像素转换
    // 画全宽虚线 + 右侧价格标签 pill
    return true; // 抑制默认 per-bar 渲染
  },
};

try { registerIndicator(BIDASK_INDICATOR); } catch { /* ok */ }
```

### PriceChart.tsx — 创建到主图

```typescript
// init chart
const chart = init(containerRef.current, { styles: DARK_THEME });

// 副图：VOL 成交量
try { chart.createIndicator('VOL', false, { id: 'volume_pane' }); } catch { /* best-effort */ }

// 主图叠加：B/A 价格线
// ⚠️ id: 'candle_pane' 是 klinecharts 源码中的 PaneIdConstants.CANDLE
try { chart.createIndicator('BIDASK', true, { id: 'candle_pane' }); } catch { /* best-effort */ }
```

## 相关文件

| 文件 | 作用 |
|------|------|
| `frontend/src/components/chart/BidAskIndicator.ts` | BIDASK indicator 注册 + draw 函数 + setBidAsk/setBidAskPrecision |
| `frontend/src/components/chart/PriceChart.tsx` | 创建 BIDASK indicator 到 candle_pane |
| `frontend/src/components/chart/useChartData.ts` | SSE bar 事件中调用 setBidAsk(barTime, bid, ask) 推送实时报价 |
