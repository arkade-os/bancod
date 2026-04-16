CREATE TABLE IF NOT EXISTS banco_pair (
    pair TEXT PRIMARY KEY,
    min_amount INTEGER NOT NULL,
    max_amount INTEGER NOT NULL,
    base_decimals INTEGER NOT NULL DEFAULT 0,
    quote_decimals INTEGER NOT NULL DEFAULT 0,
    price_feed TEXT NOT NULL,
    invert_price INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS trade (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pair TEXT NOT NULL,
    deposit_asset TEXT NOT NULL,
    deposit_amount INTEGER NOT NULL,
    want_asset TEXT NOT NULL,
    want_amount INTEGER NOT NULL,
    offer_txid TEXT NOT NULL,
    fulfill_txid TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_trade_created_at ON trade (created_at DESC);
