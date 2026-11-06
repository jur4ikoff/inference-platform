package testutil

import (
	"fmt"
	"github.com/jackc/pgx/v5"
	"reflect"
)

// Row is a deterministic pgx.Row for repository tests.
type Row struct {
	Values []any
	Err    error
}

func (r Row) Scan(dest ...any) error {
	if r.Err != nil {
		return r.Err
	}
	if len(dest) != len(r.Values) {
		return fmt.Errorf("scan columns: got %d, want %d", len(dest), len(r.Values))
	}
	for i, value := range r.Values {
		v := reflect.ValueOf(dest[i])
		if v.Kind() != reflect.Pointer || v.IsNil() {
			return fmt.Errorf("column %d: invalid destination", i)
		}
		if value == nil {
			v.Elem().SetZero()
			continue
		}
		src := reflect.ValueOf(value)
		if !src.Type().AssignableTo(v.Elem().Type()) {
			return fmt.Errorf("column %d: %s cannot be assigned to %s", i, src.Type(), v.Elem().Type())
		}
		v.Elem().Set(src)
	}
	return nil
}

// Rows supports scan, iteration and errors used by sqlc-generated queries.
type Rows struct {
	pgx.Rows
	Data    [][]any
	Index   int
	ScanErr error
	IterErr error
	Closed  bool
}

func (r *Rows) Next() bool {
	if r.Index >= len(r.Data) {
		return false
	}
	r.Index++
	return true
}
func (r *Rows) Scan(dest ...any) error {
	if r.ScanErr != nil {
		return r.ScanErr
	}
	return (Row{Values: r.Data[r.Index-1]}).Scan(dest...)
}
func (r *Rows) Err() error { return r.IterErr }
func (r *Rows) Close()     { r.Closed = true }
