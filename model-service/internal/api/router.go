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
	api := humago.New(mux, config)

	huma.Register(api, huma.Operation{
		OperationID:   "createModel",
		Method:        http.MethodPost,
		Path:          apiPrefix + "/projects/{project}/models",
		Summary:       "Создание модели",
		Tags:          []string{"Model"},
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
		Errors:      []int{http.StatusBadRequest, http.StatusInternalServerError},
	}, h.listModels)

	huma.Register(api, huma.Operation{
		OperationID: "getModel",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/projects/{project}/models/{model_id}",
		Summary:     "Получение модели",
		Tags:        []string{"Model"},
		Errors:      []int{http.StatusBadRequest, http.StatusNotFound, http.StatusInternalServerError},
	}, h.getModel)

	huma.Register(api, huma.Operation{
		OperationID: "updateModel",
		Method:      http.MethodPatch,
		Path:        apiPrefix + "/projects/{project}/models/{model_id}",
		Summary:     "Частичное обновление модели",
		Tags:        []string{"Model"},
		Errors:      []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError},
	}, h.updateModel)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteModel",
		Method:        http.MethodDelete,
		Path:          apiPrefix + "/projects/{project}/models/{model_id}",
		Summary:       "Удаление модели",
		Tags:          []string{"Model"},
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{http.StatusBadRequest, http.StatusNotFound, http.StatusInternalServerError},
	}, h.deleteModel)

	huma.Register(api, huma.Operation{
		OperationID:   "createDeployment",
		Method:        http.MethodPost,
		Path:          apiPrefix + "/projects/{project}/models/{model_id}/deployment",
		Summary:       "Создание развёртывания модели",
		Description:   "Создаёт развёртывание модели в проекте и запускает его с текущей спецификацией модели. Тело запроса не требуется. У модели может быть одно развёртывание: повторное создание возвращает 409. При успешном создании возвращает 201, состояние развёртывания и заголовок Location. Метод пока не реализован и возвращает 501.",
		Tags:          []string{"Deployment"},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusServiceUnavailable, http.StatusGatewayTimeout, http.StatusNotImplemented},
	}, h.createDeployment)

	huma.Register(api, huma.Operation{
		OperationID: "getDeployment",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/projects/{project}/models/{model_id}/deployment",
		Summary:     "Получение состояния развёртывания",
		Description: "Возвращает текущее состояние развёртывания модели: запуск, работа, остановка или ошибка. Если модель или её развёртывание не найдены в указанном проекте, возвращает 404. Метод пока не реализован и возвращает 501.",
		Tags:        []string{"Deployment"},
		Errors:      []int{http.StatusBadRequest, http.StatusNotFound, http.StatusServiceUnavailable, http.StatusNotImplemented},
	}, h.getDeployment)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteDeployment",
		Method:        http.MethodDelete,
		Path:          apiPrefix + "/projects/{project}/models/{model_id}/deployment",
		Summary:       "Удаление развёртывания модели",
		Description:   "Останавливает развёртывание модели и освобождает его ресурсы. Запись модели и её спецификация сохраняются. После завершения возвращает 204 без тела; повторное удаление отсутствующего развёртывания существующей модели также возвращает 204. Если сама модель не найдена в проекте, возвращает 404. Метод пока не реализован и возвращает 501.",
		Tags:          []string{"Deployment"},
		DefaultStatus: http.StatusNoContent,
		Errors:        []int{http.StatusBadRequest, http.StatusNotFound, http.StatusServiceUnavailable, http.StatusGatewayTimeout, http.StatusNotImplemented},
	}, h.deleteDeployment)

	huma.Register(api, huma.Operation{
		OperationID: "listDeployments",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/projects/{project}/deployments",
		Summary:     "Список развёртываний проекта",
		Description: "Возвращает существующие развёртывания моделей указанного проекта с их состояниями. Поддерживает offset и limit (по умолчанию 20, максимум 100). Порядок — по времени создания, затем по model_id. nextpagetoken содержит следующий offset либо null на последней странице. Модели без развёртывания в список не входят. Метод пока не реализован и возвращает 501.",
		Tags:        []string{"Deployment"},
		Errors:      []int{http.StatusBadRequest, http.StatusNotFound, http.StatusServiceUnavailable, http.StatusNotImplemented},
	}, h.listDeployments)

	return mux
}
