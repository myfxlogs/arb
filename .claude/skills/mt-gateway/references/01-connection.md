---
name: mt-gateway-01-connection
description: >
  MT4/MT5 连接与认证。涵盖 grpc 拨号、Connect/ConnectEx 登录、CheckConnect、
  Disconnect、sessionID 管理、gRPC metadata 规范、两个易错点（拨号目标与 Host 参数）。
---

# 01 — 连接认证

## 连接 RPC 概览

| RPC | 平台 | 用途 |
|-----|------|------|
| `Connect` | MT4/MT5 | 按 host:port 登录（主路径） |
| `ConnectEx` | MT4/MT5 | 按服务器名登录（如 "RoboForex-Demo"） |
| `ConnectProxy` | MT4/MT5 | 通过 SOCKS5 代理登录 |
| `CheckConnect` | MT4/MT5 | 检测连接状态并自动重连 |
| `Disconnect` | MT4/MT5 | 主动断开连接 |

## 拨号 (Dial)

**不能直连交易服务器**，必须通过 mtapi.io 网关代理。拨号目标由 `brokers.mtapi_endpoint` 决定，为空时回退到公共网关。

| 平台 | 公共网关 |
|------|----------|
| MT4 | `mt4grpc3.mtapi.io:443` |
| MT5 | `mt5grpc3.mtapi.io:443` |

```go
gateway := cfg.MtapiHost
if gateway == "" || gateway == cfg.BrokerHost {
    gateway = "mt5grpc3.mtapi.io:443"  // MT5
    gateway = "mt4grpc3.mtapi.io:443"  // MT4
}
if !strings.Contains(gateway, ":") {
    gateway += ":443"
}

conn, err := grpc.DialContext(ctx, gateway,
    grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})),
    grpc.WithBlock(),
)
```

## Connect — 主登录路径

metadata 携带 `id: "mdgw-<login>"` 作为临时 ID。ConnectRequest 里指定真实 broker 地址：

```go
// metadata
tempID := "mdgw-" + login
md := metadata.New(map[string]string{"id": tempID})
loginCtx := metadata.NewOutgoingContext(ctx, md)

// ConnectRequest.Host = broker_host 的值（去掉端口）
brokerHost := cfg.BrokerHost  // "18.163.85.196:443"
if idx := strings.LastIndex(brokerHost, ":"); idx > 0 {
    brokerHost = brokerHost[:idx]
}

resp, err := connCli.Connect(loginCtx, &pb.ConnectRequest{
    Host:     brokerHost,    // "18.163.85.196"
    Port:     443,
    User:     int32(strToInt(login)),    // MT4
    // User:     strToUint64(login),     // MT5
    Password: password,
    Id:       &tempID,       // MT4: 必须设置
    // MT5: ConnectRequest 无 Id 字段
})
```

## ConnectEx — 按服务器名登录

当只知道服务器名（如 "RoboForex-Demo"）而无 host:port 时使用：

```go
resp, err := connCli.ConnectEx(ctx, &pb.ConnectExRequest{
    Server:   "RoboForex-Demo",
    User:     int32(login),
    Password: password,
})
```

> `ConnectEx` 内部会从 mtapi.io 的服务器列表查找 host:port。少一次 SearchBroker 调用，但依赖 mtapi 的服务器数据库。

## ConnectProxy — 代理登录

通过 SOCKS5 代理连接（某些经纪商屏蔽直连 IP）：

```go
resp, err := connCli.ConnectProxy(ctx, &pb.ConnectProxyRequest{
    Host:          "18.163.85.196",
    Port:          443,
    User:          int32(login),
    Password:      password,
    ProxyHost:     "65.108.126.217",
    ProxyPort:     1080,
    ProxyUser:     "ProxyUser123",
    ProxyPassword: "qwerty123",
    ProxyType:     "Socks5",
})
```

## CheckConnect — 连接检测与恢复

```go
resp, err := connCli.CheckConnect(ctx, &pb.CheckConnectRequest{Id: sessionID})
if resp.GetResult() {
    // 连接正常
} else {
    // 需要重新 Connect
}
```

## Session ID

登录成功后 `resp.GetResult()` 返回 token，这就是 sessionID。对于 MT4，通常就是发送的 `tempID`（如 `"mdgw-95172262"`）。后续所有 RPC 的 metadata `id` 都使用这个值。

