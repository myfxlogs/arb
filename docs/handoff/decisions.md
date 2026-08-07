# ARB 架构决策记录（ADR-lite）

> 跨会话、跨工具（Claude Code / Windsurf）共享的决策记录。
> 每条格式：**背景 / 决策 / 理由 / 影响**。新决策追加到末尾。

---

## D-001 · AI 协作架构：SSOT + 仓库内共享记忆（2026-08-07）

**背景**
项目要由 Claude Code 和 Windsurf Cascade 交替实施，要求双方用同一套工作方法、能无损双向接手。参考项目（ant，即将删除）的做法有两个硬伤：
1. 约束在 `CLAUDE.md` / `.windsurfrules` / `AGENTS.md` 三处重复 → 漂移，两边看到不同约束。
2. `CLAUDE.md` 末尾指向 Claude 私有 memory 路径（`~/.claude/projects/.../memory/`）→ Windsurf 读不到 → 接手时上下文断裂。

**决策**
1. **SSOT（单一真相源）**：约束 + 工作方法 + 接手协议只写一份在 `AGENTS.md`。`CLAUDE.md` 用 `@AGENTS.md` 内联进 Claude system prompt；`.windsurfrules` 指向 `AGENTS.md`。冲突时一律以 `AGENTS.md` 为准。
2. **共享记忆搬进 git 仓库**：项目状态 / 决策从工具私有目录搬到 `docs/handoff/`（`STATE.md` + `decisions.md`）。入口文件强制"开工读、收工写"。Claude 私有 `~/.claude/.../memory/` 仅留跨项目用户画像，不存项目状态。

**理由**
- 约束单点修改 → 消除三处漂移，两工具看到同一份规则。
- 记忆进 git → 两个工具都能读到，实现真正的无损双向接手。

**影响**
- 所有 agent 开工第一件事：读 `AGENTS.md` + `docs/handoff/STATE.md`。
- 所有 agent 收工必写 `STATE.md`（进度/阻塞/下一步/未决决策）。
- `AGENTS.md` 成为最高优先级契约文件；`CLAUDE.md` / `.windsurfrules` 退化为薄壳入口。

---

## D-002 · 交付前自我审计为强制质量门禁（2026-08-07）

**背景**
用户要求：任何软件 / agent 在完成单项工作后，必须自我审计；架构与实现都必须是最优解、符合第一性原则；代码必须干净、无冗余、无死代码、无技术债、无 bug、全部合规；设计与文档同样适用。需把这条横切纪律写入协作契约，且对所有 agent 无差别强制。

**决策**
在 `AGENTS.md` 新增 **§3 交付前自我审计**，把上述要求落成 A–F 六维可核验清单：
- A 架构最优解（依赖方向 / 复用 / 无更简单等价方案）
- B 实现最优 + 第一性（解法直接对应本质，无多余间接层）
- C 代码洁净（无冗余 / 死代码删除不注释 / 无 TODO·FIXME·nolint·hack·legacy / 无注释代码块）
- D 无 BUG（边界·nil·并发·错误路径·资源泄漏全覆盖 + 测试 + race）
- E 全合规（§7 硬约束 + §10 Before Commit）
- F 设计与文档（同步更新 code-map / docs，无过时冗余）

门槛：A–F 全达标才可交付；存疑按不达标处理；自审结论写入 commit/PR 或 STATE.md。同步更新 `.windsurfrules`、`STATE.md`。

**理由**
"最优解 / 第一性 / 洁净 / 无 bug / 合规"若停留为口号则无法执行；拆成逐条可核验项后，agent 才能真正自检，审计才有抓手。把它放在收工交接之前，形成「工作方法 → 自我审计 → 交接」的完整交付链。

**影响**
- 所有 agent（Claude Code / Windsurf / 其他）在提交或交接前必须过 A–F。
- `AGENTS.md` 章节顺移：自我审计为 §3，原收工交接/项目内容等顺延一位（Before Commit 现 §10、文档索引现 §11）。`.windsurfrules` 的章节引用已同步。

---

## D-003 · 系统重新定位 + 策略聚焦（第一性审视，2026-08-07）

**背景**
第一性审视（北极星 = 给"下单者/用户"提供准确无误的盈利机会）发现三件事：
1. **方向冲突**：设计文档原意是「Core 全自动交易、人只监督」(`evaluation-framework.md:1394`)，与用户目的「给我提供机会、我来决策」直接冲突。
2. **代码偏离**：成本模型 §4.3.1（点差/手续费/滑点/swap）设计完整，但代码零实现；`Notional()` 硬编码 100000 且算的是名义价值不是盈利。
3. **需求/设计双缺口**：用户明确要「套息」，但设计里 swap 只是成本项、无 carry 策略；统计/期现是概率性（文档自承认），与「准确无误」有张力。
4. **物理事实**：「准确无误」分两层——发现时准确（可做到）与执行后准确（跨 broker 同时成交不可能 100%，`evaluation-framework.md:523`）。人工确认恰好夹在两层之间。

**决策（用户拍板）**
1. **系统定位 = 混合**：发现 → 评估（扣全成本算净盈利 + 可执行性预检）→ 推送给用户 → 用户一键确认 → 系统执行。决策权在用户；执行管线能力保留，但**仅在确认后触发**。
2. **策略聚焦 = 确定性三件套 + 套息**：三角 + 跨所价差 + 套息（新增）。统计/期现降级为本期不实现。
3. **净盈利计算是「准确无误」的核心**：`Opportunity` 对象须含成本拆解与 `net_profit`。

