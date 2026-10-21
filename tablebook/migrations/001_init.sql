CREATE TABLE IF NOT EXISTS reservations (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    guest_name   TEXT    NOT NULL,
    party_size   INTEGER NOT NULL CHECK (party_size BETWEEN 1 AND 20),
    table_number INTEGER NOT NULL CHECK (table_number BETWEEN 1 AND 30),
    starts_at    INTEGER NOT NULL, -- unix seconds, UTC
    notes        TEXT    NOT NULL DEFAULT '',
    created_at   INTEGER NOT NULL  -- unix seconds, UTC
);

CREATE INDEX IF NOT EXISTS idx_reservations_table_starts
    ON reservations (table_number, starts_at);
