# Distributed Digital Twin Robot

A local robot fleet simulator and an ingestion-focused digital twin backend.
The simulator publishes telemetry to NATS JetStream, where backend replicas
pull from a durable consumer and distribute messages across worker pools.
Workers update each robot's current state in shared Valkey, then fan in to a
bounded per-instance queue and batch writer for TimescaleDB. If database writes
lag, the queue fills and blocks workers, propagating backpressure upstream to
message consumption. JetStream limits outstanding unacknowledged deliveries
while retaining the backlog. Messages are acknowledged only after their
telemetry batch commits.

Each digital twin instance flushes its telemetry batch when it reaches
`TIMESCALEDB_FLUSH_BATCH_SIZE` or at `TIMESCALEDB_FLUSH_INTERVAL`. The Compose
configuration sets these to 5,000 messages and 5 seconds, respectively, and
sets `TIMESCALEDB_WRITE_QUEUE_SIZE` to 3,000 messages.

## Architecture

![High-level architecture diagram](./high-level-diagram.png)

### Design choices

- **Worker pool (competing consumers):** Each backend replica runs a pool of
  worker goroutines that share its NATS pull iterator. Multiple replicas pull
  from the same durable JetStream consumer, distributing messages among
  workers rather than broadcasting them. This enables parallel processing
  within and across replicas when runtime resources permit; the workers
  concurrently process separate messages.
- **End-to-end backpressure:** If TimescaleDB slows, its batch writer drains the
  bounded queue more slowly until workers block while enqueueing. Blocked
  workers stop draining the NATS client pull buffer. JetStream's
  `MaxAckPending` caps outstanding unacknowledged deliveries, while the durable
  stream retains the remaining backlog. The application propagates pressure
  upstream; JetStream provides the durable buffer and delivery limit.
- **Batched persistence:** A per-replica writer groups telemetry by batch size
  or time interval before writing to TimescaleDB.

### Reliability and delivery semantics

- **At-least-once delivery:** Valid messages are acknowledged after their
  TimescaleDB batch commits. Unacknowledged messages can be redelivered.
- **Event-time last-write-wins (LWW):** Workers compare telemetry timestamps
  and, when timestamps match, device sequences to avoid applying older or
  duplicate observations to the twin. This relies on synchronized clocks.
- **Optimistic concurrency:** A worker reads a twin and its version, applies
  telemetry, and asks Valkey to atomically save only if the version is still
  current. On a conflict, the worker reloads the twin, reapplies the telemetry,
  and retries.
- **Telemetry history:** Redelivery can create duplicate TimescaleDB records;
  state protection does not deduplicate history.

**Alternative:** Key-based partitioning by robot ID, for example using NATS
subject mapping, could support sequential processing of each robot's events
when intermediate transitions matter. Strict ordering would also require
coordinated partition ownership and careful handling of retries and redelivery;
it is not currently implemented.

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
