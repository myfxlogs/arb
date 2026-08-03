---
name: mt-accounts
description: >
  MT4/MT5 交易账户全生命周期管理（全栈）。涵盖经纪商搜索、账户 CRUD、
  绑定向导、连接校验、状态机、密码明文存储策略、两关制状态。当需要实现、修改、审查
  MT 账户相关功能时使用。
---

# MT 交易账户绑定

## 两关制状态

| 关 | 全称 | 状态 | 说明 |
|:---:|------|:---:|------|
| **T关** | Type Gate（编译时） | ✅ | 0 `as any` 强制转换，proto 字段不匹配→编译错误 |
| **I关** | Integration Gate（运行时） | ✅ | 19 集成测试，全 PASS，覆盖正常路径 + 错误恢复 |
| **L3** | Code Audit | ✅ | 7 轮审计收敛到零 |

**停止条件已达成**：T关 ✅ + I关 ✅ + L3 ✅ → Account 板块不再例行审计。

## 核心架构 (v3 — sqlc + Service 层)

```
前端 (React/TypeScript)
  │  三步绑定向导：选择经纪商 → 输入凭据 → 确认绑定
  │  → Connect-RPC: /ant.v1.AccountService/CreateAccount
  ▼
ConnectRPC Handler: connect/user/account_handler.go (393 行)
  │  JWT 鉴权 → 参数校验 → 调用 AccountService
  │
Service 层: service/account_service.go (550 行)
  │  业务逻辑编排、MT 连接管理、账户状态转换
  │  使用 sqlc 生成的类型安全查询
  │
Repository 层: repository/accounts.sql.go (243 行, sqlc 生成)
  │  GetAccount / ListAccounts / GetAccountCredentials / UserOwnsAccount
  │  UpdateAccountMetrics / GetAccountSnapshots
  │
数据层: PostgreSQL mt_accounts 表
  │  密码以明文存储（原因见下文）
  ▼
MT 网关 (mdgateway → mtapi.io)
     Connect(login, password, host, port)
     → AccountSummary() → balance/equity/margin/leverage/...
```

## Proto 契约

```protobuf
// 请求
message CreateAccountRequest {
  string login           = 1;  // MT 交易账号
  string password        = 2;  // MT 交易密码
  string mt_type         = 3;  // "MT4" | "MT5"
  string broker_company  = 4;  // 经纪商名称
  string broker_server   = 5;  // 服务器名称
  string broker_host     = 6;  // 服务器 host:port
}

// 响应
message Account {
  string id, user_id, login, mt_type, broker_company, broker_server, broker_host;
  string status, token, currency, account_type, alias, last_error;
  bool is_disabled, is_investor;
  double balance, credit, equity, margin, free_margin, margin_level, profit, profit_percent;
  int32 leverage;
  Timestamp connected_at, created_at, updated_at;
}

// 完整 RPC
service AccountService {
  rpc CreateAccount(CreateAccountRequest) returns (Account);
  rpc ConnectAccount(ConnectAccountRequest) returns (ConnectAccountResponse);
  rpc SearchBroker(SearchBrokerRequest) returns (SearchBrokerResponse);
  rpc ListAccounts/GetAccount/UpdateAccount/DeleteAccount/DisconnectAccount/ReconnectAccount;
  rpc VerifyTradePermission/UpdateTradingPassword;
}
```

## 前端流程

### 三步向导（3-step wizard）

**Step 1 — 选择经纪商**：
1. 选择 MT4 或 MT5 平台
2. 输入经纪商名称关键词 → 调用 `SearchBroker` RPC
3. 下拉选择公司 → 下拉选择服务器
4. 得到 `brokerHost`（服务器地址，如 `mt4-demo.roboforex.com:443`）

**Step 2 — 输入凭据**：
- 输入交易账号（login）和交易密码（password）
- 密码框使用 `type="text"`（非 password 类型，用户需确认）

**Step 3 — 确认提交**：
- 预览全部信息：经纪商、服务器、平台、账号、密码
- 点击「验证账户」→ MT 连接验证 → 显示余额/净值等信息
- 点击「确认绑定」→ `bindAccount()` → `ConnectAccount()`（阻塞等待 session）→ 导航到详情页

### 关键前端细节

**错误提示 i18n**：`BindAccount.tsx` 的 `friendlyError()` 使用 `i18n.t('accounts.bind.errors.*')` 翻译 MT 错误。翻译 key 定义在 `src/i18n/resources/{lang}/accounts.ts`。

**全局错误拦截器**：`transport.ts` Interceptor 会对所有非连接错误弹 `message.error()`。`CodeInvalidArgument` / `CodeAlreadyExists`（用户输入校验类错误）跳过全局 toast，交还给调用方展示友好信息。

