package service

import (
	"context"
	"math"
	"strings"

	"github.com/jur4ikoff/model-service/internal/domain"
)

type ListModelsInput struct {
	Project string
	Offset  int
	Limit   int
}

type ModelsPage struct {
	Models        []domain.Model
	NextPageToken *int
}

func (s *ModelService) List(ctx context.Context, input ListModelsInput) (ModelsPage, error) {
	input.Project = strings.TrimSpace(input.Project)
	if input.Project == "" {
		return ModelsPage{}, ErrInvalidInput
	}
	if input.Limit == 0 {
		input.Limit = 20
	}
	if input.Offset < 0 || input.Limit < 1 || input.Limit > 100 || input.Offset > math.MaxInt-input.Limit {
		return ModelsPage{}, ErrInvalidInput
	}
	// Fetch one extra row to detect the next page without counting all models.
	models, err := s.models.List(ctx, input.Project, input.Offset, input.Limit+1)
	if err != nil {
		return ModelsPage{}, err
	}
	page := ModelsPage{}
	if len(models) > input.Limit {
		nextOffset := input.Offset + input.Limit
		page.NextPageToken = &nextOffset
		models = models[:input.Limit]
	}
	page.Models = make([]domain.Model, len(models))
	copy(page.Models, models)
	return page, nil
}
