# MT 账户绑定 — 踩坑记录与常见陷阱

> **最后验证**：2026-05-30，基于 7 轮审计 + 19 集成测试经验。

## 1. Credit 字段全链路缺失

**症状**：绑定账户后 credit 显示为 0 / 空，MT4 和 MT5 都受影响。

**根因**：Credit 虽然在 DB `mt_accounts.credit` 列和 proto `Account.credit` 字段中存在，但多层级遗漏：
- `AccountDTO` struct 没有 `Credit` 字段
- SQL `SELECT` 没有查 `credit` 列
- `Scan` 没有扫入 `&a.Credit`
- `UpdateAccountInfo()` 函数签名没有 `credit` 参数
- 所有 4 个 handler 的 `&antv1.Account{...}` 都没有 `Credit: a.Credit`

**修复清单**：
1. `AccountDTO` 加 `Credit float64`
2. SQL 加 `COALESCE(credit, 0)`
3. Scan 加 `&a.Credit`
4. `UpdateAccountInfo` 签名加 `credit float64`，SQL UPDATE 加 credit 列
5. Handler 响应加 `Credit: a.Credit`

**教训**：新增 proto 字段或 RPC 响应字段时，必须逐层检查 DTO → SQL → Scan → Handler → Proto 全部补上。

## 2. MT4/MT5 数据流差异

### Credit 来源不同
| 字段 | MT4 | MT5 |
|------|-----|-----|
| `AccountSummary.GetCredit()` | ✓ 有 | ✓ 有 |
| `OrderUpdateSummary.GetCredit()` | ✓ 有 | ✗ **没有** |
| `ProfitUpdate.GetCredit()` | ✓ 有 | ✓ 有 |

**影响**：MT5 的 `orderUpdateRecvLoop`（`quotes.go`）无法从 `OrderUpdateSummary` 获取 Credit。必须设置 `Credit: 0` 并依赖另一个独立流 `OnAccountProfit` 补充真实数据。

### OpenTime 来源不同
| 字段 | MT4 | MT5 |
|------|-----|-----|
| `Order.GetOpenTime()` | `*Timestamp` | `*Timestamp` |
| `Order.GetOpenTimestampUTC()` | ✗ 没有 | ✓ `int64` unix 秒 |

**影响**：MT5 broker 可能只填充 `OpenTimestampUTC`（int64），不填充 `OpenTime`（Timestamp）。需要 fallback 函数：
```go
func openTimeFromOrder(o *pb.Order) time.Time {
    if t := o.GetOpenTime(); t != nil && t.GetSeconds() > 0 {
        return t.AsTime()
    }
    if ts := o.GetOpenTimestampUTC(); ts > 0 {
        return time.Unix(ts, 0)
    }
    return time.Time{}
}
```

## 3. Proto Timestamp 零值陷阱

### `timestamppb.New()` 行为
| 输入 | 输出 | `omitempty` 行为 |
|------|------|-----------------|
| `time.Time{}` (Go zero) | `seconds: -62135596800` | **不省略** (非零) |
| `time.Unix(0, 0)` (epoch) | `seconds: 0` | **省略！** (零值) |
| 有效时间 | 正确值 | 不省略 |

**关键**：`o.GetOpenTime().AsTime()` 当 proto Timestamp 为零时返回 `time.Unix(0, 0)`（epoch），不是 Go zero time。`timestamppb.New()` 会创建 `seconds: 0` 的 Timestamp，**被 `omitempty` 省略**，前端永远收不到。

### 前端 `@connectrpc/connect` v2 反序列化
`google.protobuf.Timestamp` 在 Connect v2 中反序列化为 `{seconds: bigint, nanos: number}` 对象，**不是** ISO 字符串。`String(v)` 会输出 `[object Object]`。

**修复**：`fromProtoOrders()` 必须将 Timestamp 对象转为 unix 秒（number）：
```ts
const toUnixSeconds = (ts: unknown): number => {
  if (ts == null) return 0;
  if (typeof ts === 'number') return ts;
  const t = ts as Record<string, unknown>;
  if (t.seconds != null) {
    return Number(t.seconds) + Number(t.nanos || 0) / 1_000_000_000;
  }
  return 0;
};
```

## 4. 持仓表格抖动

**症状**：实时数据更新时表格列左右抖动。

**根因**（三层）：
1. `columns` 数组每次 render 重新创建 → Ant Design 重新计算列宽
2. 没有 `tableLayout="fixed"` → CSS 层列宽不稳定
3. `scroll.x` 触发 Ant Design 内部 JS 滚动同步

