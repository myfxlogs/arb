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

---

## D-006 · 竞品 UI 借鉴：机会列表表格 + 腿角色 + Carry 年化 + 对冲手数归一化（2026-08-07）

**背景**
用户提供一张同类对冲套利产品的前端截图（`docs/1.png`，仅一张，无持仓/平台明细页）。截图信息密度高、且与本系统高度同构（15 平台、套利机会列表、对冲套息），值得做"取其精华、保我优势"的借鉴。分析截图得出六个可借鉴点，其中一个是强有力的外部证据（正收益对冲套息真实存在）。

**决策（借鉴 6 点 + 保 2 点优势）**
1. **机会列表形态：卡片 → Master-Detail 表格**（`10 §4`）。主表一机会一行（高密度扫描 + 筛选/排序），选中行展开详情面板（全成本拆解 + 确认）。竞品表格一屏看十几条，远胜卡片。
2. **腿经济角色 `LegRole`**（`02 §5` / `06`）。Carry 两腿标 `Income`（收息腿，正 swap）/ `Hedge`（对冲腿，负 swap）；CrossExchange/Triangular 两腿经济对称用 `None`+Direction。第一性：角色因策略而异，不硬套同一框架。
3. **度量双轨：NetBps（绝对）+ Carry 年化**（`02 §5.1`）。短周期策略用绝对 NetBps；Carry（天~周）用组合年化（`日净swap×365/名义`），因绝对 bps 无法跨"持仓天数不定"的机会比较。
4. **对冲手数归一化**（`02 §3.1`）。对冲前提 = 两腿名义价值相等；`ContractSize` 异时手数反比。FX 同品种通常 1:1；截图 UKOIL↔XBRUSD 需 1:10；Crypto 接入后更悬殊。公理②"换算非抹平"的对偶——换手数不抹规模差。
5. **风险提示列**（`10 §4.1`）。把已有的 `Executable`/`Confidence`/`Remaining` 映射成可读提示（不可执行/临期/低置信/可执行 + 颜色），等价竞品"价差偏大/组内最小价差"。
6. **筛选/排序栏**（`10 §4.4`）。品种/平台/状态筛选 + 按度量/差异/剩余时间排序（`CollectionViewSource`）。

**保留（竞品没做到、我们的优势）**
- **人工确认**（D-003）：截图未见确认按钮（似自动列出机会），我们保留"确认执行"——人握决策权。
- **全成本透明**（公理③）：详情面板逐项点差/手续费/滑点/swap 拆解；竞品"组合年化"是否扣全成本不确定，我们扣全成本。

**外部证据（强化 Carry 方向，非夸大）**
截图竞品（CFD 差价合约品种）实测存在**年化 +22% 的正收益对冲套息组合**（收息腿 +61.6%、对冲腿 −39.6%、组合净 +22.0%）。这证明正收益对冲套息**在足够品种覆盖下确实存在**——我们的 FX 实测（ICMarkets+Exness）暂为负（−28/天），是覆盖不足而非机制不可行 → 强化 `03 §2.2`"Carry 检测器仍建 + 扩大覆盖"。（诚实区分：竞品为 CFD 品种，非我们 FX MT5 样本；正收益需我们自己的覆盖验证后才推送。）

**理由**
- 借鉴的是**信息架构与度量**（表格/角色/年化/对冲手数），非照搬 UI 皮——这些恰好补齐我们 desk 设计的决策效率短板。
- 腿角色、对冲手数归一化、年化度量是**模型层**改进（非纯 UI），让"准确无误"更精确（对冲不留敞口、长期持仓度量可比）。
- 保人工确认 + 全成本 = 不丢 D-003/公理③ 的立身之本。

**影响（已落地到文档）**
- `02 §3.1`（对冲手数归一化）/ `§5`（Leg+LegRole）/ `§5.1`（度量双轨）/ `§8`（回溯）。
- `03 §2.1`（对冲手数引用）/ `§2.2`（腿角色 + 外部正收益佐证）。
- `06 §5.2`（Opportunity 加 `net_swap_per_day`/`hold_days_hint`/`annualized_net_bps`；Leg 加 `role`/`daily_swap`/`annualized_bps`；新增 `LegRole` 枚举）。
- `10 §2/§3/§4`（Master-Detail 表格重写 + 列 + 筛选排序栏）/ `§11`（回溯）。
- 实现时 Windsurf 须按新 proto 字段同步（`06 §5.2`）；Carry 年化由 Evaluator 算（`NetSwapPerDay` 来自 §4.2 swap 换算）。
- 截图存档 `docs/1.png`（参考，非交付物）。

