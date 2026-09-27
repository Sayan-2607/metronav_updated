-- MetroNav schema (idempotent; applied by the Go core service at start-up).
CREATE TABLE IF NOT EXISTS lines (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    color         TEXT NOT NULL,
    coaches       INT  NOT NULL,
    headway_min   DOUBLE PRECISION NOT NULL
);

CREATE TABLE IF NOT EXISTS stations (
    id                TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    weight            DOUBLE PRECISION NOT NULL,
    platform_capacity INT NOT NULL
);

CREATE TABLE IF NOT EXISTS line_stations (
    line_id    TEXT REFERENCES lines(id) ON DELETE CASCADE,
    station_id TEXT REFERENCES stations(id) ON DELETE CASCADE,
    seq        INT NOT NULL,
    PRIMARY KEY (line_id, seq)
);

CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    email         TEXT UNIQUE NOT NULL,
    name          TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL CHECK (role IN ('PASSENGER','OPERATOR','ANALYST','ADMIN')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tickets (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users(id),
    from_station TEXT NOT NULL,
    to_station   TEXT NOT NULL,
    profile      TEXT NOT NULL,
    coach        INT  NOT NULL DEFAULT 0,
    fare         INT  NOT NULL,
    route_json   TEXT NOT NULL,
    token        TEXT NOT NULL,
    status       TEXT NOT NULL CHECK (status IN ('ACTIVE','USED')),
    created_at   TIMESTAMPTZ NOT NULL,
    expires_at   TIMESTAMPTZ NOT NULL,
    used_at      TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS tickets_user_idx ON tickets(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS incidents (
    id         TEXT PRIMARY KEY,
    type       TEXT NOT NULL,
    severity   TEXT NOT NULL CHECK (severity IN ('LOW','MEDIUM','HIGH','CRITICAL')),
    station_id TEXT,
    line_id    TEXT,
    title      TEXT NOT NULL,
    detail     TEXT NOT NULL,
    source     TEXT NOT NULL,
    status     TEXT NOT NULL CHECK (status IN ('OPEN','ACKNOWLEDGED','RESOLVED')),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS incidents_created_idx ON incidents(created_at DESC);

CREATE TABLE IF NOT EXISTS occupancy_records (
    id         BIGSERIAL PRIMARY KEY,
    station_id TEXT NOT NULL,
    at         TIMESTAMPTZ NOT NULL,   -- simulated time
    density    DOUBLE PRECISION NOT NULL,
    expected   DOUBLE PRECISION NOT NULL,
    source     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS occupancy_station_at_idx ON occupancy_records(station_id, at);

CREATE TABLE IF NOT EXISTS audit_logs (
    id     BIGSERIAL PRIMARY KEY,
    at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor  TEXT NOT NULL,
    action TEXT NOT NULL,
    detail TEXT NOT NULL
);