**Server Select showSearch**：有 50+ servers 的经纪商（如 Exness）需要 `showSearch` + 自定义 `filterOption`，否则用户无法在虚拟滚动列表中找到目标 server。

**TanStack Query 缓存策略**：Account Detail 页所有查询使用 `staleTime: 0` + `refetchOnMount: true`。因为账号状态变化频繁（connecting/connected/error），客户端导航时缓存可能返回过期数据。全浏览器刷新（清缓存）和客户端导航（命中 stale 缓存）行为不同。

**formatTimestamp 数字串解析**：SSE bridge 的 `mapOrderToPosition` 将 `openTime` 从 number 转为 `String()` → `"1717000000"`。`formatTimestamp` 需要检测纯数字字符串并按 Unix 秒解析。

**History 自动同步**：Sync History 按钮始终可见（不只是数据非空时）。新账号首次进入 History tab 时自动触发 `syncOrderHistory`（`useRef` 防重复）。同步完成后 `invalidateQueries` 刷新 analytics。

### 核心前端代码路径

| 文件 | 职责 |
|---|---|
| `src/types/account.ts` | `Account`、`BindAccountRequest` 类型 |
| `src/client/account.ts` | `accountApi.create()` / `searchBroker()` |
| `src/hooks/useAccount.ts` | `createAccount()` 编排：调 API → 写入 store → toast |
| `src/stores/accountStore.ts` | Zustand: `addAccount` / `updateAccount` / `removeAccount` |
| `src/pages/accounts/BindAccount.tsx` | 三步向导 UI 组件 |
| `src/pages/accounts/components/AddAccountCard.tsx` | "+" 入口按钮 |

## 后端流程

### ConnectAccount Handler（v5 — channel 事件驱动等待 session）

```go
// connect/user/account_handler.go
// SessionReadyWaiter 提供 event-driven (channel-based) 等待 MT session 就绪 — 零轮询。
type SessionReadyWaiter interface {
    WaitSession(accountID string) <-chan struct{}
}

func (s *AccountServer) ConnectAccount(ctx, req) (*Response, error) {
    s.svc.ConnectAccount(ctx, userID, id)         // DB status = 'connecting'
    s.publisher.PublishConnect(ctx, id, userID)   // NATS 通知 runner

    // 事件驱动等待: Hub.Register() 会 close(ch) — 零 CPU
    select {
    case <-s.sessionWaiter.WaitSession(id):
        // session 就绪
    case <-ctx.Done():
        // 上下文取消
    case <-time.After(5 * time.Second):
        // 超时
    }
    return &Response{Success: true}, nil
}
```

**Hub.WaitSession** (`mthub/types.go`)：
```go
type Hub struct {
    waiters map[string][]chan struct{} // Register 时 close
}

func (h *Hub) Register(id string, s *Session, e OrderExecutor) {
    h.mu.Lock()
    h.sessions[id] = s
    h.executors[id] = e
    for _, ch := range h.waiters[id] {
        close(ch) // 唤醒所有等待者
    }
    delete(h.waiters, id)
}

func (h *Hub) WaitSession(id string) <-chan struct{} {
    h.mu.Lock(); defer h.mu.Unlock()
    if _, ok := h.sessions[id]; ok {
        ch := make(chan struct{})
        close(ch) // 已就绪，立即返回
        return ch
    }
    ch := make(chan struct{})
    h.waiters[id] = append(h.waiters[id], ch)
    return ch
}
```

**关键原则（2026-05-31 确立）**：
- ❌ 不要轮询 `hub.Get() != nil` — 浪费 CPU
- ✅ 用 channel close 事件通知 — `Hub.Register()` 时 signal
- ✅ `ConnectAccount` 返回前 session 已就绪，前端导航后 `OpenedOrders` 不会 500

### Runner 连接成功后更新状态

**问题**：`ConnectAccount` 把 DB 状态设为 `connecting`，但 runner 连接 MT 成功后没有任何代码把状态改回 `connected`。前端永远显示 "Connecting"。

**修复** (`mdgateway/runner.go:startGatewayForAccount`)：
```go
if err := gw.Connect(ctx); err != nil {
    return nil, fmt.Errorf("connect: %w", err)
}
// 持久化 connected 状态，前端立即看到
if deps.PG != nil {
    _, _ = deps.PG.Exec(ctx,
        `UPDATE mt_accounts SET account_status = 'connected', updated_at = CURRENT_TIMESTAMP WHERE id = $1`,
        accID)
}
```

### IsInvestor 全链路（Trader mode vs Investor mode）

MT5 `AccountSummary.IsInvestor` (proto field 12) 和 MT4 `AccountSummary.IsInvestor` (proto field 11) 标识账户是只读密码还是交易密码。

