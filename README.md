# Distributed Digital Twin Robot

A local robot fleet simulator and digital twin backend. The simulator publishes
telemetry to NATS JetStream; digital twin workers consume it, maintain current
robot state in Valkey, and write telemetry history to TimescaleDB in batches.

Each digital twin instance flushes its telemetry batch when it reaches
`TIMESCALEDB_FLUSH_BATCH_SIZE` or at `TIMESCALEDB_FLUSH_INTERVAL`. The Compose
configuration sets these to 5,000 messages and 5 seconds, respectively, and
sets `TIMESCALEDB_WRITE_QUEUE_SIZE` to 3,000 messages.

## Architecture

![High-level architecture diagram](./high-level-diagram.png)

## Prerequisites

- GNU Make, Podman, and a working Compose provider (`podman compose`).
- [Goose](https://pressly.github.io/goose/installation/) installed on the host.
- A TimescaleDB image supporting `CREATE TABLE ... WITH (tsdb.hypertable)`
  (TimescaleDB 2.20 or newer).

## Run locally

Run these commands from the repository root.

1. Create `backend/.env` for Goose, replacing `YOUR_LOCAL_PASSWORD` with the
   password configured in `compose.yaml`:

   ```dotenv
   GOOSE_DRIVER=postgres
   GOOSE_DBSTRING=postgres://tsdbadmin:YOUR_LOCAL_PASSWORD@localhost:5432/digitaltwin?sslmode=disable
   GOOSE_MIGRATION_DIR=db/digitaltwin/migrations
   ```

   This file is Git-ignored. `localhost` is for Goose running on the host;
   containers use the `timescaledb` service name.

2. Build and start the services, then apply migrations:

   ```sh
   make up
   (cd backend && goose up)
   ```

   `make up` does not run migrations itself. On later starts, Goose applies only
   migrations that have not yet been applied.

   If an existing volume was initialized with a different database name,
   changing `POSTGRES_DB` does not create a new database. Create `digitaltwin`
   separately or update the configuration to use the existing database; do not
   delete the volume if you need its data.

3. After migrations succeed, trigger a simulation at `http://localhost:8080`:

   ```sh
   curl -X POST http://localhost:8080/api/simulation \
     -H 'Content-Type: application/json' \
     -d '{"robot_count": 10, "ttl_seconds": 30}'
   ```

See the [Makefile](Makefile) for available commands and
[MONITORING.md](MONITORING.md) for NATS and container monitoring.
