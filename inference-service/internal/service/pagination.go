package service

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"github.com/jur4ikoff/inference-service/internal/domain"
)

type ListDeploymentsInput struct {
	Offset        int
	Limit         int
	NextPageToken string
}

type DeploymentsPage struct {
	Deployments   []domain.Deployment
	NextPageToken string
}

func (s *DeploymentService) ListDeployments(ctx context.Context, input ListDeploymentsInput) (DeploymentsPage, error) {
	return s.listDeployments(ctx, input, false)
}

func (s *DeploymentService) ListAllDeployments(ctx context.Context, input ListDeploymentsInput) (DeploymentsPage, error) {
	return s.listDeployments(ctx, input, true)
}

func (s *DeploymentService) listDeployments(ctx context.Context, input ListDeploymentsInput, all bool) (DeploymentsPage, error) {
	if input.Limit == 0 {
		input.Limit = 20
	}
	if input.Limit < 1 || input.Limit > 100 {
		return DeploymentsPage{}, fmt.Errorf("%w: limit must be between 1 and 100", domain.ErrInvalidPagination)
	}
	if input.NextPageToken != "" {
		offset, err := strconv.Atoi(input.NextPageToken)
		if err != nil || offset < 0 {
			return DeploymentsPage{}, fmt.Errorf("%w: next_page_token must be a non-negative 32-bit integer", domain.ErrInvalidPagination)
		}
		input.Offset = offset
	}
	if input.Offset < 0 {
		return DeploymentsPage{}, fmt.Errorf("%w: offset must be non-negative", domain.ErrInvalidPagination)
	}
	if input.Offset > math.MaxInt32-input.Limit {
		return DeploymentsPage{}, fmt.Errorf("%w: offset is too large", domain.ErrInvalidPagination)
	}
	if s.repo == nil {
		return DeploymentsPage{}, domain.ErrDatabaseUnavailable
	}
	// Fetch an extra row to distinguish the last page without a separate count.
	var deployments []domain.Deployment
	var err error
	if all {
		deployments, err = s.repo.ListAllDeployments(ctx, input.Offset, input.Limit+1)
	} else {
		deployments, err = s.repo.ListDeployments(ctx, input.Offset, input.Limit+1)
	}
	if err != nil {
		return DeploymentsPage{}, err
	}
	page := DeploymentsPage{Deployments: deployments}
	if len(deployments) > input.Limit {
		page.Deployments = deployments[:input.Limit]
		page.NextPageToken = strconv.Itoa(input.Offset + input.Limit)
	}
	return page, nil
}
