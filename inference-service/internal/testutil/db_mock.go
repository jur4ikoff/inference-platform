package testutil

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	dao "github.com/jur4ikoff/inference-service/internal/infrastructure/dao"
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
