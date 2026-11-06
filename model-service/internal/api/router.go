package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

const apiPrefix = "/api/v1"

func (h *Handler) Router() http.Handler {
	mux := http.NewServeMux()
	config := huma.DefaultConfig("Model Service API", "1.0.0")
	// Disable Huma's automatic $schema response fields and schema links.
	config.CreateHooks = nil
	config.DocsPath = apiPrefix + "/docs"
	config.OpenAPIPath = apiPrefix + "/openapi"
	config.SchemasPath = apiPrefix + "/schemas"
	config.DocsRenderer = huma.DocsRendererSwaggerUI
	config.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearerAuth": {Type: "http", Scheme: "bearer", BearerFormat: "JWT"},
	}
	api := humago.New(mux, config)
	authenticated := []map[string][]string{{"bearerAuth": {}}}

	huma.Register(api, huma.Operation{
		OperationID:   "createModel",
		Method:        http.MethodPost,
		Path:          apiPrefix + "/projects/{project}/models",
		Summary:       "Создание модели",
		Tags:          []string{"Model"},
		Security:      authenticated,
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusBadRequest, http.StatusConflict, http.StatusInternalServerError},
	}, h.createModel)

	huma.Register(api, huma.Operation{
		OperationID: "listModels",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/projects/{project}/models",
		Summary:     "Список моделей проекта с пагинацией",
		Description: "Возвращает models и nextpagetoken. В nextpagetoken возвращается следующий offset или null на последней странице. По умолчанию limit=20, максимум 100.",
		Tags:        []string{"Model"},
		Security:    authenticated,
		Errors:      []int{http.StatusBadRequest, http.StatusInternalServerError, http.StatusServiceUnavailable, http.StatusGatewayTimeout},
	}, h.listModels)

	huma.Register(api, huma.Operation{
		OperationID: "getModel",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/projects/{project}/models/{model_id}",
		Summary:     "Получение модели",
		Tags:        []string{"Model"},
		Security:    authenticated,
		Errors:      []int{http.StatusBadRequest, http.StatusNotFound, http.StatusInternalServerError},
	}, h.getModel)

	huma.Register(api, huma.Operation{
		OperationID: "updateModel",
		Method:      http.MethodPatch,
		Path:        apiPrefix + "/projects/{project}/models/{model_id}",
		Summary:     "Частичное обновление модели",
		Tags:        []string{"Model"},
		Security:    authenticated,
		Errors:      []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError},
	}, h.updateModel)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteModel",
		Method:        http.MethodDelete,
		Path:          apiPrefix + "/projects/{project}/models/{model_id}",
		Summary:       "Удаление модели",
		Description:   "Удаление разрешено, когда все deployment модели остановлены или завершились ошибкой.",
		Tags:          []string{"Model"},
		Security:      authenticated,
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusServiceUnavailable, http.StatusGatewayTimeout, http.StatusInternalServerError},
	}, h.deleteModel)

	huma.Register(api, huma.Operation{
		OperationID: "getActiveDeployments", Method: http.MethodGet,
		Path:        apiPrefix + "/projects/{project}/deployments",
		Summary:     "Активные запуски моделей проекта",
		Description: "Возвращает запускающиеся и запущенные deployment проекта.",
		Tags:        []string{"Deployment"}, Security: authenticated,
		Errors: []int{http.StatusBadRequest, http.StatusServiceUnavailable, http.StatusGatewayTimeout},
	}, h.getActiveDeployments)

	huma.Register(api, huma.Operation{
		OperationID: "getDeploymentsHistory",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/projects/{project}/deployments/history",
		Summary:     "История запусков моделей проекта",
		Description: "Возвращает все запуски проекта во всех состояниях.",
		Tags:        []string{"Deployment"}, Security: authenticated,
		Errors: []int{http.StatusBadRequest, http.StatusServiceUnavailable, http.StatusGatewayTimeout},
	}, h.getDeploymentsHistory)

	huma.Register(api, huma.Operation{
		OperationID:   "createDeployment",
		Method:        http.MethodPost,
		Path:          apiPrefix + "/projects/{project}/models/{model_id}/deployment",
		Summary:       "Запуск модели",
		Description:   "Запускает существующую модель с её сохранённой спецификацией. Тело запроса не требуется. При успешном принятии возвращает 202, фактический ID deployment и его состояние.",
		Tags:          []string{"Deployment"},
		Security:      authenticated,
		DefaultStatus: http.StatusAccepted,
		Errors:        []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusServiceUnavailable, http.StatusGatewayTimeout},
	}, h.createDeployment)

	huma.Register(api, huma.Operation{
		OperationID: "getDeployment",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/projects/{project}/models/{model_id}/deployment",
		Summary:     "Получение состояния развёртывания",
		Description: "Возвращает состояние deployment существующей модели. Если модели или deployment нет в указанном проекте, возвращает 404.",
		Tags:        []string{"Deployment"},
		Security:    authenticated,
		Errors:      []int{http.StatusBadRequest, http.StatusNotFound, http.StatusServiceUnavailable, http.StatusGatewayTimeout},
	}, h.getDeployment)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteDeployment",
		Method:        http.MethodDelete,
		Path:          apiPrefix + "/projects/{project}/models/{model_id}/deployment",
		Summary:       "Остановка deployment модели",
		Description:   "Запрашивает остановку последнего deployment, не удаляя модель. Возвращает 202 при принятой остановке или 204, если deployment уже остановлен либо отсутствует.",
		Tags:          []string{"Deployment"},
		Security:      authenticated,
		DefaultStatus: http.StatusAccepted,
		Responses: map[string]*huma.Response{
			"204": {Description: "No Content"},
		},
		Errors: []int{http.StatusBadRequest, http.StatusNotFound, http.StatusServiceUnavailable, http.StatusGatewayTimeout},
	}, h.deleteDeployment)

	if h.auth == nil {
		return mux
	}
	return h.authorize(mux)
}
