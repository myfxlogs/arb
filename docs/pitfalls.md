# 踩坑记录

> 已发现的陷阱 + 未来施工/维护中发现的坑追加到此文件。
> 格式：问题 → 根因 → 修复。每个条目标注发现日期。

---

## 1. MT4 Order 响应无 State 字段

| | |
|---|---|
| **发现** | 2026-08-03（设计阶段） |
| **问题** | MT4 `Order` 消息没有 `State`/`CloseVolume` 字段，无法像 MT5 一样用 `resp.Result.State == Filled` 判断成交。 |
| **根因** | MT4 proto（mtapi）的 `Order` 结构比 MT5 少 10+ 个字段。 |
| **修复** | MT4 adapter 中 `Ticket != 0` 视为成交，`CloseVolume = Volume`。不完美但正确——市价单在 MT4 中要么完全成交要么返回 error。 |

---

## 2. MT5 Events 统一流的 proto 定义不完整

| | |
|---|---|
| **发现** | 2026-08-03（设计阶段） |
| **问题** | MT5 `Streams.Events` RPC 的 `EventsReply` 只有 `string result` 字段，不是 `oneof` 多态消息。无法在一个流里区分 Quote/OrderUpdate/MarketWatch 事件。 |
| **根因** | mtapi 的 Events 流可能还在开发中，或实际使用 `/events` websocket endpoint。 |
| **修复** | MT5 adapter 使用独立的 `OnQuote`/`OnOrderUpdate`/`OnOrderProfit` 流（类似 MT4），不使用 Events 统一流。 |

---

## 3. float64 精度在累加中不可靠

| | |
|---|---|
| **发现** | 2026-08-03（设计阶段） |
| **问题** | `decimal.NewFromFloat(1.05000)` 可能产生 `1.0499999999...`。如果在 Warm/Cold Path 直接使用，会被桶计数累计放大。 |
| **根因** | IEEE 754 double 的二进制表示无法精确表示大多数十进制小数。 |
| **修复** | 统一使用 `decimalutil.FromFloat64(f, digits)` 走 `strconv.FormatFloat` 中介。Warm/Cold Path 禁止直接 `decimal.NewFromFloat`。 |

---

## 4. 示例代码中错误检查变量名错误

| | |
|---|---|
| **发现** | 2026-08-03（审查 mtapi goExample 时） |
| **问题** | `main.go:53` 检查了 `resp.Error` 而非 `response.Error`。AccountSummary 调用如果失败，不会触发错误处理。 |
| **根因** | 复制粘贴错误。 |
| **修复** | 施工 agent 的 adapter 实现必须检查每次 gRPC 调用的正确 `response.Error`。Code review 时专门检查。 |

---

## 5. 跨所套利需要双边保证金

| | |
|---|---|
| **发现** | 2026-08-03（设计阶段） |
| **问题** | 跨 broker 套利需要在两个 broker 同时存入保证金。如果 Broker A 有 $10,000 而 Broker B 只有 $500，最大可套利量受限于 $500 的 broker。 |
| **根因** | 套利系统天然的多账户资金分布不均。 |
| **修复** | CapitalGate 的保证金检查必须对所有腿执行，取最紧张的那条腿作为容量上限。Desk 持仓 Tab 显示每个 broker 的 `FreeMargin` 百分比。 |

---

## 6. 重连期间订单状态黑洞

| | |
|---|---|
| **发现** | 2026-08-03（设计阶段） |
| **问题** | Adapter 重连期间该 broker 的所有在途订单状态不可知。可能已成交、可能未成交、可能部分成交。 |
| **根因** | gRPC 流断开 = 失去订单更新推送。 |
| **修复** | 1.1.4 重连状态机：进入 Reconnecting → 标记所有订单 uncertain → 暂停新开仓 → 重连成功 → 全量同步 OpenedOrders 补全状态 → 超限熔断 → 紧急平仓。 |

---

## 7. MT4/MT5 Balance/Credit 订单类型无 Symbol

| | |
|---|---|
| **发现** | 2026-08-03（从 ant 项目引进） |
| **问题** | MT5 `OrderType_Balance(100)` / `OrderType_Credit(101)`，MT4 `Op_Balance(6)` / `Op_Credit(7)` 是入金/出金/利息操作，没有 symbol 字段。如果按普通订单处理，`o.Symbol` 会 panic。 |
| **根因** | 这些订单不是交易订单，是账户操作记录。 |
| **修复** | Adapter 读取 OpenedOrders 时识别这些类型并跳过 symbol 字段。Desk 持仓/历史 Tab 中这些记录标记为 "BALANCE"/"CREDIT" 而非交易。 |

---

## 8. mtapi 网关地址必须包含版本号

| | |
|---|---|
| **发现** | 2026-08-03 |
| **问题** | mtapi 示例代码使用 `mt4grpc.mtapi.io` / `mt5grpc.mtapi.io`，但实际生产地址是带版本号的 `mt4grpc3.mtapi.io:443` / `mt5grpc3.mtapi.io:443`。旧地址可能不可用或被重定向。 |
| **根因** | mtapi 示例未更新到当前生产网关。 |
| **修复** | 全局使用 `mt4grpc3.mtapi.io:443` 和 `mt5grpc3.mtapi.io:443`。 |

## 9. 禁止直连 broker IP — 必须通过 mtapi 网关

| | |
|---|---|
| **发现** | 2026-08-03（从 ant 项目引进） |
| **问题** | `ConnectRequest.Host` 必须填 broker 的真实 IP/DNS，但 gRPC Dial 的目标始终是 mtapi 网关，不是 broker IP。混淆两者会导致连接失败。 |
| **根因** | mtapi 作为代理连接 MT 服务器。Dial 目标 = mtapi 网关，Host 字段 = broker 服务器。 |
| **修复** | `grpc.Dial("mt5grpc3.mtapi.io:443")` + `ConnectRequest{Host: "broker-real-ip"}`。二者永远不同。写入 adapter 实现规范。 |

## 10. MT 密码必须明文 — 加密无意义

| | |
|---|---|
| **发现** | 2026-08-03（从 ant 项目引进） |
| **问题** | MT 交易密码在连接 mtapi 时必须提交明文 `password`。任何加密（AES/Bcrypt）都无法避免——加密的密文最终还是要解密成明文才能发出去，密钥同样在服务器上。 |
| **根因** | mtapi ConnectRequest.password 需要原文，不是哈希。 |
| **修复** | 密码明文通过环境变量注入，不写 config 文件。`Redacted` 类型包装日志输出。TLS 保护传输层。具体见 `operations.md` §1.0。 |
