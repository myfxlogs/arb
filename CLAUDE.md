# ARB — Claude Code 项目入口

> 本文件是 Claude Code 的项目入口。**约束、工作流、接手协议的唯一真相源是 `AGENTS.md`**，
> 下方通过 `@AGENTS.md` 内联进 system prompt。冲突时以 `AGENTS.md` 为准。

@AGENTS.md

> （兜底：若上方 AGENTS.md 内容未自动展开，立即用 Read 读取 `/opt/arb/AGENTS.md` —— 它是约束真相源。）

---

## Claude 专属补充

以下仅适用于 Claude Code，不进 SSOT（Windsurf 无对应机制）：

### 项目记忆定位（重要 — 无损接手的关键）
- **项目工作状态一律写入 `docs/handoff/STATE.md`**（git 跟踪，Claude 与 Windsurf 共享）。
- **不要把项目状态写进 `~/.claude/projects/-opt-arb/memory/`** —— Windsurf 读不到这个私有路径，会断接手。
- `~/.claude/.../memory/` 仅用于**跨项目的用户画像 / 工作风格**，不存 arb 项目状态。

### Claude 专属技能 `.claude/skills/`
- `mt-gateway`、`mt-accounts`、`stream-pattern`、`preflight-checks`、`debugging-symptom-to-root` 等是 **Claude 专属**能力（Windsurf 无此机制）。
- 涉及 MT4/MT5 proto 知识、实时流模式等时，主动用 Skill 调用。
- **交接给 Windsurf 时**：把这些 skill 里的相关知识显式写进 `docs/handoff/STATE.md` 或对话，否则 Windsurf 接手时丢失这部分上下文。
