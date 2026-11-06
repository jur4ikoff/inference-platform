package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

type ProviderType string

const (
	maxRepositoryLength              = 512
	maxModelNameLength               = 63
	ProviderHuggingFace ProviderType = "HUGGING_FACE"
)

var (
	ErrInvalidModelSpec = errors.New("invalid model specification")
	modelNamePattern    = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
)

type ModelSpec struct {
	ProviderType        ProviderType         `json:"provider_type"`
	ProviderHuggingFace *HuggingFaceProvider `json:"provider_hugging_face"`
}

type HuggingFaceProvider struct {
	Repo string `json:"repo"`
}

func ParseModelSpec(raw json.RawMessage) (ModelSpec, json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ModelSpec{}, nil, ErrInvalidModelSpec
	}

	var spec ModelSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return ModelSpec{}, nil, err
	}
	canonical, err := spec.CanonicalJSON()
	if err != nil {
		return ModelSpec{}, nil, err
	}
	return spec, canonical, nil
}

func (s *ModelSpec) UnmarshalJSON(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	type wireModelSpec ModelSpec
	var decoded wireModelSpec
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidModelSpec, err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ErrInvalidModelSpec
	}
	value := ModelSpec(decoded)
	if err := value.Validate(); err != nil {
		return err
	}
	*s = value
	return nil
}

func (s ModelSpec) CanonicalJSON() (json.RawMessage, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("marshal model spec: %w", err)
	}
	return json.RawMessage(canonical), nil
}

func (s *ModelSpec) Validate() error {
	if s == nil || s.ProviderType != ProviderHuggingFace || s.ProviderHuggingFace == nil {
		return ErrInvalidModelSpec
	}
	if err := s.ProviderHuggingFace.Validate(); err != nil {
		return err
	}
	return nil
}

func (p *HuggingFaceProvider) Validate() error {
	if p == nil {
		return ErrInvalidModelSpec
	}
	repo := strings.TrimSpace(p.Repo)
	if repo == "" || len(repo) > maxRepositoryLength || strings.ContainsAny(repo, "\t\r\n ") || strings.Contains(repo, "://") || strings.Contains(repo, "@") {
		return ErrInvalidModelSpec
	}
	p.Repo = repo
	return nil
}

func ValidModelName(value string) bool {
	return len(value) > 0 && len(value) <= maxModelNameLength && modelNamePattern.MatchString(value)
}
