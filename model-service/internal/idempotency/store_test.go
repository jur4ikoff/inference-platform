package idempotency

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestConcurrentReplayAcrossPools(t *testing.T) {
	dsn := os.Getenv("MODEL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set MODEL_TEST_DATABASE_URL to run PostgreSQL integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	root, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	schema := fmt.Sprintf("idempotency_test_%d", time.Now().UnixNano())
	if _, err := root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = root.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") }()
	if _, err := root.Exec(ctx, "CREATE TABLE "+schema+`.deployment_idempotency (
		owner_id TEXT NOT NULL, idempotency_key TEXT NOT NULL, project TEXT NOT NULL,
		model_id UUID NOT NULL, state TEXT NOT NULL, response JSONB, error_status INTEGER,
		PRIMARY KEY (owner_id, idempotency_key))`); err != nil {
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	config.MaxConns = 4
	first, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	stores := []*Store{New(first), New(second)}
	const model = "42a83c70-4ae9-4f53-935b-9631dc536f84"
	var calls atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			data, err := stores[i%len(stores)].Execute(ctx, "actor", "key", "project", model, func() ([]byte, int, error) {
				calls.Add(1)
				time.Sleep(50 * time.Millisecond)
				return []byte(`{"id":"same"}`), 0, nil
			})
			if err != nil || string(data) != `{"id": "same"}` && string(data) != `{"id":"same"}` {
				t.Errorf("replay: %s %v", data, err)
			}
		}(i)
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("operation ran %d times", calls.Load())
	}
	_, err = stores[1].Execute(ctx, "actor", "key", "different", model, func() ([]byte, int, error) { t.Error("conflicting request ran"); return nil, 0, nil })
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	if _, err := first.Exec(ctx, `INSERT INTO deployment_idempotency (owner_id,idempotency_key,project,model_id,state) VALUES ('actor','pending','project',$1,'pending')`, model); err != nil {
		t.Fatal(err)
	}
	_, err = stores[1].Execute(ctx, "actor", "pending", "project", model, func() ([]byte, int, error) { t.Error("uncertain request ran"); return nil, 0, nil })
	if !errors.Is(err, ErrPending) {
		t.Fatalf("expected pending, got %v", err)
	}
	operationErr := errors.New("runtime failed")
	_, err = stores[0].Execute(ctx, "actor", "failed", "project", model, func() ([]byte, int, error) { return nil, 503, operationErr })
	if !errors.Is(err, operationErr) {
		t.Fatalf("expected original failure, got %v", err)
	}
	_, err = stores[1].Execute(ctx, "actor", "failed", "project", model, func() ([]byte, int, error) { t.Error("failed request ran again"); return nil, 0, nil })
	var failure PreviousFailure
	if !errors.As(err, &failure) || failure.Status != 503 {
		t.Fatalf("expected recorded status 503, got %v", err)
	}
}