---

## D-007 · 自审作用域明确：文档审归 Claude，代码审归施工者（2026-08-07）

**背景**
AGENTS §3 自审原表述"适用于所有 agent，无差别"，但未区分**文档**与**代码**的审计责任归属——这是个空白，导致疑问"文档审计由谁做"。实际分工：Claude 是文档定稿人（用户定："定稿由你完成"）+ 第一责任人 + 安全审计负责人（§0）；Windsurf 是施工者，遇设计疑问须回找决策者不自行变更（§0）。

**决策**
在 `AGENTS.md §3` 新增 **§3.0 自审作用域**：
1. **设计文档**（`docs/design/`、`AGENTS.md`、proto 定义）：定稿 + 自审均 **Claude**。每次改文档必自审（防跨文档自相矛盾）。
2. **代码**：施工 agent 做 A–F 自审 + Claude 架构/合规复审。其中 E+F 是"代码对文档的反向核对"，非"文档本身"的审计。
3. 施工 agent 读文档发现矛盾/不可实现 → **上报 Claude，不自行改文档**。施工 agent 是文档的"第二读者/实现反馈者"，非文档审计责任人。

**理由**
- 审计权归属定稿权——Windsurf 无架构决策权、非文档作者，没有立场"审"文档对错；强行让它审会与 §0"不自行变更设计"冲突。
- 自审的真正价值在防自相矛盾（D-006 落地时 Claude 自审就抓出 `AnnualizedNetBps` 存储vs推导的 proto/Go/UI 三处不一致），必须由最懂全局架构的定稿人执行。
- 把"机制同等强制"与"作用对象因角色而异"分开表述，消除"无差别"的歧义。

**影响**
- `AGENTS.md §3` 新增 §3.0（被 `CLAUDE.md @AGENTS.md` + `.windsurfrules` 内联生效）。
- Windsurf 接手时不再困惑"文档我该不该审"——它只审自己写的代码（A-F，E/F 含对文档的反向核对），文档问题上报 Claude。
- Claude 每次改文档后必跑一次跨文档自审（A-F，重点 F）。

---

## D-008 · Phase A 审查结论 + Phase B Evaluator 设计决策（2026-08-07）

**背景**
Phase A（数据源地基，Windsurf 施工）已通过真实验收（真实 MT5 数据吻合 02 §7）。本轮 Claude 按 §3 A–F + §3.0（文档/代码双审）复审 Phase A 全部产出，并设计 Phase B（Evaluator，落地 02 §6）成文 `docs/design/12-evaluator.md`。设计过程中发现 3 处**跨文档不一致**（02 §4.4 要求 commission 入 Listing，但 §1.2 结构与代码都无；02 §6 要评估阈值，但 config.proto 无该参数；Instrument 解析层缺失）须在 Phase B 前补齐。

**决策**

*Phase A 审查（A–F）*：
1. **映射正确性已核**：proto↔Go 枚举数值逐项对齐（SwapType/CalcMode/TradeMode/ExecutionType/FillingFlags/V3DaysSwap），`mt5_listing.go` 直接强转注释属实；字段名对 proto；build/vet/test 通过。地基扎实，可作 Phase B 输入。
2. **A–F 门禁有 4 处待修**（详见 STATE.md「Phase A 审查结论」）：F1 已修（`cache_test` 违规 `decimal.NewFromFloat`→`RequireFromString`）；F2 `Cache.Populate` 全失败静默返 nil 须改返错；F3 proto→Listing 映射须补单测；F4 `TripleSwap` 双存须去重。F2/F3/F4 建议 Windsurf 在 Phase B 前置时一并清。