```go
sessionID := resp.GetResult()
if sessionID == "" {
    return fmt.Errorf("empty token")
}
```

## Disconnect — 主动断开

mtapi 侧：
```go
connCli.Disconnect(ctx, &pb.DisconnectRequest{Id: sessionID})
```

ant 侧（Gateway.Disconnect）：
```go
func (g *Gateway) Disconnect(ctx context.Context) error {
    g.mu.Lock()
    defer g.mu.Unlock()
    if g.cancelSub != nil {
        g.cancelSub()       // 取消订阅流
        g.cancelSub = nil
    }
    if g.conn != nil {
        g.conn.Close()
        g.conn = nil
    }
    // 清理所有客户端句柄
    g.client = nil
    g.connCli = nil
    g.streamCli = nil
    g.subCli = nil
    g.qhCli = nil
    g.sessionID = ""
    return nil
}
```

## gRPC 元数据规范

```
          ┌────────────────┬─────────────────────────────────┐
          │   RPC 调用      │  metadata 必须字段               │
          ├────────────────┼─────────────────────────────────┤
          │ Connect         │  id: "mdgw-<login>"             │
          │ ConnectEx       │  id: "mdgw-<login>"             │
          │ CheckConnect    │  id: <sessionID>                │
          │ Disconnect      │  id: <sessionID>                │
          │ Symbols         │  id: <sessionID>                │
          │ AccountSummary  │  id: <sessionID>                │
          │ SubscribeMany   │  id: <sessionID>                │
          │                 │  authorization: "Bearer <token>" │
          │ OnQuote         │  id: <sessionID>                │
          │                 │  authorization: "Bearer <token>" │
          │ OrderSend       │  id: <sessionID>                │
          │ PriceHistory    │  id: <sessionID>                │
          └────────────────┴─────────────────────────────────┘
```

- 所有 RPC 都必须携带 `id`
- `authorization: Bearer <mt_token>` 仅用于 SubscribeMany、OnQuote 和 stream 类 RPC
- Connect/ConnectEx 阶段不需要 authorization

## MT4/MT5 差异表

| | MT4 | MT5 |
|---|---|---|
| gRPC 网关 | `mt4grpc3.mtapi.io:443` | `mt5grpc3.mtapi.io:443` |
| ConnectRequest.Id | **必须设置** (`&tempID`) | **无此字段** |
| ConnectRequest.User | `int32` | `uint64` |
| 客户端类型 | `MT4Client` + `ConnectionClient` + `StreamsClient` + `SubscriptionsClient` | 同 MT4 + `QuoteHistoryClient` |
| 可用 Service | 6 个（MT4, Connection, Trading, Streams, Subscriptions, Service） | 7 个（MT5, Connection, Trading, Streams, Subscriptions, QuoteHistory, TickHistory, Service） |

## 两个易错点

### 坑 1：拨号目标

❌ `grpc.Dial("18.163.85.196:443")` — 直连交易服务器行不通

✅ `grpc.Dial("mt4grpc3.mtapi.io:443")` + `ConnectRequest{Host: "18.163.85.196"}`

### 坑 2：ConnectRequest.Host

❌ 使用 `broker_server`（"Exness-Trial"）— DNS 无法解析

✅ 使用 `broker_host`（"18.163.85.196"）— 真实 IP/域名

### 坑 3：MT4 必须设置 Id

❌ MT4 ConnectRequest 不设 Id → 连接可能失败

✅ `Id: &tempID`（`"mdgw-" + login`）

## 参考代码

- `internal/mdgateway/adapter/mt4/gateway.go` → Connect / Disconnect
- `internal/mdgateway/adapter/mt5/gateway.go` → Connect / Disconnect
- `internal/mdgateway/adapter/mdtick/mdtick.go` → AccountConfig
- Legacy：`/opt/ant/backend/legacy/mt4client/connection.go`
- Legacy：`/opt/ant/backend/legacy/mt5client/connection.go`
- alfq 参考：`/opt/alfq/backend/go/internal/mdgateway/adapter/mt4/client.go`
- MT4 proto：`/opt/ant/grpc/mt4.proto` L8-52（Connection service）
- MT5 proto：`/opt/ant/grpc/mt5.proto`（Connection service）
