# 02 - 后端事件管道

## 数据流: MT Broker → SSE

```
mtapi (OnOrderProfit gRPC)
  │ 每次账户余额/权益/盈亏变化时推送
  ▼
mt4.Gateway.SubscribeProfit() / mt5.Gateway.SubscribeProfit()
  │ adapter 层, 文件: backend/internal/mdgateway/adapter/mt4/quotes.go:102
  │                                  backend/internal/mdgateway/adapter/mt5/quotes.go:174
  ▼
runner.go: gw.SubscribeProfit(ctx, handler)
  │ handler 将 mdtick.ProfitUpdate 转发到 OnAccountProfit 回调
  │ 文件: backend/internal/mdgateway/runner.go:213
  ▼
OnAccountProfit(accountID, userID, *mdtick.ProfitUpdate)
  │ 回调实现在 server/main.go 的 wiring 中
  │ 转换为 mthub.AccountProfitEvent
  ▼
MtHubService.PublishAccountProfit(ev)
  │ 文件: backend/internal/mthub/service.go:297
  │ 单行: s.profitBroker.Publish(ev)
  ▼
AccountProfitBroker.Publish(ev)
  │ 文件: backend/internal/mthub/types.go:82
  │ 遍历 ev.AccountID 的所有订阅 channel, 非阻塞发送
  │ for _, ch := range chs { select { case ch <- ev: default: } }
  ▼
StreamServer.SubscribeProfitUpdates() / SubscribeEvents()
  │ 文件: backend/internal/connect/system/stream_handler.go
  │ ConnectRPC server-stream (SSE over HTTP)
  ▼
Frontend (浏览器 SSE EventSource → ConnectRPC transport)
```

## DTO 转换链

| 层 | 类型 | 包 |
|----|------|-----|
| mtapi | `mt4grpc.ProfitUpdate` | `backend/mt4/mt4.pb.go` |
| adapter | `mdtick.ProfitUpdate` | `backend/internal/mdgateway/adapter/mdtick/mdtick.go:41` |
| hub | `mthub.AccountProfitEvent` | `backend/internal/mthub/types.go:50` |
| proto | `antv1.ProfitUpdateEvent` | `proto/ant/v1/stream_event_account.proto:9` |
| 前端 | `ProfitUpdate` (camelCase) | `frontend/src/adapters/dataAdapter.ts` |

## AccountProfitBroker 关键实现

```go
type AccountProfitBroker struct {
    mu          sync.RWMutex
    subscribers map[string][]chan *AccountProfitEvent
}

func (b *AccountProfitBroker) Publish(ev *AccountProfitEvent) {
    // 非阻塞: 若 channel 满则丢弃 (保护 broker 不被慢消费者阻塞)
    for _, ch := range chs {
        select { case ch <- ev: default: }
    }
}

func (b *AccountProfitBroker) Subscribe(accountID string) (<-chan *AccountProfitEvent, func()) {
    ch := make(chan *AccountProfitEvent, 8)  // 缓冲 8
    b.subscribers[accountID] = append(b.subscribers[accountID], ch)
    return ch, func() { /* remove + close */ }
}
```

## StreamServer 三种 SSE 端点

| 端点 | 方法 | 粒度 |
|------|------|------|
| `SubscribeEvents` | 聚合流 (order+profit+status+snapshot) | 用户级 (空 accountIds 表示全部) |
| `SubscribeProfitUpdates` | 单账户 profit | 单 accountId |
| `SubscribeUserSummary` | 用户聚合摘要 | 用户级 (所有账户) |
