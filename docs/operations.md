# 运维操作手册

> 施工 agent：本文档中的操作接口必须实现。运维人员（用户）通过本文档执行日常操作。

---

## 1. 添加 MT4/MT5 交易账户

### 1.0 密码存储策略

MT 交易密码**以明文存储**在配置环境变量中，**不做任何加密/哈希**。

**原因**：
1. 连接 mtapi 时，必须将原始密码以明文形式提交给 MT gRPC 网关（`ConnectRequest.password`）
2. 系统无法用哈希值连接 MT 服务器
3. 加密存储（如 AES）只是把明文换成"密文+密钥"，密钥同样在服务器上，等于没加密
4. 增加一次加解密操作，徒增 CPU 负担，无安全增益

**补偿措施**：
- 传输层：所有 gRPC 走 TLS，密码不在网络明文暴露
- 环境变量：密码通过 `ARB_<BROKER>_PASSWORD` 环境变量注入，不写在 config 文件里
- 日志脱敏：`Redacted` 类型包装凭证字段，不在日志中记录原始密码
- 内存安全：凭证加载后立即 Connect，连接成功即丢弃原始字符串（见 constraints §一）

### 1.1 步骤

```
1. 在经纪商处开通 MT4/MT5 账户，获取：
   - 账号（user/login）
   - 密码（master password，用于交易）
   - 服务器地址（host:port 或 server name）
   - 账户类型（demo/real）

2. 确认 API 权限：使用 grpcurl 测试连接
   grpcurl -d '{"user":12345,"password":"xxx","host":"broker.com","port":443}' \
     mt5grpc3.mtapi.io:443 mt5grpc.Connection/Connect

3. 将凭证写入环境变量（不放在 config 文件中）：
   export ARB_NEWBROKER_USER=12345
   export ARB_NEWBROKER_PASSWORD=xxx

4. 在 config/default.textproto 中添加 broker 条目：
   brokers: [
     // ... 已有 broker
     {
       name: "NewBroker"
       platform: PLATFORM_TYPE_MT5
       host: "broker.com"
       port: 443
       user: 0                         # 留 0，从环境变量读取
       password: ""                    # 留空，从环境变量读取
     }
   ]

5. 重启 core：
   kill -TERM $(pgrep arb-core)
   ./bin/arb-core -config=config/default.textproto

6. 验证：打开 desk，价差矩阵中出现新 broker 行
```

### 1.2 连接验证原则

**没有从 MT 服务器获得正确返回值时，一律拒绝保存/启用该 broker。**

```
Connect 失败 / 密码错误 / 服务器拒绝 / 超时
  → 都是无效连接
  → 禁止写入配置、禁止启动 adapter
  → 向用户报告明确错误信息
```

施工 agent：core 启动时遍历所有 broker 配置，逐个 Connect。任一 broker Connect 失败 → 跳过该 broker（不阻塞其他），记录错误到 desk 的 AccountSnapshot 中（`is_connected = false`）。

### 1.3 禁止事项

```
❌ 不要把密码写在 config 文件中（config 文件可能进 git）
❌ 不要用 investor password 做交易（只读密码无法下单）
❌ 不要在 core 运行时修改 config（修改后必须重启）
```

### 1.3 施工 agent 实现要求

**动态添加（v2 功能，v1 可选）**：

DashboardService 需预留以下 RPC，v1 可以不实现，但接口必须存在：

```protobuf
// 预留：动态添加 broker
rpc AddBroker(AddBrokerRequest) returns (AddBrokerReply);
rpc RemoveBroker(RemoveBrokerRequest) returns (RemoveBrokerReply);

message AddBrokerRequest {
  string name = 1;
  PlatformType platform = 2;
  string host = 3;
  int32 port = 4;
  int64 user = 5;
  string password = 6;
}

message AddBrokerReply {
  bool success = 1;
  string token = 2;
  string error = 3;
}
```

---

## 2. 移除交易账户

```
1. 从 config 文件中删除对应 broker 条目
2. 重启 core
3. 验证：desk 中该 broker 行消失
```

---

## 3. 添加新品种（Symbol）

mtapi 的 `Subscribe` RPC 支持动态订阅。添加新品种不需要重启 core。

### 3.1 通过 desk 添加

施工 agent：在 desk 交易 Tab 中添加"Subscribed Symbols"列表 + 添加按钮，调用 gRPC。

