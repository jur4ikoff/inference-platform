package domain

import (
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
	Username    string
	Project     string
	Spec        ModelSpec
}

type ModelState struct {
	ModelID string
	Project string
	Name    string
	Status  string
}

// Deployment is the runtime-owned representation returned to the API. Model
// Service deliberately keeps no copy of it: InferenceService is the source of
// truth for a deployment and its lifecycle state.
type Deployment struct {
	ID        string
	ModelID   string
	Project   string
	Name      string
	Username  string
	Status    string
	StartedAt *time.Time
	UpdatedAt *time.Time
}
