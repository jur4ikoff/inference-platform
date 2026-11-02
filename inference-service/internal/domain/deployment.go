package domain

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

var (
	ErrInvalidStatus           = errors.New("invalid deployment status")
	ErrDatabaseUnavailable     = errors.New("database is unavailable")
	ErrDeploymentAlreadyExists = errors.New("deployment already exists")
	ErrDeploymentNotFound      = errors.New("deployment not found")
	ErrInvalidDeploymentID     = errors.New("deployment_id must be a non-zero UUID")
	ErrInvalidDeploymentName   = errors.New("deployment name must not be empty")
	ErrInvalidPagination       = errors.New("invalid deployment pagination")
)

type Status string

const (
	StatusPending  Status = "PENDING"
	StatusStarting Status = "STARTING"
	StatusStarted  Status = "STARTED"
	StatusRunning  Status = "RUNNING"
	StatusStopping Status = "STOPPING"
	StatusStopped  Status = "STOPPED"
	StatusFailed   Status = "FAILED"
	StatusUnknown  Status = "UNKNOWN"
)

func (s Status) Validate() error {
	switch s {
	case StatusPending, StatusStarting, StatusStarted, StatusRunning,
		StatusStopping, StatusStopped, StatusFailed, StatusUnknown:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrInvalidStatus, s)
	}
}

func ParseStatus(value string) (Status, error) {
	status := Status(value)
	if err := status.Validate(); err != nil {
		return "", err
	}
	return status, nil
}

type ModelProviderType string

const ModelProviderTypeHuggingFace = "HUGGING_FACE"

type HuggingFaceProvider struct {
	Repo string `json:"repo"`
}

type DeploymentSpec struct {
	ProviderType        ModelProviderType    `json:"provider_type"`
	ProviderHuggingFace *HuggingFaceProvider `json:"provider_hugging_face,omitempty"`
}

type Deployment struct {
	ID             uuid.UUID
	ContainerID    string
	ModelID        uuid.UUID
	Project        string
	Username       string
	Name           string
	DeploymentName string
	Status         Status
	Spec           DeploymentSpec
}

type DeploymentState struct {
	ID             uuid.UUID
	ContainerID    string
	Status         Status
	Project        string
	DeploymentName string
	Name           string
}

func ValidDNSLabel(value string) bool {
	if len(value) == 0 || len(value) > 63 || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}