*Phase B Evaluator 设计（`12-evaluator.md`）*：
3. **纯函数算核**：`Evaluate(Candidate)→*Opportunity`，warm-path decimal，无 broker I/O/无副作用。落地 02 §6 七步。Executable=false 也产出（透明展示拒因，呼应 D-006 风险提示列）。
4. **三项前置补齐**（跨文档不一致修复）：① `Listing` 加 `Commission{Mode,Rate}`（默认 0，未配置则 desk 标注——诚实高估，不抹平 MT5 固有限制）；② `listing.CanonicalIndex` 解析器（symbol_map+cache→(broker,canonical) Listing，Instrument 由 canonical 推导，补 Phase A 留下的 nil）；③ `config.proto` 加 `EvaluatorConfig`（阈值/滑点/新鲜度/容差/持仓天数/盘口）。
5. **swap 未实测模式不猜值**：InPoints 已验证为主公式；SymInfo_s408/PointClose/PointBid 等未实测模式 → 判 Executable=false（无法保证成本准确 = 无法保证「准确无误」），**禁止猜 0**（swap 多为成本，猜 0 造假机会）。Phase F 归因/扩覆盖后精确化。
6. **Notional 定义**：对冲手数归一化后两腿名义相等 → `Notional = 基准腿名义`（USD）；NetBps = NetProfit/Notional×10000。
7. **黄金用例锚定真实数据**：测试表直接复用 02 §7 对照表 + 03 §2.2 实测 swap，锁死「准确无误」可核验性。

*项目级缺口（已解）*：
8. **行数检查统一为 `scripts/check-lines.sh`（最优解，非造 Go 工具）**：`tools/check-file-lines` 从未存在（`development.md:343` 自承「需自己实现」）；根因 = CI 用 shell 片段、5 处文档却引用不存在的 go 工具（两套分裂）。落 `scripts/check-lines.sh` 单一 shell 真相源（>450 失败 / >300 警告，豁免 `proto/`+`docs/`+`*_test.go`），CI + AGENTS §10/constraints/development/11-testing 全指向它，删 phantom 引用。顺带补 CI 漏排 `*_test.go`（规格偏离）、排 `docs/ant/`（本地扫到、CI 因未提交侥幸通过）。当前 EXIT=0（4 软警告，0 硬违例）。为数行数专门写 Go 程序属过度工程（§2.6），shell 片段是更直接的解。

**理由**
- Phase A 难点在「proto→Listing 映射的正确性」，已用真实数据 + 枚举逐项核对验证通过；剩下的 F2/F3/F4 是工程洁净度，不撼动地基。
- Phase B 三项前置都是「设计文档要求了、但代码/配置尚未跟上」的真不一致——第一性必须先补齐才能让 Evaluator 有正确输入；这正是 §3.0「文档审归 Claude」的价值（跨文档自洽）。
- swap 未实测模式「不猜值、判不可执行」是把公理③（漏成本=假机会）从口号落到可核验行为：宁可少推、不可错推。
- Notional 取归一化后相等的名义，是对冲手数归一化（02 §3.1）的自然推论，避免另造度量。

**影响**
- `docs/design/12-evaluator.md` 新增（设计 SSOT，Windsurf 照此施工）。
- `code-map.md §7` 加 Phase A→B 前置 + Phase B 文件清单。
- `docs/design/README.md` 索引加 12。
- Phase B 施工顺序：前置（Commission/CanonicalIndex/EvaluatorConfig proto 全套同步）→ 子模型（§4 各文件+单测）→ 主流程编排 → 黄金用例。
- Phase A 的 F2/F3/F4 待修；F5（行数检查）已解（`scripts/check-lines.sh` 统一，CI + 文档共用）。

---

## D-009 · 仓库瘦身：docs/ant 移出 + Makefile 清死目标（2026-08-07）

**背景**
用户提出「是否把历史文件全删、做成全新项目」。盘点事实：git 仓库本身已极精简（~120 跟踪文件 / 292KB Go core），273MB 体量 99% 来自**未跟踪**的 `docs/ant/` 参考项目；可复用 Go core 是刚验收的真资产。Claude（§0，技术判断优于用户时须直说）**反对推倒重写**（违反 §2.1 root-cause-first、纯沉没成本），建议剪枝而非新建。

**决策（用户拍板）**
1. **`docs/ant/` 移出仓库**至 sibling `/opt/arb-ant-ref`（同文件系统，`mv` 秒级 rename；保留参考、不污染本仓）。**推翻** STATE 原「保留 docs/ant/」的决定。
2. **Makefile 清死目标**：删 `run-desk`/`build-frontend`/`build-desk`（D-005 已废 Wails/Svelte，`cmd/desk`+`frontend` 早已 0 跟踪文件）；`test`/`lint` 去掉无意义的 `grep -v '/desk'`。
3. **旧设计文档**（`evaluation-framework.md`/`implementation.md`）**保持现状**作历史快照（design README 已声明），不移不删。
4. **可复用 Go core 全保留**（decimalutil/errclass/bus/adapter/store/execute/risk/listing）——Phase B 地基，不碰。

