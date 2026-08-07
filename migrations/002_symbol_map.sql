-- symbol_map: broker-specific symbol → canonical symbol mapping (02 §2)
-- Manually maintained. Used for cross-broker opportunity detection.
-- Raw BrokerSymbol is still used as-is for order placement.

BEGIN;

CREATE TABLE IF NOT EXISTS symbol_map (
    broker           TEXT NOT NULL,
    broker_symbol    TEXT NOT NULL,
    canonical_symbol TEXT NOT NULL,
    PRIMARY KEY (broker, broker_symbol)
);

CREATE INDEX idx_symbol_map_broker_canonical ON symbol_map (broker, canonical_symbol);

COMMIT;
