package digitaltwin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/karljucutan/distributed-digitaltwin-robot/internal/domain"
	"github.com/valkey-io/valkey-go"
)

var errTwinConflict = errors.New("digital twin was changed by another worker")

const (
	twinFieldVersion = "version"
	twinFieldData    = "data"
)

type twinStore struct {
	client valkey.Client
}

func newTwinStore(client valkey.Client) *twinStore {
	return &twinStore{client: client}
}

func twinKey(robotID int64) string {
	return "digitaltwin:robot:{" + strconv.FormatInt(robotID, 10) + "}"
}

// Get returns the stored twin, or an empty twin (Version 0) if none exists.
func (s *twinStore) Get(ctx context.Context, robotID int64) (domain.RobotTwin, error) {
	// B builds the HGET command; Build creates the command but does not send it.
	cmd := s.client.B().Hget().Key(twinKey(robotID)).Field(twinFieldData).Build()
	// Do sends the command and returns its reply. HGET returns the stored value,
	// so read it as bytes for json.Unmarshal below.
	data, err := s.client.Do(ctx, cmd).AsBytes()
	if valkey.IsValkeyNil(err) {
		return domain.RobotTwin{}, nil
	}
	if err != nil {
		return domain.RobotTwin{}, fmt.Errorf("read twin %d: %w", robotID, err)
	}

	var twin domain.RobotTwin
	if err := json.Unmarshal(data, &twin); err != nil {
		return domain.RobotTwin{}, fmt.Errorf("decode twin %d: %w", robotID, err)
	}
	return twin, nil
}

// saveTwinScript compares the stored version and writes the twin in one atomic
// step. Valkey runs a script without interleaving other commands, so no other
// worker can write between the check and the HSET.
//
// KEYS[1] = twin key
// ARGV[1] = expected version, ARGV[2] = new version, ARGV[3] = twin JSON
// Returns 1 if written, 0 on version conflict.
var saveTwinScript = valkey.NewLuaScript(`
local current = redis.call('HGET', KEYS[1], 'version') or '0'
if current ~= ARGV[1] then
  return 0
end
redis.call('HSET', KEYS[1], 'version', ARGV[2], 'data', ARGV[3])
return 1
`)

// Save writes the twin only if the stored version still equals expectedVersion.
// Otherwise it returns errTwinConflict. Like PostgreSQL's
// UPDATE ... WHERE version = expectedVersion, it checks and writes atomically.
func (s *twinStore) Save(ctx context.Context, twin domain.RobotTwin, expectedVersion uint64) error {
	data, err := encodeTwinForSave(twin, expectedVersion)
	if err != nil {
		return err
	}

	// Exec sends EVALSHA, falling back to EVAL if the server hasn't cached the script.
	written, err := saveTwinScript.Exec(ctx, s.client,
		[]string{twinKey(twin.RobotID)},
		[]string{
			strconv.FormatUint(expectedVersion, 10),
			strconv.FormatUint(twin.Version, 10),
			string(data),
		},
	).AsInt64()
	if err != nil {
		return fmt.Errorf("save twin %d: %w", twin.RobotID, err)
	}
	if written == 0 {
		// The caller's snapshot is stale; it must reload and retry.
		return errTwinConflict
	}
	return nil
}

// SaveWatchMultiExec is an alternative to Save, kept for reference. It provides
// the same guarantee using WATCH/MULTI/EXEC instead of a Lua script, which costs
// extra round trips and a dedicated connection.
// A plain version check followed by HSET has a race: another worker could write
// between those commands. WATCH makes EXEC reject the write if the key changes.
func (s *twinStore) SaveWatchMultiExec(ctx context.Context, twin domain.RobotTwin, expectedVersion uint64) error {
	data, err := encodeTwinForSave(twin, expectedVersion)
	if err != nil {
		return err
	}
	key := twinKey(twin.RobotID)

	// WATCH belongs to a connection, so use the same dedicated connection through
	// EXEC. This does not lock the key or prevent other workers from writing.
	return s.client.Dedicated(func(c valkey.DedicatedClient) (resultErr error) {
		watching := false
		defer func() {
			if !watching {
				return
			}
			// An early return before EXEC leaves WATCH active. Clear it before
			// returning the connection, even if the request context was canceled.
			cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := c.Do(cleanupCtx, c.B().Unwatch().Build()).Error(); err != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("unwatch twin %d: %w", twin.RobotID, err))
			}
		}()

		// Send WATCH before reading the version so any subsequent key change
		// invalidates our write. DoMulti batches commands; it is not a transaction.
		replies := c.DoMulti(ctx,
			c.B().Watch().Key(key).Build(),
			c.B().Hget().Key(key).Field(twinFieldVersion).Build(),
		)
		if err := replies[0].Error(); err != nil {
			return fmt.Errorf("watch twin %d: %w", twin.RobotID, err)
		}
		watching = true

		current, err := replies[1].AsUint64()
		if valkey.IsValkeyNil(err) {
			// No version field yet means this is the initial revision.
			current = 0
		} else if err != nil {
			return fmt.Errorf("read twin %d version: %w", twin.RobotID, err)
		}
		if current != expectedVersion {
			// The caller's snapshot is already stale; it must reload and retry.
			return errTwinConflict
		}

		// MULTI queues HSET instead of executing it immediately. EXEC writes both
		// hash fields only if the key has not changed since WATCH began.
		// This closes the race between the check above and the write below.
		replies = c.DoMulti(ctx,
			c.B().Multi().Build(),
			c.B().Hset().Key(key).FieldValue().
				FieldValue(twinFieldVersion, strconv.FormatUint(twin.Version, 10)).
				FieldValue(twinFieldData, string(data)).
				Build(),
			c.B().Exec().Build(),
		)

		// EXEC returns command results on success, or nil if a watched key changed.
		_, err = replies[2].ToArray()
		if err == nil || valkey.IsValkeyNil(err) {
			watching = false // EXEC ran, which clears WATCH.
		}
		if valkey.IsValkeyNil(err) {
			return errTwinConflict
		}
		if err != nil {
			return fmt.Errorf("save twin %d: %w", twin.RobotID, err)
		}
		return nil
	})
}

// encodeTwinForSave validates the version transition and encodes the twin as JSON.
func encodeTwinForSave(twin domain.RobotTwin, expectedVersion uint64) ([]byte, error) {
	// Version is the twin's optimistic-concurrency revision; prevent uint64
	// wraparound, which could make an old revision look current again.
	if expectedVersion == ^uint64(0) {
		return nil, fmt.Errorf("save twin %d: version overflow", twin.RobotID)
	}
	if twin.Version != expectedVersion+1 {
		return nil, fmt.Errorf("save twin %d: version must increment from %d to %d",
			twin.RobotID, expectedVersion, expectedVersion+1)
	}

	data, err := json.Marshal(twin)
	if err != nil {
		return nil, fmt.Errorf("encode twin %d: %w", twin.RobotID, err)
	}
	return data, nil
}