**理由**
- 仓库「重」的错觉来自一个 273MB 未跟踪 blob；移走即得「全新项目」的轻装感，零浪费。
- 推倒 core = 重写刚验证的最优解，反 §2.1；本仓并无遗留代码烂摊子可清。
- Makefile 死目标是 D-005 迁移遗留，顺手清掉（§C 无死代码）。

**影响**
- 工作树 **-273MB**；`git status` 不再有 `?? docs/ant/`。
- AGENTS §11 / `.windsurfrules` / STATE 的 `docs/ant` 引用改为「已移出」。
- Makefile 不再有 desk/frontend 目标（desk 将来为独立 C# 项目，不走本 Makefile）。
- `check-lines.sh` 仍排 `docs/`（`docs/api` 的 mtapi Go 示例仍是参考、非我方代码），行为不变。

---

## D-010 · Phase C Detector 实现级设计：13-detector.md（2026-08-07）

**背景**
Evaluator 落地后（B-1 复审通过），下一阶段是 Detector（候选发现层）。`03-strategies.md` 已有策略级规格（三类 Detector、职责边界、Candidate 接口），但缺少实现级细节——文件布局、逐类扫描算法、QuoteBus 消费模式、CanonicalIndex 接入、黄金测试用例。这些是实现级文档，对标 `12-evaluator.md` 的深度。

**决策**
1. **设计成文 `13-detector.md`**：实现级规格，包含三类扫描器的精确算法、Quote 消费模式（Snapshot 轮询，100ms）、文件布局（5 文件）、黄金用例。
2. **Detector 不建独立 Candidate 类型**——复用在 `evaluator.Candidate`（Detector→Evaluator 单向 import evaluator，不违依赖方向）。
3. **CrossExchange 优先**（最稳、闭环最短），次 Carry（swap 差），最后 Triangular（三腿成交最难）。
4. **Triangular 初版枚举 3–5 已知三角**（EUR/USD/GBP、EUR/USD/JPY 等），不写全自动图搜索——broker+品种少时枚举 > 通用性。
5. **不新建 orchestrator**：Detector 的 Scan 是纯函数；运行时循环（Snapshot→Scan→Evaluate→push）在 cmd/core 或 engine 层（本阶段不建，后续接线时加）。

**理由**
- 手递 Evaluator 顺是因为 `12-evaluator.md` 把实现细节全写清了（公式、文件布局、黄金用例）。Detector 更复杂（跨 broker 配对、三类算法），没有对等级别的设计文档就交给 Windsurf 大概率走偏。
- Candidate 类型共享避免重复定义（§C 无冗余）；单向 import 不违依赖方向（Evaluator 定义输入格式，Detector 导入消费）。
- 三角枚举是初版最优解——通用图搜索对 2-4 broker × 10-20 品种是过度工程（§2.6）。

**影响**
- `docs/design/13-detector.md` 新增（实现 SSOT，Windsurf 照此施工）。
- `docs/design/README.md` 索引加 13；`code-map.md §7` 加 Phase C 文件清单。
- Detector 施工顺序：CrossExchange → Carry → Triangular，逐类加测试。交付前过 A–F。

---

## D-011 · Phase D Dashboard 机会闭环接线（2026-08-07）

**背景**
Phase C 中 Windsurf 顺手建的 `internal/engine/` 已完成扫描循环（Snapshot→CanonicalIndex→Detect→Evaluate→push），含 sub/pub + ConfirmOpportunity。下一步是把 engine 的 sub/pub 接到 gRPC OpportunityStream + ConfirmOpportunity unary，闭环「发现→评估→推送→确认」链路的最后两环。

**决策**
1. **设计成文 `14-dashboard-wiring.md`**：实现级规格，proto 变更清单 + dashboard Go 实现 + 类型映射 + 测试计划。
2. **Proto 同步** `dashboard.proto`：搬入 `06 §5.2` 全部 message + enum + 新增 `OpportunityStream` / `ConfirmOpportunity` RPC。`buf generate` 全套。
3. **Dashboard Go**：`internal/dashboard/opportunity.go` — `OpportunityStream`（订阅 engine→stream.Send）+ `ConfirmOpportunity`（调 engine.ConfirmOpportunity）+ 类型映射（evaluator.Opportunity→proto Opportunity，decimal→string，time→unix_ms）。
4. **不改 Engine 逻辑**、不改 Detector/Evaluator——纯接线。

