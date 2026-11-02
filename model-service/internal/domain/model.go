package domain

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrModelNotFound      = errors.New("model not found")
	ErrModelAlreadyExists = errors.New("model name already exists in project")
)

type Model struct {
	ID          string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Name        string
	Description string
	UserID      string
	Project     string
	Spec        json.RawMessage
}

type ModelState struct {
	ModelID string
	Project string
	Name    string
	Status  string
}