**全链路**：
```
MT AccountSummary.IsInvestor
  → mdtick.MTAccountInfo.IsInvestor (adapter FetchAccountInfo)
    → UpdateAccountInfoTx(..., isInvestor) → DB is_investor
      → AccountDTO.IsInvestor (mtAccountToDTO)
        → accountToProto: IsInvestor: a.IsInvestor
          → 前端 currentAccount.isInvestor ? "Investor" : "Trader"
```

**MT5 adapter** (`mt5/connection.go:FetchAccountInfo`)：
```go
return &mdtick.MTAccountInfo{
    Balance:    s.GetBalance(),
    ...
    IsInvestor: s.GetIsInvestor(),  // ← 新增
}, nil
```

**MT4 adapter**（同样处理）：
```go
IsInvestor: s.GetIsInvestor(),
```

**Proto 映射** (`account_handler.go:accountToProto`)：
```go
IsInvestor: a.IsInvestor,  // ← 之前缺失！
```

### Balance/Credit 订单类型处理

MT5 proto `OrderType_Balance = 100`, `OrderType_Credit = 101`。MT4 proto `Op_Balance = 6`, `Op_Credit = 7`。这些是入金/出金/利息操作，没有 symbol。

**MT5 adapter** (`mt5/orders.go:mt5OrderTypeToSideAndOrderType`)：
```go
case pb.OrderType_OrderType_Balance:
    return mthub.SideBuy, mthub.OrderBalance  // ← 之前 fallthrough 到 default
case pb.OrderType_OrderType_Credit:
    return mthub.SideBuy, mthub.OrderCredit
```

**MT4 adapter**（同样处理）：
```go
case pb.Op_Op_Balance: ot = mthub.OrderBalance
case pb.Op_Op_Credit:  ot = mthub.OrderCredit
```

**account_sync.go**：
```go
case mthub.OrderBalance: ot = "BALANCE"
case mthub.OrderCredit:  ot = "CREDIT"
```

**前端 fallback**（兼容旧数据）：
```tsx
const isBalanceRecord = !symbol || orderType === 'balance' || orderType === 'credit';
```

### CreateAccount Handler（v4 — 验证失败不写入 DB）

```go
// connect/user/account_handler.go
func (s *AccountServer) CreateAccount(ctx, req) (*Account, error) {
    tx, _ := s.svc.BeginTx(ctx)
    defer tx.Rollback(ctx) // 验证失败自动回滚，不写入 DB

    id, _ := s.svc.CreateAccountTx(ctx, tx, ...)

    // MT 经纪商连接验证
    info, err := s.mtTester.Test(ctx, platform, host, login, password)
    if err != nil {
        // ⚠️ 验证失败 → defer Rollback 保证不写入数据库
        // SERVICE_NOT_AVAILABLE / INVALID_ACCOUNT / 密码错误 / 服务器拒绝
        // 都是无效账户，一律拒绝保存
        return nil, connect.NewError(CodeInvalidArgument, "account verification failed: ...")
    }

    // 验证成功才提交
    s.svc.UpdateAccountInfoTx(ctx, tx, id, info.Balance, ...)
    tx.Commit(ctx)
    return account, nil
}
```

**关键原则（2026-05-31 确立）**：没有从 MT 服务器获得正确返回值时，一律拒绝写入数据库。
连接失败 / 密码错误 / 服务器拒绝 / 超时 → 都是无效账户，`defer tx.Rollback()` 保证干净回滚。

### Service 层 (account_service.go)

- 15 个账户方法：Create / Connect / Disconnect / Reconnect / Get / List / Update / Delete
- SQL 查询使用 sqlc 生成的类型安全方法
- 连接管理委托给 `mthub.EnsureSession()`
- 账户同步委托给 `account_sync.go`

### 关键设计：密码明文存储

MT 交易密码 **以明文存储** 在 `mt_accounts` 表，**不做任何加密/哈希**。

**原因**：
1. 连接 MT 服务器时，必须将原始密码以明文形式提交给 MT gRPC 网关
2. 后端无法用哈希值连接 MT 服务器
3. 加密存储（如 AES）只是把明文换成"密文 + 密钥"，密钥同样在服务器上，等于没加密
4. 增加一次加解密操作，徒增 CPU 负担，无安全增益

**补偿措施**：
- 传输层：所有 API 走 HTTPS/TLS，密码不会在网络中明文暴露
- 访问控制：`GetAccount` RPC 校验 `userID`，用户只能查看自己的账户
- 日志脱敏：密码字段不打入日志（`zap` 日志中不记录 password 字段）
- 数据库层面：`mt_accounts` 表的 SELECT 权限受限于应用账户，外部不可直接访问

**与用户登录密码的区别**：

| | 用户登录密码 (`users.password_hash`) | MT 交易密码 (`mt_accounts.password`) |
|---|---|---|
| 存储方式 | bcrypt 哈希 | 明文 |
| 用途 | 验证用户身份（比较哈希） | 转发给 MT 服务器 |
| 是否需要原文 | 不需要 | 必须 |

