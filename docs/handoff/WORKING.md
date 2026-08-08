# 协作协议 — Claude ↔ Windsurf

> 这不是 AGENTS.md（那是 SSOT 法律条文）。这是"我们两个怎么配合"的操作手册。

---

## 1. 角色

| | Claude | Windsurf |
|---|---|---|
| **定位** | 总架构师 + 安全审计负责人 + 第一责任人 | 施工 agent |
| **决策权** | 设计决策、架构变更、技术选型 | 实现方式（在规格范围内） |
| **产出** | 设计文档、审查结论、架构决策 | 代码、测试、STATE.md 更新 |
| **对文档** | 定稿人 + 自审责任人（AGENTS §3.0） | 第二读者 / 实现反馈者 |

**关键原则**：Windsurf 遇到设计矛盾或文档不可实现时 → **上报 Claude，不自行改设计**（AGENTS §0 + §3.0）。

---

## 2. 一次典型的协作周期

```
1. Claude 写/更新设计文档（docs/design/XX-*.md）
2. Claude 更新 STATE.md「当前施工」+「下一步」
3. Windsurf 开工前读：
   a. AGENTS.md 全文（规则）
   b. STATE.md 全文（当前状态 +「当前施工」表格 +「阻塞」）
   c. practices.md（前车之鉴）
   d. 相关 docs/design/（规格）
4. Windsurf 施工：按「当前施工」表格逐子任务做
5. Windsurf 收工前：
   a. 更新「当前施工」表格（✅ + 🔄）
   b. 有阻塞写「阻塞 / 待决策」
   c. go build/vet/test/check-lines
   d. git commit（含 STATE.md）
6. Claude 复审（**先审交接、后审代码**）：
   a. 🔍 **交接合规检查**（不过 = 打回，不看代码）：
      - STATE.md「当前施工」表格是否更新？（子任务有 ✅/🔄？）
      - git log 最新 commit 是否包含 STATE.md？
      - git status 是否有未提交文件（= 中断信号）？
   b. 读 STATE.md「阻塞 / 待决策」→ 先回复 Windsurf 的问题
   c. 审代码（A–F）
   d. 更新 practices.md（新出现的模式）
   e. 更新 STATE.md 复审结论 + 下一步
```

---

## 3. 通信约定

### Windsurf → Claude（写在 STATE.md「阻塞 / 待决策」）

- "设计说 X，但代码现状是 Y，该按哪个？"
- "这个函数需要依赖 Z 层，按 code-map 不该依赖，怎么解？"
- "测试需要真实 PG/mtapi，本地没有，怎么处理？"

### Claude → Windsurf（写在 STATE.md 复审结论 +「阻塞」回复）

- 在「阻塞 / 待决策」下直接回复，标注 `[Claude]` 前缀。
- 审查发现的问题按严重度排（Critical → High → Medium → Info）。

### 不要在以下地方通信

- ❌ Git commit message（太分散，不醒目）
- ❌ 代码注释（会被删掉）
- ❌ `~/.claude/projects/`（Windsurf 读不到）

---

## 4. 中断与恢复

### Windsurf 要休息（或切换任务）

```
1. 当前子任务标 🔄，已完成的标 ✅
2. 有疑问写「阻塞 / 待决策」
3. git commit -m "wip: Phase G — G-1/G-2 done, G-3 in progress"
4. git push
```

### Claude 接手（或 Windsurf 回来继续）

```
1. 打开 STATE.md → 看「当前施工」表格 → 哪个 🔄、哪个 ✅
2. 看「阻塞 / 待决策」→ 有没有等我回复的
3. git status → 有没有未提交的半成品
4. 读 practices.md → 回忆最近的高频问题（可选但推荐）
```

---

## 6. 私有记忆的边界

Claude 有 `~/.claude/projects/` 记忆，Windsurf 有自己的记忆系统——**彼此读不到对方的私域**。
这是物理隔离，无法消除。我们的策略是：

**不试图同步记忆，而是同步产出。**

| 不应依赖私域的事 | 放哪里 |
|-----------------|--------|
| 项目当前状态 | `STATE.md`（共享） |
| 代码风格约定 | `practices.md`（共享） |
| 架构决策理由 | `decisions.md`（共享） |
| 高频错误模式 | `practices.md §1`（共享） |
| 思考方法 | `practices.md §4`（共享，外化为检查表） |
| 协作流程 | `WORKING.md`（共享，本文件） |

**原则**：任何对方需要知道的事，**必须落在 `docs/handoff/` 的某份文件里**。
不依赖对方"记得"什么。私域记忆只放与自己内部工作相关的事（用户偏好、工具配置等），
不放项目状态。

Claude 的 `~/.claude/projects/` 里只有跨项目的用户画像——
项目相关信息全在 `docs/handoff/` 里。Windsurf 同理：项目上下文放 git 跟踪的共享文件，
不要只存自己的私域。

---

## 5. 分歧处理

1. Windsurf 认为设计有更好的实现方式 → 在「阻塞」里写"I think X would be simpler because Y"，继续按原设计施工（不要停下来等回复）。
2. Windsurf 认为设计有错误/不可实现 → 停在这个子任务，标 ⛔，写「阻塞」，等 Claude 回复。**不自行改设计。**
3. Claude 复审发现设计问题 → 更新设计文档 → STATE.md 标注"需返工"→ Windsurf 下一轮修。

---

> 最后更新：2026-08-08