**修复**：
1. `columns` 用 `useMemo(() => [...], [t, handleClose])`
2. `Table` 组件设置 `tableLayout="fixed"` + `scroll={{ x: 总宽度 }}`
3. `index.css` 兜底：`.ant-table-container > table { table-layout: fixed !important; }`

## 5. `stripSuffix` 后缀匹配顺序

**症状**：`"EURUSD_r"` 被错误解析为 `"EURUSD_"`。

**根因**：后缀列表 `["m","pro","x","c","t","r","_i","_r",...]` 中 `"r"` 在 `"_r"` 之前被匹配。

**修复**：后缀按长度降序排列：`["_institutional","_retail","_i","_r","pro","m","x","c","t","r"]`

## 6. `formatTimestamp` 零值吞没

**症状**：MT5 持仓 `openTime` 显示为空。

**根因**：`if (!ts) return ''` 对 `ts = 0` 返回空字符串。

**修复**：`if (ts == null || ts === '') return ''`，number 类型额外检查 `if (ts <= 0) return ''`。

## 7. 函数签名变更后的测试更新

当修改函数签名时（如 `PublishTick` 加 `context.Context` 参数、`Recalculate` 去掉参数），必须同步更新：
- 所有调用点
- 所有测试文件（`_test.go`）
- 测试期望值（如 mock fallback 移除后需更新期望）

## 8. 集成测试反模式

### 8a. 硬编码 DB 凭据

**症状**：集成测试在 CI 中 `Skipped (no DB_PASSWORD)` 或被拒绝连接。

**根因**：测试硬编码 `"ant:ant@localhost"` 凭据。CI 环境 DB 密码不同，本地也可能不同。

**修复**：从环境变量读取：
```go
dbUser := os.Getenv("DB_USER")
if dbUser == "" { dbUser = "ant" }
dbPassword := os.Getenv("DB_PASSWORD")
if dbPassword == "" { t.Skip("integration test requires DB_PASSWORD") }
```

### 8b. 硬编码测试邮箱冲突

**症状**：`TestErrorRecovery` duplicate key error — email 已存在。

**根因**：测试用硬编码邮箱 `"err-recov@test.io"`，同文件多次运行或并行测试时冲突。

**修复**：用纳秒时间戳生成唯一邮箱：
```go
email := fmt.Sprintf("err-recov-%d@test.io", time.Now().UnixNano())
```

### 8c. nil 指针跳过而非 Fatal

**症状**：`TestAnalyticsCacheHitMiss` nil pointer dereference。

**根因**：`GetTradeStats()` 对无交易记录的账户返回 `nil`，测试直接访问 `result.Stats` 导致 panic。

**修复**：在访问返回结构体字段前检查 nil。如果数据合理为空则 `t.Skip()`，不应用 `t.Fatal()`：
```go
result, err := svc.GetTradeStats(ctx, accountID)
if err != nil { t.Fatalf(...) }
if result == nil {
    t.Skip("no trade data for test account (expected for new accounts)")
}
```

### 8d. 集成测试端口未暴露

**症状**：`TestAnalyticsCacheHitMiss` 连接 Redis 失败 `connection refused`。

**根因**：`redis` 容器没有 `ports` 映射到宿主机。集成测试在宿主机运行（非容器内），无法访问 `redis:6379`。

**修复**：`docker-compose.yml` 中添加端口映射：
```yaml
redis:
  ports:
    - "6379:6379"
```

## 9. DTO→Proto 映射重复

**症状**：4 个 handler 方法各自手写 `&antv1.Account{Id: a.ID, UserId: a.UserID, ...}`，共 28 字段，每次修改 proto 都要改 4 处。

**根因**：无共享的 DTO→Proto 转换函数。

**修复**：提取 `accountToProto()` 辅助函数，所有 handler 共享：
```go
func accountToProto(a *model.MTAccount) *antv1.Account {
    return &antv1.Account{
        Id: a.ID, UserId: a.UserID, Login: a.Login,
        // ... 28 fields, single source of truth
    }
}
```

## 10. userID 提取不一致

**症状**：5 个 AI handler 各自手写 JWT userID 提取，错误信息不一致（有的用英文，有的用中文，有的缺少错误 wrap）。

**根因**：没有共享的 `userIDFromCtx()` 函数。

**修复**：提取到 `helpers.go`，统一错误格式：
```go
func userIDFromCtx(ctx context.Context) (string, error) {
    userID := interceptor.GetUserID(ctx)
    if userID == "" {
        return "", connect.NewError(connect.CodeUnauthenticated, 
            errors.New("authentication required"))
    }
    return userID, nil
}
```