**理由**
- Engine 的 sub/pub 模式天然对接 gRPC server stream（一个 goroutine select on channel+ctx.Done → stream.Send）。
- 类型映射集中在一处（`toProtoEvent`），避免 proto 生成类型散落到 evaluator 或 engine 包。
- 短规格（~100 行）足以指导实现；

**影响**
- `docs/design/14-dashboard-wiring.md` 新增。
- `proto/dashboard/dashboard.proto` 新增内容；`buf generate` 重生成 Go + C# stub。
- `internal/dashboard/opportunity.go` 新增。
- Phase D 施工后，全链路「Quote → Detect → Evaluate → Engine → gRPC → desk」可运行。

---

## D-012 · 仓库二次瘦身 + Phase E Desk WPF 设计（2026-08-07）

**背景**
D-009 移走 docs/ant（273MB）后，跟踪文件里仍有被 docs/design/ 取代的旧设计文档（evaluation-framework.md 63K、implementation.md 36K）、非我方代码的 mtapi Go 示例（docs/api/ 440K）、以及无任何引用的旧 Wails desk TLS 证书（certs/ 2.8K）。用户要求彻底清理使仓库干净。同时 Phase E desk WPF 需要实施级设计文档。

**决策**
1. **删 12 个死文件**（`git rm`）：`docs/evaluation-framework.md` + `docs/implementation.md`（被 00-15 取代）+ `docs/api/`（8 文件，mtapi 示例）+ `certs/`（2 文件，旧 TLS 证书无引用）。AGENTS §11 索引同步去 implementation.md、补 docs/design/。
2. **Desk WPF 实施设计 `15-desk-wpf.md`**：.NET 8 项目骨架 + NuGet 包（Grpc.Net.Client/Grpc.Tools/CommunityToolkit.Mvvm）+ MVVM 模式 + grpc-dotnet 线程模型 + 6 视图文件清单 + v0→v1→v2 分阶段实施。
3. **Desk 优先于 audit**：归因校准需要真实成交数据 → 成交需要确认→执行接线 → 确认需要 desk UI → 先 desk。

**理由**
- 删除的文件覆盖 "旧设计/外部示例/死配置" 三类，删除后仓库只有当前设计+当前代码，无歧义。
- Desk WPF 是 C# 独立项目（D-005），Windsurf 可直接创建 .NET 项目。
- 分阶段（v0 最小可用 → v1/v2 完善）降低风险，每阶段独立可验收。

**影响**
- git 跟踪文件 -12（540K），工作树 3.4M。
- `15-desk-wpf.md` 新增；code-map §7 加 Phase E 文件清单。
- AGENTS §11 索引更新。

---

## D-013 · Phase F 执行接线 + Audit 归因设计（2026-08-07）

**背景**
Phase E desk WPF v2 完成后，下一步是把 Engine 的 ConfirmOpportunity 接到 ExecutionPipeline（让人确认的机会真正下单），同时用 Evaluator 算好的真实 NotionalUSD 替换 pipeline 里的硬编码 ×100000。另外归因记账（17-audit.md）是「准确无误」闭环的最后一块——成交后用实际 swap/commission/滑点校准 Evaluator 预估参数。

**决策**
1. **设计成文两份**：`16-execute-wiring.md`（执行接线 + Notional 替换）+ `17-audit.md`（Event Logger + opportunities 表 + 归因骨架）。
2. **ConfirmOpportunity → 异步 Pipeline.Execute**：gRPC unary 立即返回 `{Accepted: true}`，go routine 异步执行 pipeline → 结果回填 Filled/Failed → broadcast。
3. **Notional 替换**：`ArbitrageOpportunity` 加 `NotionalUSD float64` 字段，`Notional()` 直接返回该字段（由 Evaluator 算，不再硬编码 ×100000）。
4. **Audit JSON Lines**（非 protobuf 文件）：同步写、人工可 grep/jq，Phase F 归因分析直接用。
5. **opportunities 表 + audit_events 表**：DDL 含预估/实际/偏差字段，支持归因校准查询。

**理由**
- 异步执行不阻塞 gRPC 返回，用户确认后立即看到 "Confirmed"。
- Notional 替换消除最后一个硬编码魔术数（D-003）。
- JSON Lines 审计比 protobuf 更实用（人工可读、可 grep、可 jq）。

**影响**
- `docs/design/16-execute-wiring.md` + `17-audit.md` 新增。
- `code-map.md §7` 加 Phase F 文件清单（后拆为 Phase F 执行接线 + Phase G Audit）。
- Phase F 施工范围：F-1~F-4（执行接线）+ G-1~G-6（审计归因）。

