package api

import (
	"encoding/json"
	"time"
)

type createModelInput struct {
	Project string `path:"project" minLength:"1" doc:"Имя проекта"`
	Body    createModelRequest
}

type createModelRequest struct {
	Name        string          `json:"name" minLength:"1" doc:"Название модели"`
	Description string          `json:"description,omitempty" doc:"Описание модели"`
	UserID      string          `json:"user_id" minLength:"1" doc:"Идентификатор пользователя"`
	Spec        json.RawMessage `json:"spec" doc:"Спецификация модели в формате JSON"`
}

type listModelsInput struct {
	Project string `path:"project" minLength:"1" doc:"Имя проекта"`
	Offset  int    `query:"offset" minimum:"0" default:"0" doc:"Смещение от начала списка"`
	Limit   int    `query:"limit" minimum:"1" maximum:"100" default:"20" doc:"Размер страницы"`
}

type modelInput struct {
	Project string `path:"project" minLength:"1" doc:"Имя проекта"`
	ModelID string `path:"model_id" format:"uuid" doc:"Идентификатор модели"`
}

type updateModelInput struct {
	Project string `path:"project" minLength:"1" doc:"Имя проекта"`
	ModelID string `path:"model_id" format:"uuid" doc:"Идентификатор модели"`
	Body    updateModelRequest
}

type updateModelRequest struct {
	Name        *string         `json:"name,omitempty" minLength:"1" doc:"Название модели"`
	Description *string         `json:"description,omitempty" doc:"Описание модели"`
	Spec        json.RawMessage `json:"spec,omitempty" doc:"Спецификация модели в формате JSON"`
}

type modelOutput struct {
	Body modelResponseBody
}

type listModelsOutput struct {
	Body listModelsResponse
}

type listModelsResponse struct {
	Models        []modelResponseBody `json:"models" doc:"Модели на текущей странице"`
	NextPageToken *int                `json:"nextpagetoken" doc:"Следующий offset; null на последней странице"`
}

type modelResponseBody struct {
	ID          string          `json:"id"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	UserID      string          `json:"user_id"`
	Project     string          `json:"project"`
	Spec        json.RawMessage `json:"spec"`
}
