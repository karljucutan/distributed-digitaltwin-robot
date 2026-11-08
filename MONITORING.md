# Monitoring NATS and Podman

These commands assume NATS is reachable at `nats://localhost:4222` and the containers use the names shown below. Press `Ctrl+C` to stop a live command or a `watch` loop.

## NATS

### Stream storage

```sh
nats --server nats://localhost:4222 stream info ROBOT_TELEMETRY
```

Shows a snapshot of the `ROBOT_TELEMETRY` stream configuration and current state, including stored message count, bytes, and configured limits. It does not refresh by itself.

To refresh the snapshot every second:

```sh
watch -n 1 'nats --server nats://localhost:4222 stream info ROBOT_TELEMETRY'
```

Change `1` to another interval in seconds as needed. The stream is limited to 1 GiB and one hour; with its `Discard Old` policy, older messages are removed when a limit is reached.

### Consumer status

```sh
nats --server nats://localhost:4222 consumer info ROBOT_TELEMETRY digital-twin
```

Shows a point-in-time snapshot of the durable `digital-twin` consumer. Check `Unprocessed Messages` for messages not yet delivered and `Outstanding Acks` for delivered messages that have not yet been acknowledged.

To refresh it every second:

```sh
watch -n 1 'nats --server nats://localhost:4222 consumer info ROBOT_TELEMETRY digital-twin'
```

This reports consumer state; it does not display the messages being processed.

### Live subject subscription

```sh
nats --server nats://localhost:4222 sub 'telemetry.robot.*'
```

Live view of new messages published on subjects matching `telemetry.robot.*`. It starts receiving from the time you subscribe; it does not replay stored messages and is not the durable `digital-twin` consumer. The `*` matches one subject token, such as `state` in `telemetry.robot.state`.

### View messages retained in the stream

```sh
nats --server nats://localhost:4222 stream view ROBOT_TELEMETRY
```

Reads messages stored in the stream. This is for inspecting retained messages, not a live feed and not a view of which messages the application has pulled or acknowledged.

## Podman containers

The following `podman stats` commands continuously refresh resource usage every second. They show CPU, memory, network and block I/O, and process counts for the selected container.

```sh
podman stats -i 1 distributed-digitaltwin-robot-simulator-1
```

```sh
podman stats -i 1 distributed-digitaltwin-robot-digitaltwin-1
```

```sh
podman stats -i 1 distributed-digitaltwin-robot-digitaltwin-2
```

To follow timestamped logs from a container as new lines are written:

```sh
podman logs --timestamps -f distributed-digitaltwin-robot-simulator-1
podman logs --timestamps -f distributed-digitaltwin-robot-digitaltwin-1
podman logs --timestamps -f distributed-digitaltwin-robot-digitaltwin-2
```

Log timestamps indicate when the container wrote each line. They are distinct from the telemetry event timestamp in the message payload.