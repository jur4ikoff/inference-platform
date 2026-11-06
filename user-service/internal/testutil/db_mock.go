package testutil

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	dao "github.com/jur4ikoff/user-service/internal/infrastructure/dao"
)

type MockDBTX struct {
	ExecFunc     func(context.Context, string, ...interface{}) (pgconn.CommandTag, error)
	QueryFunc    func(context.Context, string, ...interface{}) (pgx.Rows, error)
	QueryRowFunc func(context.Context, string, ...interface{}) pgx.Row
}

func (m *MockDBTX) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return m.ExecFunc(ctx, sql, args...)
}
func (m *MockDBTX) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return m.QueryFunc(ctx, sql, args...)
}
func (m *MockDBTX) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return m.QueryRowFunc(ctx, sql, args...)
}

var _ dao.DBTX = (*MockDBTX)(nil)

type MockTx struct {
	pgx.Tx
	MockDBTX
	CommitFunc   func(context.Context) error
	RollbackFunc func(context.Context) error
}

func (m *MockTx) Commit(ctx context.Context) error   { return m.CommitFunc(ctx) }
func (m *MockTx) Rollback(ctx context.Context) error { return m.RollbackFunc(ctx) }

type MockDatabase struct {
	MockDBTX
	BeginFunc func(context.Context) (pgx.Tx, error)
}

func (m *MockDatabase) Begin(ctx context.Context) (pgx.Tx, error) { return m.BeginFunc(ctx) }
func (m *MockTx) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return m.MockDBTX.Exec(ctx, sql, args...)
}
func (m *MockTx) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return m.MockDBTX.Query(ctx, sql, args...)
}
func (m *MockTx) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return m.MockDBTX.QueryRow(ctx, sql, args...)
}

var _ pgx.Tx = (*MockTx)(nil)
