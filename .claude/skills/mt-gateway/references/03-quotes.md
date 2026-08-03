---
name: mt-gateway-03-quotes
description: >
  MT4/MT5 实时报价获取。涵盖 SubscribeMany 订阅、OnQuote 流接收、
  tip 结构转换、自动重连机制。报价流是 tick 数据管道的第一级入口。
---

# 03 — 实时报价

## 报价订阅流程

```
Connect（获得 sessionID）
    │
    ▼
SubscribeMany（将品种加入 MarketWatch）
    │
    ▼
OnQuote stream（接收实时 tip 推送）
    │
    ▼
handler(tick)（进入 mdgateway 管道）
```

## SubscribeMany — 订阅品种

将品种加入 MarketWatch，这样 OnQuote 才会推送该品种的报价。

```go
subMd := metadata.New(map[string]string{
    "id":            sessionID,
    "authorization": "Bearer " + mtToken,
})
subCtx := metadata.NewOutgoingContext(ctx, subMd)

_, err := subCli.SubscribeMany(subCtx, &pb.SubscribeManyRequest{
    Id:      sessionID,
    Symbols: []string{"BTCUSDm", "ETHUSDm", "EURUSDm"},
})
```

**必须在 OnQuote 之前调用**，否则 stream 不会有数据。

## OnQuote — 接收实时 tick

```go
md := metadata.New(map[string]string{
    "id":            sessionID,
    "authorization": "Bearer " + mtToken,
})
subCtx := metadata.NewOutgoingContext(subCtx, md)

stream, err := streamCli.OnQuote(subCtx, &pb.OnQuoteRequest{Id: sessionID})
if err != nil {
    // handle error, backoff, retry
}

for {
    tick, err := stream.Recv()
    if err != nil {
        break  // reconnect
    }
    q := tick.GetResult()
    if q == nil {
        continue
    }
    // q.GetSymbol()   — "BTCUSDm"
    // q.GetBid()      — 76334.51
    // q.GetAsk()      — 76339.42
    // q.GetTime()     — timestamp
}
```

## Tick 结构转换

MT4/MT5 返回的 tick 需要转换为内部 `mdtick.Tick` 格式：

```go
handler(&mdtick.Tick{
    UserID:        cfg.UserID,
    AccountID:     cfg.AccountID,
    Broker:        cfg.Broker,
    Platform:      "mt5",
    SymbolRaw:     q.GetSymbol(),
    Canonical:     "",                    // mdgateway 填充
    TsUnixMs:      q.GetTime().AsTime().UnixMilli(),
    ArrivedUnixMs: time.Now().UTC().UnixMilli(),
    Bid:           decimal.NewFromFloat(q.GetBid()),
    Ask:           decimal.NewFromFloat(q.GetAsk()),
    BidVolume:     float64(q.GetVolume()),
})
```

## 重连机制

OnQuote stream 断线后自动重连，带指数退避（1s → 2s → 4s → ... → 5min 上限）：

```go
const maxBackoff = 5 * time.Minute
backoff := time.Second

for {
    // ensureConnected — 如果 conn 丢失则重新 Connect
    if err := g.ensureConnected(ctx, &backoff, maxBackoff); err != nil {
        return
    }

    // OnQuote stream
    stream, err := sc.OnQuote(subCtx, &pb.OnQuoteRequest{Id: sid})
    if err != nil {
        g.sleep(ctx, backoff)
        backoff = minDuration(backoff*2, maxBackoff)
        continue
    }

    backoff = time.Second  // 成功后重置
    // ... 接收循环
}
```

## 默认订阅品种

当账户未配置 `canonical_subscribed_symbols` 时使用：

```go
func defaultQuoteSymbols() []string {
    return []string{
        "BTCUSDm", "ETHUSDm", "XRPUSDm", "SOLUSDm", "BNBUSDm",
        "EURUSDm", "GBPUSDm", "USDJPYm", "XAUUSDm", "US30m",
    }
}
```

## 参考代码

- `internal/mdgateway/adapter/mt5/gateway.go` → Subscribe / recvLoop
- `internal/mdgateway/adapter/mt4/gateway.go` → Subscribe / recvLoop
- `internal/mdgateway/runner.go` → defaultQuoteSymbols
- `internal/mdgateway/adapter/mdtick/mdtick.go` → Tick / TickHandler
