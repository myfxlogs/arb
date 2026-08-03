-- ARB 全网套利系统：初始数据库迁移
-- PostgreSQL 15+
-- 运行：psql -f migrations/001_init.sql

BEGIN;

-- ============================================================
-- ticks：原始行情数据（按月分区，BRIN 索引，COPY 批量写入）
-- ============================================================
CREATE TABLE ticks (
    ts       TIMESTAMPTZ NOT NULL,
    broker   TEXT NOT NULL,
    symbol   TEXT NOT NULL,
    bid      NUMERIC NOT NULL,
    ask      NUMERIC NOT NULL
) PARTITION BY RANGE (ts);

-- 按月分区（施工 agent：代码中需每月初自动创建下月分区）
CREATE TABLE ticks_2026_08 PARTITION OF ticks
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');
CREATE TABLE ticks_2026_09 PARTITION OF ticks
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');
CREATE TABLE ticks_2026_10 PARTITION OF ticks
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');
CREATE TABLE ticks_2026_11 PARTITION OF ticks
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');
CREATE TABLE ticks_2026_12 PARTITION OF ticks
    FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');

-- BRIN 索引：时序数据的最佳选择，索引大小仅为 B-tree 的 1/1000
CREATE INDEX idx_ticks_ts_brin ON ticks USING BRIN (ts);
CREATE INDEX idx_ticks_broker_symbol_ts ON ticks (broker, symbol, ts);

-- ============================================================
-- signals：套利信号记录
-- ============================================================
CREATE TABLE signals (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ts         TIMESTAMPTZ NOT NULL DEFAULT now(),
    strategy   TEXT NOT NULL,
    legs       JSONB NOT NULL,
    gross_bps  NUMERIC NOT NULL,
    net_bps    NUMERIC NOT NULL,
    executed   BOOLEAN NOT NULL DEFAULT FALSE,
    dismissed  BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX idx_signals_ts ON signals (ts DESC);
CREATE INDEX idx_signals_strategy ON signals (strategy, ts DESC);

-- ============================================================
-- orders：订单记录（ClientID 是幂等键，UK 约束防重复）
-- ============================================================
CREATE TABLE orders (
    client_id   UUID PRIMARY KEY,
    ticket      BIGINT NOT NULL,
    broker      TEXT NOT NULL,
    symbol      TEXT NOT NULL,
    side        TEXT NOT NULL CHECK (side IN ('Buy', 'Sell')),
    volume      NUMERIC NOT NULL,
    open_price  NUMERIC,
    close_price NUMERIC,
    open_time   TIMESTAMPTZ,
    close_time  TIMESTAMPTZ,
    pnl         NUMERIC DEFAULT 0,
    commission  NUMERIC DEFAULT 0,
    swap        NUMERIC DEFAULT 0,
    signal_id   UUID REFERENCES signals(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_orders_ticket ON orders (ticket);
CREATE INDEX idx_orders_broker_symbol ON orders (broker, symbol);
CREATE INDEX idx_orders_open_time ON orders (open_time DESC);
CREATE INDEX idx_orders_signal_id ON orders (signal_id);

-- ============================================================
-- daily_summary：每日资金汇总
-- ============================================================
CREATE TABLE daily_summary (
    date         DATE NOT NULL,
    broker       TEXT NOT NULL,
    start_equity NUMERIC NOT NULL,
    end_equity   NUMERIC NOT NULL,
    pnl          NUMERIC NOT NULL DEFAULT 0,
    deposits     NUMERIC NOT NULL DEFAULT 0,
    withdrawals  NUMERIC NOT NULL DEFAULT 0,
    PRIMARY KEY (date, broker)
);

-- ============================================================
-- partition_mgmt：自动分区管理函数
-- ============================================================
CREATE OR REPLACE FUNCTION create_next_month_partition()
RETURNS void AS $$
DECLARE
    next_month_start DATE;
    next_month_end   DATE;
    partition_name   TEXT;
BEGIN
    next_month_start := date_trunc('month', now()) + INTERVAL '1 month';
    next_month_end   := next_month_start + INTERVAL '1 month';
    partition_name   := 'ticks_' || to_char(next_month_start, 'YYYY_MM');

    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS %I PARTITION OF ticks FOR VALUES FROM (%L) TO (%L)',
        partition_name, next_month_start, next_month_end
    );
END;
$$ LANGUAGE plpgsql;

-- 施工 agent：core 启动时需调用此函数
-- 施工 agent：需配置 pg_cron 或应用层定时（每月 1 号）调用：
-- SELECT create_next_month_partition();

COMMIT;