**理由**
- "给下单者提供机会" ⇒ 人握决策权 ⇒ 不能全自动。
- "准确无误"分层 ⇒ 人工确认在发现/执行之间多争取一层确定性。
- 确定性套利直接兑现"准确无误"；套息虽带汇率敞口（半确定），但有真实息差支撑，优于纯统计；统一由成本模型把关。

**影响（待落地）**
- `engine` 从"自动执行"重构为 **Detector（发现）+ Evaluator（评估）** 两层，中间不接 Execute。
- 新增 **Opportunity 一等对象** + **OpportunityStream**（推送）+ **ConfirmOpportunity**（确认）RPC。
- `QuoteBus` 输入扩展 **SymbolInfo 缓存**（swap/commission/contractSize，不污染 hot path）。
- 新增 **Carry detector**；统计/期现策略文件不建。
- `evaluation-framework` / `implementation` 多处需改（desk 定位、执行触发、Opportunity 定义、策略清单）；成本模型 §4.3.1 落地为代码。
- 目标架构细节（Detector/Evaluator/Opportunity/Carry）待用户认可后落地。

---

## D-004 · 实施顺序：先 FX，Crypto 留接口（2026-08-07）

**背景**
第一版边界 = FX+Crypto 的 A+B（D-003）。但实施需分阶段：先把整套"发现→评估→确认→执行"链路在一个资产类别上跑通验证，再加第二个，降低风险。用户确认 FX broker 可按需提供（数量充足）。

**决策**
1. **实现顺序：先 FX（MT4/5），Crypto（Binance）留接口暂不实现。**
2. **抽象层仍按 FX+Crypto 设计**（`Listing.Swap(Funding)` 含 swapType/swapLong/Short/结算频率，见 02 §4.3；`Instrument.Kind` 预留 SPOT/PERP），实现分阶段——扩展不改地基。
3. FX broker 充足，故第一步做**跨所价差套利**（最稳、最直接兑现"准确无误"）；单 broker 内同时可做三角 + 套息。
4. broker 数量重质不重量：起步选 2–4 个优质（点差低/swap 友好/稳定/允许套利），非越多越好。

**理由**
- 一套链路先在一个类别验证，比同时双类别风险低、反馈快。
- broker 充足解除跨所前置约束，跨所价差成为最佳起点。
- 抽象兼容保证后续接 Crypto 不返工。

**影响**
- 设计文档按 FX+Crypto 写，实现路线（roadmap）FX 在前、Crypto 留接口。
- `Instrument` 抽象必须为 Crypto 字段预留。

---

## D-005 · 架构最优解重审：多语言 + desk 改 .NET（2026-08-07，推翻 constraints 前端）

**背景**
用户授权：constraints/现有代码/旧原则全可推翻重来，唯一要求"最优解"。以纯工程师视角逐层重审选型。核心判断：**最优 ≠ 单语言统一，而是每层用最适合的语言，gRPC+protobuf 桥接**（跨语言成本极小）。

**决策（逐层第一性选型）**
1. **core = Go**。理由：mtapi.io 是 gRPC（Go gRPC 一等）、goroutine 处理"15 长连接+channel 分发"最简洁、单二进制容器部署最干净、策略迭代快。否决 Rust（性能过剩+开发慢）、.NET core（并发/部署略逊 Go）、C++（过度）。
2. **desk = .NET 8 WPF + C#**（**推翻原 Wails v3 + Svelte/JS**）。理由：Windows 桌面标杆、实时数据绑定强、图表生态成熟（LiveCharts/OxyPlot）、稳定（Wails v3 仍 beta 已踩坑）。grpc-dotnet 连 core（通信不输 Go）。否决 WinUI 3（图表生态弱）、Wails（beta）、Avalonia（跨平台用不上）。
3. **通信 = gRPC + protobuf**（双向 stream 推送 + unary 确认，proto Go/C# 共享）。保留。
4. **存储 = PostgreSQL**（纯 PG 够；第二阶段数据量大可加 TimescaleDB 扩展）。保留。Redis 不需要。
5. **部署 = core 德国 Hetzner VPS（同 mtapi.io region，ping 实测 164ms→几ms）+ desk 本地 Windows**。C/S 架构（第一性必然：采集要近 broker→云 core，UI 要本地→desk）。

**推翻的旧 constraints**
- desk 栈：Wails v3 + Svelte/JS → **.NET 8 WPF + C#**。
- 前端语言：JS(Svelte) → **C#**。
- 「Go 后端唯一」原则**保留**（core 仍 Go）；但前端不再 JS，改 C#——即**多语言架构（Go core + C# desk）**，各层最优。
- AGENTS §6 / constraints §三 / CLAUDE.md / code-map §九 的前端章节须同步改写。

**保留（第一性复审仍最优）**
- core Go、gRPC+protobuf、PostgreSQL、精度分层（float64+decimal）、并发原则（无 pool/无 sync.Map/无热路径 Mutex）、Push-First、MT4/MT5 via mtapi.io。

**理由小结**
- core Go：gRPC+并发+部署综合最优（套利后端网络密集+高并发+容器）。
- desk C# WPF：Windows 桌面+实时数据主场。
- 多语言 > 单语言：各层用最适合工具，gRPC 桥接成本小；强行统一会逼某层次优。
- 这是"各层最优"工程判断，非妥协。

**影响**
- `docs/design/` 全部文档的前端部分（04 desk 桥接、06 gRPC desk 侧、08 Phase C desk UI）须从 Wails/Svelte 改为 WPF/C#。
- constraints/AGENTS/CLAUDE.md 前端章节改写。
- 现有 desk(Wails) 代码 + frontend/ 作废（视为测试，不计沉没）。
- Windsurf 施工按新栈：core Go（复用现有基础设施层）+ desk C# WPF（全新）。