### 3.2 通过 grpcurl 添加

```bash
grpcurl -d '{"symbols":["XAUUSD","XAGUSD"]}' \
  localhost:50051 arb.dashboard.DashboardService/SubscribeSymbols
```

### 3.3 施工 agent 实现

DashboardService 需实现：

```protobuf
rpc SubscribeSymbols(SubscribeSymbolsRequest) returns (SubscribeSymbolsReply);
rpc UnsubscribeSymbols(UnsubscribeSymbolsRequest) returns (UnsubscribeSymbolsReply);
rpc ListSubscribedSymbols(ListSymbolsRequest) returns (ListSymbolsReply);
```

内部流程：`DashboardServer.SubscribeSymbols → 通知所有 adapter.Subscribe(symbols)`

---

## 4. 查看策略状态

```bash
# 通过 grpcurl 查询
grpcurl localhost:50051 arb.dashboard.DashboardService/GetStrategyStatus
```

施工 agent 实现：

```protobuf
rpc GetStrategyStatus(StrategyStatusRequest) returns (StrategyStatusReply);

message StrategyStatusReply {
  repeated StrategyItem items = 1;

  message StrategyItem {
    string name = 1;
    bool enabled = 2;
    bool circuit_breaker_open = 3;
    int32 consecutive_losses = 4;
    double window_pnl = 5;
    int32 trades_today = 6;
    double pnl_today = 7;
  }
}
```

---

## 5. 手动触发熔断恢复

```
1. 通过 desk "交易" Tab 的 "Resume Strategy" 按钮
2. 或 grpcurl:
   grpcurl -d '{"strategy":"triangular"}' \
     localhost:50051 arb.dashboard.DashboardService/ResumeStrategy
```

施工 agent 实现：

```protobuf
rpc ResumeStrategy(ResumeStrategyRequest) returns (ResumeStrategyReply);
rpc ResetGlobalCircuitBreaker(ResetCBRequest) returns (ResetCBReply);
```

---

## 6. 紧急 Kill Switch

```
# 通过 desk 菜单栏 Emergency → Kill All
# 或 grpcurl:
grpcurl localhost:50051 arb.dashboard.DashboardService/Kill

# 解除（仅恢复 core，策略仍需逐一手动启用）：
grpcurl localhost:50051 arb.dashboard.DashboardService/Resume
```

---

## 7. 日常检查清单

```bash
# 连接状态
grpcurl localhost:50051 arb.dashboard.DashboardService/GetAccountSnapshots

# 今日 PnL
grpcurl -d '{"from_unix_ms": <当日 00:00 UTC ms>}' \
  localhost:50051 arb.dashboard.DashboardService/GetDailySummary

# 检查 Kill Switch 状态
cat .kill_switch  # 如果文件存在 → switch 已激活

# 检查日志
tail -f audit.log  # 审计日志（protobuf binary）
```

---

## 8. 备份

```bash
# PostgreSQL 备份
pg_dump -Fc arb > backup_$(date +%Y%m%d).dump

# 恢复
pg_restore -d arb backup_20260803.dump

# 配置文件备份
cp config/default.textproto config/backup_$(date +%Y%m%d).textproto
```

---

## 9. 施工 agent 需要新增的 RPC（汇总）

以下 RPC 必须在 `proto/dashboard/dashboard.proto` 中定义并在 `internal/dashboard/server.go` 中实现。

其中标记为 `v1 必须` 的必须完整实现；标记为 `v2` 的只需实现接口骨架（返回 Unimplemented 或空响应）。

| RPC | 优先级 | 说明 |
|-----|--------|------|
| `AddBroker` | v2 | 动态添加 broker，v1 通过 config+重启 |
| `RemoveBroker` | v2 | 动态移除 broker |
| `SubscribeSymbols` | v1 必须 | 所有 adapter 订阅新品种 |
| `UnsubscribeSymbols` | v1 必须 | 所有 adapter 取消订阅 |
| `ListSubscribedSymbols` | v1 必须 | 查看当前订阅列表 |
| `GetStrategyStatus` | v1 必须 | 查看所有策略的运行状态 |
| `ResumeStrategy` | v1 必须 | 手动恢复被熔断的策略 |
| `ResetGlobalCircuitBreaker` | v1 必须 | 手动重置全局熔断 |
