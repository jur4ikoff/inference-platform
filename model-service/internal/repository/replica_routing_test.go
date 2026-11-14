package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jur4ikoff/model-service/internal/repository"
	"github.com/jur4ikoff/model-service/internal/testutil"
)

func TestReadReplicaRoutingKeepsWritesOnPrimary(t *testing.T) {
	primaryCalls, replicaCalls := 0, 0
	makeDB := func(calls *int) *testutil.MockDBTX {
		return &testutil.MockDBTX{
			QueryRowFunc: func(context.Context, string, ...any) pgx.Row { *calls++; return testutil.Row{Err: pgx.ErrNoRows} },
			QueryFunc:    func(context.Context, string, ...any) (pgx.Rows, error) { *calls++; return &testutil.Rows{}, nil },
			ExecFunc: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
				*calls++
				return pgconn.NewCommandTag("DELETE 0"), nil
			},
		}
	}
	repo := repository.NewModelRepository(makeDB(&primaryCalls), makeDB(&replicaCalls))
	ctx := repository.WithReplicaRead(context.Background())
	_, _ = repo.Get(ctx, "project", "c105b48b-17d0-4ae8-9a77-989d50654c8d")
	_, _ = repo.List(ctx, "project", 0, 10)
	_, _ = repo.Update(ctx, "project", "c105b48b-17d0-4ae8-9a77-989d50654c8d", nil, nil, nil, time.Now())
	_ = repo.Delete(ctx, "project", "c105b48b-17d0-4ae8-9a77-989d50654c8d")
	_, _ = repo.Get(context.Background(), "project", "c105b48b-17d0-4ae8-9a77-989d50654c8d")
	if primaryCalls != 3 || replicaCalls != 2 {
		t.Fatalf("primary calls=%d, replica calls=%d", primaryCalls, replicaCalls)
	}
}
