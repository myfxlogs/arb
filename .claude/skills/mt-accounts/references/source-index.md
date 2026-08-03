# MT 账户绑定 — 源码参考索引

> **最后验证**：2026-05-30，已对标当前代码库（v3: sqlc + Service 层）。

## 前端 (React/TypeScript)

| 文件 | 关键内容 |
|---|---|
| `frontend/src/types/account.ts` | `Account` (28 字段)、`BindAccountRequest` (7 字段) |
| `frontend/src/client/account.ts` | `accountApi.create()` / `searchBroker()` / `connect()` / `verifyTradePermission()` / `updateTradingPassword()` |
| `frontend/src/hooks/useAccount.ts` | `createAccount()` / `bindAccount()` / `connectAccount()` / `enableAccount()` / `disableAccount()` |
| `frontend/src/stores/accountStore.ts` | Zustand store: `addAccount()` / `updateAccount()` / `removeAccount()` / `updateAccountStatus()` / `setEnablingAccount()` |
| `frontend/src/pages/accounts/BindAccount.tsx` | 三步向导 UI：Step1(选择经纪商) → Step2(输入凭据) → Step3(确认绑定) |
| `frontend/src/pages/accounts/components/AddAccountCard.tsx` | 金色 "+" 卡片入口按钮，hover 效果 |
| `frontend/src/pages/accounts/components/EditAccountModal.tsx` | 编辑账户弹窗 |
| `frontend/src/pages/accounts/components/DisabledAccountsSection.tsx` | 已禁用账户折叠区域 |

## 后端 (Go)

### ConnectRPC Handler 层

| 文件 | 行数 | 关键内容 |
|---|---|---|
| `backend/internal/connect/user/account_handler.go` | 393 | `AccountHandler`：15 个 RPC handler（CreateAccount / GetAccount / ListAccounts / UpdateAccount / DeleteAccount / ConnectAccount / DisconnectAccount / ReconnectAccount / SearchBroker / VerifyTradePermission / UpdateTradingPassword） |
| `backend/internal/connect/user/account_handler_test.go` | — | 单元测试 |
| `backend/internal/connect/user/account_handler_integration_test.go` | 560+ | 6 集成测试：生命周期、重复绑定、UUID 校验、所有权、密码修改、PG 不可用错误恢复 |

### Service 层（sqlc + 业务逻辑编排）

| 文件 | 行数 | 关键内容 |
|---|---|---|
| `backend/internal/service/account_service.go` | 550 | `AccountService`：15 个账户方法，使用 sqlc 类型安全查询 |
| `backend/internal/service/account_sync.go` | 188 | 账户同步：AnalyticsCache 失效、BatchCreate |
| `backend/internal/service/analytics_cache.go` | 67 | Redis-backed 分析缓存，30-min TTL |

### Repository 层（sqlc 生成）

| 文件 | 行数 | 关键内容 |
|---|---|---|
| `backend/internal/repository/queries/accounts.sql` | — | 6 个参数化 sqlc 查询：GetAccount / ListAccounts / GetAccountCredentials / UserOwnsAccount / UpdateAccountMetrics / GetAccountSnapshots |
| `backend/internal/repository/accounts.sql.go` | 243 | sqlc 自动生成，类型安全 SQL 执行 |

### Proto 定义

| 文件 | 关键内容 |
|---|---|
| `proto/ant/v1/account.proto` | `AccountService` 定义（11 个 RPC） |
| `proto/ant/v1/account_crud.proto` | `CreateAccountRequest` / `UpdateAccountRequest` / `DeleteAccountRequest` |
| `proto/ant/v1/account_entity.proto` | `Account` message（28 字段） |
| `proto/ant/v1/account_connection.proto` | `ConnectAccountRequest` / `ConnectAccountResponse` |
| `proto/ant/v1/account_permission.proto` | `VerifyTradePermission` / `UpdateTradingPassword` |

### 数据库

| 文件 | 关键内容 |
|---|---|
| `backend/migrations/001_init.up.sql` | `mt_accounts` 表结构：`password VARCHAR(255) NOT NULL`（明文列） |

## MT 网关 (mtapi.io)

> **v3 架构**：所有 MT 连接通过 `mdgateway` → `mtapi.io` gRPC。v1 的 `mt4client/mt5client` 已迁至 `backend/legacy/`，不直接在业务代码中使用。

| 文件 | 关键内容 |
|---|---|
| `grpc/mt4.proto` (2028 lines) | MT4 proto 定义：~43 RPC (6 Service) |
| `grpc/mt5.proto` (2986 lines) | MT5 proto 定义：~57 RPC (7 Service) |
| `backend/legacy/mt4client/` | MT4 客户端封装（v1 遗留） |
| `backend/legacy/mt5client/` | MT5 客户端封装（v1 遗留） |
| `backend/gen/proto/ant/v1/account*.go` | 从 proto 生成的 gRPC/Connect 代码 |

## 关键数据流 (v3 — sqlc + Service 层)

```
BindAccount.tsx:handleBind()
  → request = { mtType, brokerCompany, brokerServer, brokerHost, login, password }
  → useAccount().bindAccount(request)
    → useAccount().createAccount(request)
      → accountApi.create(request)
        → accountClient.createAccount(CreateAccountRequest)  [ConnectRPC]
          → backend: AccountHandler.CreateAccount()
            → userIDFromCtx(ctx)                         // JWT 鉴权
            → svc.CreateAccount(ctx, userID, req)        // Service 层
              → repo.GetAccountCredentials()             // sqlc 查询（去重检查）
              → model.MTAccount{...Status:"connecting"}
              → repo.CreateAccount()                     // sqlc INSERT
              → mthub.EnsureSession()                    // L5: session 管理
                → adapter/mt[45].Connect()               // L2: mtapi.io gRPC
                → mtapi.AccountSummary()                 // balance/equity/...
                → repo.UpdateAccountMetrics()            // sqlc UPDATE
              → accountToProto(account)                  // DTO → Proto（共享辅助函数）
      ← addAccount(account)                              // 写入 Zustand store
      ← showSuccess('创建成功')
  ← navigate('/')                                        // 跳转首页
```

## 两关制状态

| 关 | 状态 | 测试数 | 覆盖 |
|:---:|:---:|:---:|------|
| **T关** (Type Gate) | ✅ | 0 `as any` | proto 字段不匹配→编译错误 |
| **I关** (Integration Gate) | ✅ | 19 tests, 全 PASS | 正常路径 + DuplicateBlocked + InvalidUUID + Ownership + PasswordChange + ErrorRecovery |
| **L3** (Audit) | ✅ | 7 轮收敛到零 | Account 板块不再例行审计 |

## 密码安全说明（重要）

**MT 交易密码以明文存储**，不做加密。

- `mt_accounts.password` 列类型 `VARCHAR(255)`，存原始密码
- 连接 MT 服务器时必须使用原始密码，哈希/加密无意义
- 用户登录密码 (`users.password_hash`) 使用 bcrypt 哈希，两者不同
- 安全依赖：HTTPS 传输加密 + JWT 鉴权 + DB 访问控制 + 日志脱敏

## 相关文档

| 文档 | 说明 |
|------|------|
| [pitfalls.md](pitfalls.md) | 踩坑记录：Credit 全链路/MT4-MT5 差异/Proto 零值/表格抖动/suffix 排序 |
