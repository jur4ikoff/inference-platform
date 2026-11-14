package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/jur4ikoff/model-service/internal/domain"
)

type createModelInput struct {
	Project string `path:"project" minLength:"1" doc:"Имя проекта"`
	Body    createModelRequest
}

type createModelRequest struct {
	Name        string       `json:"name" minLength:"1" maxLength:"63" pattern:"^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$" doc:"Уникальное в проекте имя модели"`
	Description string       `json:"description,omitempty" doc:"Описание модели"`
	Spec        modelSpecDTO `json:"spec" doc:"Спецификация модели Hugging Face"`
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

type createDeploymentInput struct {
	Project        string `path:"project" minLength:"1" doc:"Имя проекта"`
	ModelID        string `path:"model_id" format:"uuid" doc:"Идентификатор модели"`
	IdempotencyKey string `header:"Idempotency-Key" required:"true" minLength:"1" maxLength:"128" doc:"Уникальный ключ для безопасного повторения запроса"`
}

type updateModelInput struct {
	Project string `path:"project" minLength:"1" doc:"Имя проекта"`
	ModelID string `path:"model_id" format:"uuid" doc:"Идентификатор модели"`
	Body    updateModelRequest
}

type updateModelRequest struct {
	Name        *string       `json:"name,omitempty" minLength:"1" maxLength:"63" pattern:"^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$" doc:"Уникальное в проекте имя модели"`
	Description *string       `json:"description,omitempty" doc:"Описание модели"`
	Spec        *modelSpecDTO `json:"spec,omitempty" doc:"Спецификация модели Hugging Face"`
}

type modelOutput struct {
	Body modelResponseBody
}

type listModelsOutput struct {
	Body listModelsResponse
}

type listModelsResponse struct {
	Models        []listedModelResponse `json:"models" doc:"Модели на текущей странице"`
	NextPageToken *int                  `json:"nextpagetoken" doc:"Следующий offset; null на последней странице"`
}

type listedModelResponse struct {
	modelResponseBody
	Status string `json:"status" enum:"starting,running,stopping,stopped,failed,unknown" doc:"Статус по последнему deployment; stopped, если запусков не было"`
}

type modelResponseBody struct {
	ID          string       `json:"id"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Username    string       `json:"username"`
	Project     string       `json:"project"`
	Spec        modelSpecDTO `json:"spec"`
}

// modelSpecDTO is the HTTP representation. It intentionally mirrors neither
// domain.ModelSpec nor the InferenceService protobuf: either may evolve
// without changing the public API contract.
type modelSpecDTO struct {
	ProviderType        string                  `json:"provider_type" enum:"HUGGING_FACE"`
	ProviderHuggingFace *huggingFaceProviderDTO `json:"provider_hugging_face,omitempty"`
}

type huggingFaceProviderDTO struct {
	Repo string `json:"repo"`
}

func (dto *modelSpecDTO) UnmarshalJSON(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	type wireModelSpecDTO modelSpecDTO
	var value wireModelSpecDTO
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("invalid model specification")
	}
	*dto = modelSpecDTO(value)
	return nil
}

func (dto modelSpecDTO) toDomain() *domain.ModelSpec {
	spec := &domain.ModelSpec{ProviderType: domain.ProviderType(dto.ProviderType)}
	if dto.ProviderHuggingFace != nil {
		spec.ProviderHuggingFace = &domain.HuggingFaceProvider{Repo: dto.ProviderHuggingFace.Repo}
	}
	return spec
}

func modelSpecResponse(spec domain.ModelSpec) modelSpecDTO {
	dto := modelSpecDTO{ProviderType: string(spec.ProviderType)}
	if spec.ProviderHuggingFace != nil {
		dto.ProviderHuggingFace = &huggingFaceProviderDTO{Repo: spec.ProviderHuggingFace.Repo}
	}
	return dto
}
