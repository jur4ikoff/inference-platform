// Package idempotency persists the outcome of a deployment creation across retries and restarts.
package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrConflict = errors.New("idempotency key was used for a different request")
var ErrPending = errors.New("previous deployment creation has an uncertain outcome")

type PreviousFailure struct{ Status int }

func (e PreviousFailure) Error() string { return "previous deployment creation failed" }

type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Execute serializes identical keys across processes. A pending record survives
// a crash, preventing an uncertain operation from being executed a second time.
func (s *Store) Execute(ctx context.Context, owner, key, project, modelID string, run func() ([]byte, int, error)) ([]byte, error) {
	waitCtx, cancelWait := context.WithTimeout(ctx, 5*time.Second)
	defer cancelWait()
	conn, err := s.pool.Acquire(waitCtx)
	if err != nil {
		return nil, fmt.Errorf("acquire idempotency connection: %w", err)
	}
	checksum := sha256.Sum256([]byte(owner + "\x00" + key))
	lockID := int64(binary.BigEndian.Uint64(checksum[:8]))
	if _, err := conn.Exec(waitCtx, "SELECT pg_advisory_lock($1)", lockID); err != nil {
		conn.Release()
		return nil, fmt.Errorf("lock idempotency key: %w", err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := conn.Exec(unlockCtx, "SELECT pg_advisory_unlock($1)", lockID); err != nil {
			_ = conn.Hijack().Close(unlockCtx)
			return
		}
		conn.Release()
	}()
	result, err := conn.Exec(ctx, `INSERT INTO deployment_idempotency (owner_id, idempotency_key, project, model_id, state) VALUES ($1, $2, $3, $4::uuid, 'pending') ON CONFLICT DO NOTHING`, owner, key, project, modelID)
	if err != nil {
		return nil, fmt.Errorf("reserve idempotency key: %w", err)
	}
	if result.RowsAffected() == 0 {
		var savedProject, savedModel, state string
		var body []byte
		var errorStatus *int
		err = conn.QueryRow(ctx, `SELECT project, model_id::text, state, response, error_status FROM deployment_idempotency WHERE owner_id=$1 AND idempotency_key=$2`, owner, key).Scan(&savedProject, &savedModel, &state, &body, &errorStatus)
		if err != nil {
			return nil, fmt.Errorf("read idempotency result: %w", err)
		}
		if savedProject != project || savedModel != modelID {
			return nil, ErrConflict
		}
		switch state {
		case "completed":
			return body, nil
		case "failed":
			if errorStatus != nil {
				return nil, PreviousFailure{Status: *errorStatus}
			}
			return nil, PreviousFailure{Status: 500}
		default:
			return nil, ErrPending
		}
	}
	body, status, opErr := run()
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if opErr != nil {
		_, err = conn.Exec(finishCtx, `UPDATE deployment_idempotency SET state='failed', error_status=$3 WHERE owner_id=$1 AND idempotency_key=$2`, owner, key, status)
	} else {
		_, err = conn.Exec(finishCtx, `UPDATE deployment_idempotency SET state='completed', response=$3::jsonb WHERE owner_id=$1 AND idempotency_key=$2`, owner, key, string(body))
	}
	if err != nil {
		return nil, fmt.Errorf("save idempotency result: %w", err)
	}
	return body, opErr
}