### BrokerService 搜索逻辑

- 经纪商/服务器列表通过 `SearchBroker` RPC 返回
- 数据来源：MT4/MT5 官方 `brokers.dat` 或内置配置
- 前端用 `companyName` + `access[]` 结构渲染选择器

### 账户状态机

```
connecting  →  connected  →  disconnected
     │              │              │
     └── error ←────┘              │
                                   │
                        disabled (is_disabled=true)
```

- `connecting`：创建中 / 连接测试中
- `connected`：MT 连接正常，实时数据流活跃
- `disconnected`：用户主动断开或会话过期
- `error`：连接失败
- `disabled`：用户禁用账户（不接收数据流）

## 集成测试覆盖 (I关)

19 个集成测试，3 个测试文件，全 PASS：

| 测试文件 | 测试数 | 覆盖 |
|---------|:---:|------|
| `account_handler_integration_test.go` | 6 | 生命周期、重复绑定、UUID 校验、所有权、密码修改、PG 不可用错误恢复 |
| `mthub_service_integration_test.go` | 8 | 订单生命周期、空 canonical、负 volume、无效价格、空持仓、时间范围、SSE 事件 |
| `analytics_integration_test.go` | 5 | 缓存命中/未命中、缓存失效、跨账户鉴权、最近交易、月度 PnL |

运行方式：
```bash
cd backend && go test ./internal/connect/user/ ./internal/connect/system/ -tags=integration -count=1 -v
```

## SSE 实时数据通道

`SubscribeEvents` SSE 流，全程推送，无轮询：

- 首次加载：`OpenedOrders` RPC → 全部持仓 batch 推送为 `position_snapshot`
- 实时更新：`OnOrderUpdate` gRPC stream → `PositionSnapshotBroker` → SSE `position_snapshot`
- 账户指标：`AccountProfitBroker` → SSE `profit_update`
- 用户摘要：`SubscribeUserSummary` → SSE `user_summary`
- 前端 `setPositions` 一次性渲染，避免逐条更新

## 踩坑记录

详见 [references/pitfalls.md](references/pitfalls.md)，涵盖：
- Credit 字段全链路缺失（DTO → SQL → Scan → Handler → Proto）
- MT4/MT5 数据流差异（Credit 来源、OpenTime 来源）
- Proto Timestamp 零值 → `omitempty` 省略 → 前端收不到
- `@connectrpc/connect` v2 反序列化 Timestamp 为对象而非字符串
- 持仓表格左右抖动（columns 重建 + tableLayout + CSS）
- `stripSuffix` 后缀匹配顺序
- `formatTimestamp` 零值吞没
- 函数签名变更后测试同步更新

**2026-05-31 踩坑（本轮完整修复）**：

| 问题 | 根因 | 修复 |
|------|------|------|
| 绑定失败显示原始英文错误 | `transport.ts` Interceptor 对 `CodeInvalidArgument` 弹 raw toast → `handleBind` 的友好提示被掩盖 | Interceptor 跳过 InvalidArgument/AlreadyExists；`friendlyError()` 用 i18n |
| 绑定后导航到详情页 OpenedOrders 500 | `ConnectAccount` 返回时 runner 还未 `Hub.Register` | Hub.WaitSession channel 通知，阻塞到就绪 |
| 新账号状态永远是 "Connecting" | runner 连接成功后无人更新 DB 状态 | `gw.Connect` 成功后 UPDATE account_status = 'connected' |
| Equity 曲线是平的 | 新账号只有 1-2 个快照，曲线在快照之间不变 | `appendLiveEquity` 从 mt_accounts 取实时 equity 附加到曲线最后 |
| History 中 symbol 为空/显示 "undefined" | MT5 Balance/Credit 订单（Op 100/101）未处理，被当成 BUY | 全链路添加 OrderBalance/OrderCredit；前端 `!symbol` fallback |
| Trader mode 是硬编码 | `accountToProto` 漏了 `IsInvestor` 映射 | 全链路：MT API → MTAccountInfo → DB → DTO → Proto → 前端 |
| 新账号 History 不显示 Sync 按钮 | 按钮只在 `historyTrades.length > 0` 时渲染 | 按钮始终可见；空数据时自动触发 sync |
| 客户端导航数据不加载 | TanStack Query `staleTime: 30_000` 命中旧缓存 | `staleTime: 0` + `refetchOnMount: true` |
| Server 选择器无法找到目标 | 59 个 servers 用虚拟滚动，无搜索 | 添加 `showSearch` + 自定义 `filterOption` |
| Positions openTime 为空 | SSE bridge `String(o.openTime)` 产物无法被 `formatTimestamp` 解析 | `formatTimestamp` 检测纯数字串按 Unix 秒解析 |
