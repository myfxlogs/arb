-- ARB 新架构机会闭环 schema（PostgreSQL 15+）
-- 运行：psql -f migrations/003_opportunity.sql
--
-- 依据：
--   - docs/design/02-opportunity.md（Opportunity 对象 / 成本模型 / Leg）
--   - docs/design/07-risk-audit.md（归因记账 / 审计 / P1 自适应数据源）
--   - docs/design/04-human-in-loop.md（Opportunity 状态机）
-- 关联：
--   - migrations/001_init.sql（orders / ticks / daily_summary 基础表）
--   - migrations/002_symbol_map.sql（symbol_map：broker_symbol → canonical，本表不重复）
--
-- 注意：本迁移原任务文本记为 002_opportunity.sql，但 002 已被 symbol_map 占用，
-- 故按依赖顺序用 003（opportunities 引用 symbol_map 的 canonical 概念，002 须先于 003 执行）。

BEGIN;

-- ============================================================
-- opportunities：机会生命周期 + 预估 vs 实际归因（02 §5 / 07 §4）
--
-- 存每个机会从 Pushed 到 Filled/Failed/Expired 的完整记录：
--   - 预估字段（Evaluator 算，02 §6）
--   - 成本拆解（公理③，02 §4）
--   - 实际成交回填（pipeline 成交后写，归因数据源）
--   - 偏差（actual − estimate，校准 Evaluator，07 §4 自适应闭环）
-- ============================================================

CREATE TABLE IF NOT EXISTS opportunities (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- 分类 / 状态（02 §5 / 04 §2 状态机）
    type            TEXT NOT NULL CHECK (type IN ('CROSS_EXCHANGE','CARRY','TRIANGULAR')),
    status          TEXT NOT NULL CHECK (status IN
                        ('PUSHED','CONFIRMED','EXECUTING','FILLED','FAILED','EXPIRED')),

    -- 腿（02 §5 Leg）：broker / broker_symbol / canonical_symbol / direction / lots / estimate_price
    -- JSONB 存全腿快照（与 proto 的 repeated Leg 一一对应；下单后腿的实际成交价也回填在此 JSONB）
    legs            JSONB NOT NULL,

    -- 预估成本拆解（warm/cold path，NUMERIC(20,8)，constraints §四 4.2）
    gross_profit    NUMERIC(20,8) NOT NULL,
    spread_cost     NUMERIC(20,8) NOT NULL DEFAULT 0,
    commission_cost NUMERIC(20,8) NOT NULL DEFAULT 0,
    slippage_cost   NUMERIC(20,8) NOT NULL DEFAULT 0,
    swap_cost       NUMERIC(20,8) NOT NULL DEFAULT 0,

    -- 净盈利 / 统一度量（02 §4.1 / §3）
    net_profit      NUMERIC(20,8) NOT NULL,   -- = gross − 上述全部成本
    net_bps         NUMERIC(20,8) NOT NULL,   -- 跨机会排序用（统一度量）

    -- 时间（公理④新鲜度）
    quote_time      TIMESTAMPTZ NOT NULL,     -- 价格采样时刻（Evaluator 评估基准）
    expires_at      TIMESTAMPTZ NOT NULL,     -- 报价有效期（Pushed 阶段倒计时基准）

    -- 准确性（02 §6）
    confidence      REAL NOT NULL DEFAULT 0,  -- P1 占位，归因校准（07 §4）

    -- ===== 实际成交回填（pipeline 成交后写；预估为空直到 Filled/Failed）=====
    exec_actual_fill_price  NUMERIC(20,8),    -- 实际成交均价（各腿加权）
    exec_actual_swap        NUMERIC(20,8),    -- Order.Swap 累计（成交后真实值）
    exec_actual_commission  NUMERIC(20,8),    -- Order.Commission 累计
    exec_actual_slippage    NUMERIC(20,8),    -- 实测滑点 = 估价 − 成交价
    exec_actual_net_profit  NUMERIC(20,8),    -- 实际净盈利（回填后与 net_profit 对比）
    exec_filled_at          TIMESTAMPTZ,      -- 全腿成交 / 失败对冲完成时刻

    -- ===== 偏差（actual − estimate；归因校准 Evaluator 的滑点/swap/commission 预估）=====
    deviation_slippage_bps  NUMERIC(20,8),    -- 实际滑点 − 预估滑点（→ 机会阈值，讨论五①）
    deviation_swap          NUMERIC(20,8),    -- 实际 swap − 预估 swap（→ swap 成本模型）
    deviation_commission    NUMERIC(20,8),    -- 实际 commission − 预估 commission

    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 状态 + 时间查询（desk 历史 Tab / 归因复盘）
CREATE INDEX IF NOT EXISTS idx_opportunities_status_created
    ON opportunities (status, created_at DESC);
-- 按类型/时间统计（归因：哪类机会预估偏差大）
CREATE INDEX IF NOT EXISTS idx_opportunities_type_quote
    ON opportunities (type, quote_time DESC);
-- Expire 扫描（core 后台清理 Pushed 但超 expires_at 的机会）
CREATE INDEX IF NOT EXISTS idx_opportunities_expires
    ON opportunities (expires_at)
    WHERE status IN ('PUSHED','CONFIRMED');

-- updated_at 自动维护
CREATE OR REPLACE FUNCTION touch_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_opportunities_touch ON opportunities;
CREATE TRIGGER trg_opportunities_touch
    BEFORE UPDATE ON opportunities
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- ============================================================
-- symbol_map：已在 002_symbol_map.sql 创建（broker, broker_symbol → canonical）
-- 本迁移不重复定义；opportunities.legs JSONB 内的 canonical_symbol 字段
-- 与 symbol_map.canonical_symbol 语义一致（02 §2）。
-- ============================================================

-- ============================================================
-- audit_events：结构化审计（07 §3）
--
-- 设计说明（07 §3）：
--   - 审计 PRIMARY 存储为 protobuf 文件（constraints §二 2.1 允许的本地文件）。
--   - 本表为「可选的 queryable 索引层」，存关键事件的结构化摘要，供 desk 历史 Tab
--     查询与归因复盘（PG 可索引/可 JOIN，protobuf 文件不可）。
--   - 现有 store/audit.go 的 audit_log 表（简单事件流，EnsureAuditTable 自建）
--     可由本表替代（更丰富字段）或并存；落地时择一，避免双写歧义。
-- ============================================================

CREATE TABLE IF NOT EXISTS audit_events (
    id              BIGSERIAL PRIMARY KEY,
    ts              TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- 关联实体（二者至少填一）
    opportunity_id  UUID REFERENCES opportunities(id),
    order_client_id UUID,                  -- 关联 orders.client_id（001_init.sql 定义）

    -- 事件分类（07 §3：每个 Opportunity / Order 事件落审计）
    event_type      TEXT NOT NULL,         -- OPPORTUNITY_PUSHED / CONFIRMED / LEG_FILLED /
                                           -- LEG_FAILED / HEDGED / KILLED / CB_TRIGGERED / ...
    broker          TEXT,                  -- 涉及 broker（可空，全局事件无）

    -- 结构化 payload（与 protobuf 审计文件对齐，JSONB 兼容）
    detail          JSONB,

    -- 全序保证（跨 broker / 跨事件类型可排序）
    sequence_num    BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_events_ts        ON audit_events (ts DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_opp       ON audit_events (opportunity_id, ts);
CREATE INDEX IF NOT EXISTS idx_audit_events_type_ts   ON audit_events (event_type, ts DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_order     ON audit_events (order_client_id);

COMMIT;
