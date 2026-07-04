-- v1: event store + read-side tables for booking reference arch
-- Postgres target; in-memory impl mirrors these invariants.

CREATE TABLE IF NOT EXISTS event_streams (
    stream_id   TEXT        NOT NULL,
    version     BIGINT      NOT NULL,
    event_id    TEXT        NOT NULL,
    event_type  TEXT        NOT NULL,
    payload     JSONB       NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (stream_id, version),
    UNIQUE (event_id)
);

-- inventory is separate BC: many bookings fight over same night slot
CREATE TABLE IF NOT EXISTS inventory_slots (
    property_id TEXT    NOT NULL,
    room_type   TEXT    NOT NULL,
    slot_date   DATE    NOT NULL,
    available   INT     NOT NULL CHECK (available >= 0),
    reserved    INT     NOT NULL CHECK (reserved >= 0),
    version     BIGINT  NOT NULL DEFAULT 1,
    PRIMARY KEY (property_id, room_type, slot_date)
);

CREATE TABLE IF NOT EXISTS reservation_holds (
    booking_id  TEXT NOT NULL,
    property_id TEXT NOT NULL,
    room_type   TEXT NOT NULL,
    slot_date   DATE NOT NULL,
    qty         INT  NOT NULL,
    slot_version BIGINT NOT NULL,
    PRIMARY KEY (booking_id, slot_date)
);

-- saga checkpoint — resume cancel after crash without double-void
CREATE TABLE IF NOT EXISTS saga_instances (
    saga_id     TEXT PRIMARY KEY,
    booking_id  TEXT NOT NULL,
    saga_type   TEXT NOT NULL,
    state       TEXT NOT NULL,
    steps       JSONB NOT NULL DEFAULT '[]',
    started_at  TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_saga_by_booking ON saga_instances (booking_id);
