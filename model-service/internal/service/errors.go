package service

import "errors"

var (
	ErrInvalidInput       = errors.New("invalid input")
	ErrRuntimeUnavailable = errors.New("model runtime is unavailable")
	ErrAlreadyRunning     = errors.New("model is already running")
	ErrNotRunning         = errors.New("model is not running")
)
