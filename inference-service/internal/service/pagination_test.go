package service_test

import (
	"errors"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/google/uuid"
	"github.com/jur4ikoff/inference-service/internal/config"
	"github.com/jur4ikoff/inference-service/internal/domain"
	"github.com/jur4ikoff/inference-service/internal/service"
	"github.com/jur4ikoff/inference-service/internal/testutil"
	"go.uber.org/mock/gomock"
)

// Техника: анализ граничных значений; страницы из одного элемента при наличии следующего.
func TestListDeploymentsReturnsNextPageToken(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		d := testutil.DeploymentMother{}.Pending()
		ctrl := gomock.NewController(allureCtx)
		defer ctrl.Finish()
		repo := NewMockDeploymentRepository(ctrl)
		repo.EXPECT().ListDeployments(gomock.Any(), 0, 2).Return([]domain.Deployment{d, d}, nil)
		svc := service.NewDeploymentService(repo, config.ServiceConfig{}, nil)

		page, err := svc.ListDeployments(t.Context(), service.ListDeploymentsInput{Limit: 1})
		if err != nil || len(page.Deployments) != 1 || page.Deployments[0].ID != d.ID || page.NextPageToken != "1" {
			allureCtx.Fatalf("page=%+v err=%v", page, err)
		}
	})
}

// Техника: анализ граничных значений; limit выше разрешённого максимума.
func TestListDeploymentsRejectsInvalidLimit(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		ctrl := gomock.NewController(allureCtx)
		defer ctrl.Finish()
		repo := NewMockDeploymentRepository(ctrl)
		svc := service.NewDeploymentService(repo, config.ServiceConfig{}, nil)
		_, err := svc.ListDeployments(t.Context(), service.ListDeploymentsInput{Limit: 101})
		if !errors.Is(err, domain.ErrInvalidPagination) {
			allureCtx.Fatalf("err=%v", err)
		}
	})
}

// Техника: анализ граничных значений; страницы из одного элемента при наличии следующего.
func TestListAllDeploymentsReturnsNextPageToken(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		running := testutil.DeploymentMother{}.Running()
		stopped := testutil.NewDeploymentBuilder().WithID(uuid.MustParse("0197eb5a-37c6-7e48-8cd6-0c19edcd72f3")).WithStatus(domain.StatusStopped).Build()
		ctrl := gomock.NewController(allureCtx)
		defer ctrl.Finish()
		repo := NewMockDeploymentRepository(ctrl)
		repo.EXPECT().ListAllDeployments(gomock.Any(), 0, 2).Return([]domain.Deployment{running, stopped}, nil)
		svc := service.NewDeploymentService(repo, config.ServiceConfig{}, nil)

		page, err := svc.ListAllDeployments(t.Context(), service.ListDeploymentsInput{Limit: 1})
		if err != nil || len(page.Deployments) != 1 || page.Deployments[0].ID != running.ID || page.NextPageToken != "1" {
			allureCtx.Fatalf("page=%+v err=%v", page, err)
		}
	})
}

// Техника: анализ граничных значений; запрос следующей страницы истории с завершённым deployment.
func TestListAllDeployments(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		stopped := testutil.DeploymentMother{}.Stopped()
		ctrl := gomock.NewController(allureCtx)
		defer ctrl.Finish()
		repo := NewMockDeploymentRepository(ctrl)
		repo.EXPECT().ListAllDeployments(gomock.Any(), 1, 2).Return([]domain.Deployment{stopped}, nil)
		svc := service.NewDeploymentService(repo, config.ServiceConfig{}, nil)

		page, err := svc.ListAllDeployments(t.Context(), service.ListDeploymentsInput{Limit: 1, NextPageToken: "1"})
		if err != nil || len(page.Deployments) != 1 || page.Deployments[0].Status != domain.StatusStopped || page.NextPageToken != "" {
			allureCtx.Fatalf("page=%+v err=%v", page, err)
		}
	})
}

// Техника: анализ граничных значений; limit выше разрешённого максимума.
func TestListAllDeploymentsRejectsInvalidLimit(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		ctrl := gomock.NewController(allureCtx)
		defer ctrl.Finish()
		repo := NewMockDeploymentRepository(ctrl)
		svc := service.NewDeploymentService(repo, config.ServiceConfig{}, nil)
		_, err := svc.ListAllDeployments(t.Context(), service.ListDeploymentsInput{Limit: 101})
		if !errors.Is(err, domain.ErrInvalidPagination) {
			allureCtx.Fatalf("err=%v", err)
		}
	})
}

// Техника: разбиение на классы эквивалентности; случай ошибки внешнего репозитория.
func TestListAllDeploymentsReportsRepositoryError(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		failure := errors.New("database unavailable")
		ctrl := gomock.NewController(allureCtx)
		defer ctrl.Finish()
		repo := NewMockDeploymentRepository(ctrl)
		repo.EXPECT().ListAllDeployments(gomock.Any(), 0, 21).Return(nil, failure)
		svc := service.NewDeploymentService(repo, config.ServiceConfig{}, nil)

		page, err := svc.ListAllDeployments(t.Context(), service.ListDeploymentsInput{})
		if !errors.Is(err, failure) || len(page.Deployments) != 0 || page.NextPageToken != "" {
			allureCtx.Fatalf("page=%+v err=%v", page, err)
		}
	})
}
