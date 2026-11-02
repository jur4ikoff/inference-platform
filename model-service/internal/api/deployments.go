package api

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

type createDeploymentOutput struct {
	Location string `header:"Location" doc:"URL созданного развёртывания"`
	Body     deploymentResponse
}

type deploymentOutput struct {
	Body deploymentResponse
}

type deploymentResponse struct {
	ModelID   string    `json:"model_id" format:"uuid" doc:"Идентификатор развёрнутой модели"`
	Project   string    `json:"project" doc:"Имя проекта"`
	Status    string    `json:"status" enum:"starting,running,stopping,failed" doc:"Текущее состояние развёртывания"`
	CreatedAt time.Time `json:"created_at" doc:"Время создания развёртывания"`
	UpdatedAt time.Time `json:"updated_at" doc:"Время последнего изменения состояния"`
}

type deleteDeploymentOutput struct{}

type listDeploymentsInput struct {
	Project string `path:"project" minLength:"1" doc:"Имя проекта"`
	Offset  int    `query:"offset" minimum:"0" default:"0" doc:"Смещение от начала списка"`
	Limit   int    `query:"limit" minimum:"1" maximum:"100" default:"20" doc:"Размер страницы"`
}

type listDeploymentsOutput struct {
	Body listDeploymentsResponse
}

type listDeploymentsResponse struct {
	Deployments   []deploymentResponse `json:"deployments" doc:"Развёртывания на текущей странице"`
	NextPageToken *int                 `json:"nextpagetoken" doc:"Следующий offset; null на последней странице"`
}

func (h *Handler) createDeployment(context.Context, *modelInput) (*createDeploymentOutput, error) {
	return nil, huma.Error501NotImplemented("create deployment endpoint is not implemented")
}

func (h *Handler) getDeployment(context.Context, *modelInput) (*deploymentOutput, error) {
	return nil, huma.Error501NotImplemented("get deployment endpoint is not implemented")
}

func (h *Handler) deleteDeployment(context.Context, *modelInput) (*deleteDeploymentOutput, error) {
	return nil, huma.Error501NotImplemented("delete deployment endpoint is not implemented")
}

func (h *Handler) listDeployments(context.Context, *listDeploymentsInput) (*listDeploymentsOutput, error) {
	return nil, huma.Error501NotImplemented("list deployments endpoint is not implemented")
}
