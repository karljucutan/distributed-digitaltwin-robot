-- +goose Up
CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE robot_telemetry (
    timestamp TIMESTAMPTZ NOT NULL,
    robot_id BIGINT NOT NULL,
    sequence BIGINT NOT NULL,
    joint_position_rad DOUBLE PRECISION[] NOT NULL,
    joint_velocity_rad_per_sec DOUBLE PRECISION[] NOT NULL,
    status TEXT NOT NULL,
    fault_code TEXT NOT NULL,
    PRIMARY KEY (robot_id, timestamp, sequence)
) WITH (
    tsdb.hypertable,
    tsdb.partition_column = 'timestamp'
    -- tsdb.chunk_interval = '1 day', default to 7 days
);

-- +goose Down
DROP TABLE robot_telemetry;
