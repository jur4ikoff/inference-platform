package service_test

import (
	"context"
	"errors"
	"github.com/jur4ikoff/inference-service/internal/config"
	"github.com/jur4ikoff/inference-service/internal/domain"
	"github.com/jur4ikoff/inference-service/internal/service"
	"github.com/jur4ikoff/inference-service/internal/testutil"
	"testing"
)

// Техника: граничные значения пагинации и переход на следующую страницу.
func TestDeploymentPagination(t *testing.T) {
	for _, all := range []bool{false, true} {
		for _, valid := range []bool{true, false} {
			t.Run(map[bool]string{false: "active", true: "history"}[all]+map[bool]string{true: "/valid", false: "/invalid"}[valid], func(t *testing.T) {
				d := testutil.DeploymentMother{}.Pending()
				list := func(context.Context, int, int) ([]domain.Deployment, error) { return []domain.Deployment{d, d}, nil }
				svc := service.NewDeploymentService(&testutil.MockDeploymentRepository{ListDeploymentsFunc: list, ListAllDeploymentsFunc: list}, config.ServiceConfig{}, nil)
				call := svc.ListDeployments
				if all {
					call = svc.ListAllDeployments
				}
				limit := 1
				if !valid {
					limit = 101
				}
				page, err := call(context.Background(), service.ListDeploymentsInput{Limit: limit})
				if valid && (err != nil || len(page.Deployments) != 1 || page.NextPageToken != "1") {
					t.Fatalf("page=%+v err=%v", page, err)
				}
				if !valid && !errors.Is(err, domain.ErrInvalidPagination) {
					t.Fatalf("err=%v", err)
				}
			})
		}
	}
}