---

## D-014 · Phase F 复审：执行接线通过、审计归因拆分 Phase G（2026-08-08）

**背景**
Windsurf 完成 Phase F 施工后 Claude 复审。实际交付 = 执行接线（16-execute-wiring.md）全部完成 + Phase E v2 bug fixes F1–F4 完成，但审计归因（17-audit.md）**零交付**（`internal/audit/` 不存在、Engine 无埋点、store/opportunities.go 不存在）。DDL 已就位（`migrations/003_opportunity.sql`，Phase A 已提交）。

**决策**
1. **执行接线部分 A–F 全达标**：架构正确（无循环依赖）、Notional 替换干净、6 引擎测试覆盖正常/失败/边界/Cancel、go build/vet/test/check-lines 全过。
2. **Phase E v2 bug fixes F1–F4 复核通过**：SignalRecord 对齐 DB schema、handler 补填字段、strategy 筛选生效、SL/TP 传递完整。
3. **审计归因拆为 Phase G**（独立 phase，G-1~G-6）：`internal/audit/` 包 + store CRUD + Engine 5 处埋点 + main.go 接线 + context 生命周期修复 + 测试。
4. **复审发现 2 项**（非阻塞，Phase G 顺手修）：① `engine.go:101` `context.Background()` → 应用 engine 的 `runCtx`（shutdown 时取消在途 pipeline）；② `engine_test.go` 缺 `Executable=false` 的 Confirm 拒绝测试。

**理由**
- 执行接线可独立验收（A–F 全达标），不等审计代码绑在一起提交。
- 审计归因是独立功能块（§2.3 cross-scope），拆 Phase G 符合约束。
- DDL 已在 Phase A 提交，Phase G 只需写 Go 代码，不碰 SQL。

**影响**
- STATE.md 更新：Phase F 执行接线 → 已通过；Phase G Audit → 待施工（6 子任务清单）。
- code-map.md §7 拆分 Phase F（done）和 Phase G（pending）。
- decisions.md 本条目（D-014）。

---

## D-015 · 审计日志格式修正：JSON Lines → Protobuf（2026-08-08）

**背景**
17-audit.md 初版设计审计 Logger 用 JSON Lines 格式（`encoding/json`，`json.Encoder`）。
用户指出这违反 `constraints.md §二 2.1`（审计日志 MUST 用 protobuf），且 JSON 本身有严谨性问题：
无 schema 校验、数字精度为 float64、字段类型漂移不报错。审计日志是归因校准的**数据源**——不严谨 = 校准不准 = 「准确无误」崩塌。

**决策**
1. **审计日志格式改为 protobuf 长度前缀**（`varint(len) + proto.Marshal(body)`），标准 streaming protobuf 格式。
2. **新增 `proto/audit/audit.proto`**：`AuditEvent` + `LegResult` + `OrderResult` + `EventType` 枚举。
   Decimal 字段走 string（与 dashboard proto 一致），时间用 `google.protobuf.Timestamp`。
3. **Logger 写文件用 `proto.Marshal` + `binary.PutUvarint` 长度前缀**（不用 `json.Encoder`）。
4. **人读方案**：`cat audit.pb | protoc --decode=arb.audit.AuditEvent proto/audit/audit.proto`（标准 protoc 管道），
   或可选的 Go 便利工具 `tools/readaudit/main.go`（~30 行，非必须）。
5. **不影响归因查询**：归因统计走 PG `opportunities` 表（SQL），不走 audit.pb 文件。
   audit.pb 是 append-only 不可篡改的完整事件流；PG 表是结构化查询层。

**理由**
- JSON 的"人能读"是以牺牲严谨性为代价——审计日志是归因校准数据源，不严谨不如不记。
- protobuf = 编译期 schema + 精确 decimal（string 字段）+ 向后兼容。
- `protoc --decode` 已覆盖"人读"需求，无需另造工具。
- constraints §二 2.1 原文："审计日志 (protobuf)"——初版设计偏离了约束，修正回正轨。

**影响**
- `docs/design/17-audit.md` 重写：§0 加"为什么必须是 protobuf"、§1 proto 定义、§2 Logger 改用长度前缀。
- STATE.md / code-map.md / practices.md 同步更新，去除所有 JSON Lines 引用。
- Phase G-1 任务范围扩大：加 `proto/audit/audit.proto`（新建 proto 文件 + buf generate）。
- decisions.md 本条目（D-015）。
