package api

import (
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/jur4ikoff/user-service/internal/api/handler"
)

const apiPrefix = "/api/v1"

func NewRouter(authHandler *handler.AuthHandler, userHandler *handler.UserHandler, projectHandler *handler.ProjectHandler, roleHandler *handler.RoleHandler, version string) http.Handler {
	mux := http.NewServeMux()

	config := huma.DefaultConfig("User Service API", version)
	// Disable Huma's automatic $schema response fields and schema links.
	config.CreateHooks = nil
	config.DocsPath = apiPrefix + "/docs"
	config.OpenAPIPath = apiPrefix + "/openapi"
	config.SchemasPath = apiPrefix + "/schemas"
	config.DocsRenderer = huma.DocsRendererSwaggerUI
	config.Transformers = append(config.Transformers, omitRegistrationErrorValues)
	config.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearerAuth": {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "JWT",
		},
	}

	api := humago.New(mux, config)
	authentication := authHandler.Middleware(api)
	mux.HandleFunc("GET /internal/auth", authHandler.Authenticate)
	mux.HandleFunc("GET /.well-known/jwks.json", authHandler.JWKS)

	registerAuthRoutes(api, authHandler)
	registerUserRoutes(api, userHandler, authentication)
	registerProjectRoutes(api, projectHandler, authentication)
	registerRoleRoutes(api, roleHandler, authentication)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/users" || strings.HasPrefix(r.URL.Path, "/api/v1/users/") || r.URL.Path == "/api/v1/projects" || strings.HasPrefix(r.URL.Path, "/api/v1/projects/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		mux.ServeHTTP(w, r)
	})
}

func omitRegistrationErrorValues(ctx huma.Context, _ string, value any) (any, error) {
	switch ctx.Operation().OperationID {
	case "registerUser", "updateUser":
		if problem, ok := value.(*huma.ErrorModel); ok {
			// Validation errors can echo the whole request, including its password.
			for _, detail := range problem.Errors {
				detail.Value = nil
			}
		}
	}
	return value, nil
}

func registerAuthRoutes(api huma.API, authHandler *handler.AuthHandler) {
	huma.Register(api, huma.Operation{
		OperationID:   "registerUser",
		Method:        http.MethodPost,
		Path:          apiPrefix + "/auth/register",
		Summary:       "Регистрация пользователя",
		Tags:          []string{"Auth"},
		DefaultStatus: http.StatusCreated,
		Errors:        []int{http.StatusBadRequest, http.StatusConflict, http.StatusInternalServerError},
	}, authHandler.Register)

	huma.Register(api, huma.Operation{
		OperationID: "loginUser",
		Method:      http.MethodPost,
		Path:        apiPrefix + "/auth/login",
		Summary:     "Вход пользователя",
		Tags:        []string{"Auth"},
	}, authHandler.Login)

	huma.Register(api, huma.Operation{
		OperationID: "createProjectToken", Method: http.MethodPost, Path: apiPrefix + "/auth/project-token",
		Summary: "Выпуск JWT с грантом одного проекта", Tags: []string{"Auth"},
		Security: []map[string][]string{{"bearerAuth": {}}}, Middlewares: huma.Middlewares{authHandler.Middleware(api)},
	}, authHandler.ProjectToken)
}

func registerUserRoutes(api huma.API, userHandler *handler.UserHandler, authentication handler.AuthMiddleware) {
	huma.Register(api, huma.Operation{
		OperationID: "listUsers",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/users",
		Summary:     "Список пользователей с пагинацией",
		Description: "Возвращает users и nextpagetoken. В nextpagetoken возвращается следующий offset или null на последней странице. По умолчанию limit=20, максимум 100. Требуется аутентификация.",
		Tags:        []string{"User"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusInternalServerError},
		Middlewares: huma.Middlewares{authentication},
	}, userHandler.ListUsers)

	huma.Register(api, huma.Operation{
		OperationID: "updateUser",
		Method:      http.MethodPatch,
		Path:        apiPrefix + "/users/{id}",
		Summary:     "Обновление пользователя",
		Description: "Частичное обновление собственных email, пароля, имени и фамилии. Пропущенные поля сохраняют текущие значения.",
		Errors:      []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError},
		Tags:        []string{"User"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Middlewares: huma.Middlewares{authentication},
	}, userHandler.UpdateUser)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteUser",
		Method:        http.MethodDelete,
		Path:          apiPrefix + "/users/{id}",
		Summary:       "Удаление пользователя",
		Tags:          []string{"User"},
		DefaultStatus: http.StatusNoContent,
		Security:      []map[string][]string{{"bearerAuth": {}}},
		Middlewares:   huma.Middlewares{authentication},
	}, userHandler.DeleteUser)

	huma.Register(api, huma.Operation{
		OperationID: "getUserByLogin",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/users/by-login/{login}",
		Summary:     "Получение пользователя по login",
		Tags:        []string{"User"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{http.StatusUnauthorized, http.StatusNotFound},
		Middlewares: huma.Middlewares{authentication},
	}, userHandler.GetUserByLogin)

	huma.Register(api, huma.Operation{
		OperationID: "getUser",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/users/{id}",
		Summary:     "Получение пользователя по id",
		Tags:        []string{"User"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Errors:      []int{http.StatusUnauthorized, http.StatusNotFound},
		Middlewares: huma.Middlewares{authentication},
	}, userHandler.GetUserByID)

	if userHandler.HasPlatformRoleService() {
		huma.Register(api, huma.Operation{
			OperationID: "setPlatformRole", Method: http.MethodPut,
			Path: apiPrefix + "/users/{id}/platform-role", Summary: "Назначение глобальной роли",
			Tags: []string{"User"}, Security: []map[string][]string{{"bearerAuth": {}}},
			Middlewares: huma.Middlewares{authentication},
		}, userHandler.SetPlatformRole)
	}
}

func registerProjectRoutes(api huma.API, projectHandler *handler.ProjectHandler, authentication handler.AuthMiddleware) {
	huma.Register(api, huma.Operation{
		OperationID:   "createProject",
		Method:        http.MethodPost,
		Path:          apiPrefix + "/projects",
		Summary:       "Создание проекта",
		Tags:          []string{"Project"},
		DefaultStatus: http.StatusCreated,
		Security:      []map[string][]string{{"bearerAuth": {}}},
		Middlewares:   huma.Middlewares{authentication},
	}, projectHandler.CreateProject)

	huma.Register(api, huma.Operation{
		OperationID: "getProject",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/projects/{project_name}",
		Summary:     "Получение проекта по имени",
		Tags:        []string{"Project"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Middlewares: huma.Middlewares{authentication},
	}, projectHandler.GetProject)

	huma.Register(api, huma.Operation{
		OperationID: "updateProject",
		Method:      http.MethodPatch,
		Path:        apiPrefix + "/projects/{project_name}",
		Summary:     "Частичное обновление информации о проекте",
		Tags:        []string{"Project"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Middlewares: huma.Middlewares{authentication},
	}, projectHandler.UpdateProject)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteProject",
		Method:        http.MethodDelete,
		Path:          apiPrefix + "/projects/{project_name}",
		Summary:       "Удаление проекта",
		Tags:          []string{"Project"},
		DefaultStatus: http.StatusNoContent,
		Security:      []map[string][]string{{"bearerAuth": {}}},
		Middlewares:   huma.Middlewares{authentication},
	}, projectHandler.DeleteProject)

	huma.Register(api, huma.Operation{
		OperationID: "listProjects",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/projects",
		Summary:     "Список проектов с пагинацией",
		Description: "Проекты текущего пользователя. В nextpagetoken возвращается следующий offset или null на последней странице. По умолчанию limit=20, максимум 100.",
		Errors:      []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusInternalServerError},
		Tags:        []string{"Project"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Middlewares: huma.Middlewares{authentication},
	}, projectHandler.ListProjects)
	huma.Register(api, huma.Operation{OperationID: "listProjectMembers", Method: http.MethodGet, Path: apiPrefix + "/projects/{project_name}/members", Tags: []string{"Project"}, Security: []map[string][]string{{"bearerAuth": {}}}, Middlewares: huma.Middlewares{authentication}}, projectHandler.ListMembers)
	huma.Register(api, huma.Operation{
		OperationID: "putProjectMember", Method: http.MethodPut,
		Path:          apiPrefix + "/projects/{project_name}/members/{user_id}",
		DefaultStatus: http.StatusOK,
		Responses: map[string]*huma.Response{
			"200": {Description: "Роль участника назначена или уже была назначена"},
			"201": {Description: "Участник добавлен в проект"},
		},
		Errors: []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity, http.StatusInternalServerError},
		Tags:   []string{"Project"}, Security: []map[string][]string{{"bearerAuth": {}}}, Middlewares: huma.Middlewares{authentication},
	}, projectHandler.PutMember)
	huma.Register(api, huma.Operation{OperationID: "deleteProjectMember", Method: http.MethodDelete, Path: apiPrefix + "/projects/{project_name}/members/{user_id}", DefaultStatus: http.StatusNoContent, Tags: []string{"Project"}, Security: []map[string][]string{{"bearerAuth": {}}}, Middlewares: huma.Middlewares{authentication}}, projectHandler.DeleteMember)

}

func registerRoleRoutes(api huma.API, roleHandler *handler.RoleHandler, authentication handler.AuthMiddleware) {
	huma.Register(api, huma.Operation{
		OperationID:   "createRole",
		Method:        http.MethodPost,
		Path:          apiPrefix + "/roles",
		Summary:       "Создание роли",
		Tags:          []string{"Role"},
		DefaultStatus: http.StatusCreated,
		Security:      []map[string][]string{{"bearerAuth": {}}},
		Middlewares:   huma.Middlewares{authentication},
	}, roleHandler.CreateRole)

	huma.Register(api, huma.Operation{
		OperationID: "listRoles",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/roles",
		Summary:     "Список ролей с пагинацией",
		Description: "В nextpagetoken возвращается следующий offset или null на последней странице. По умолчанию limit=20, максимум 100.",
		Errors:      []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusInternalServerError},
		Tags:        []string{"Role"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Middlewares: huma.Middlewares{authentication},
	}, roleHandler.ListRoles)

	huma.Register(api, huma.Operation{
		OperationID: "getRole",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/roles/{id}",
		Summary:     "Получение роли",
		Tags:        []string{"Role"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Middlewares: huma.Middlewares{authentication},
	}, roleHandler.GetRole)

	huma.Register(api, huma.Operation{
		OperationID: "updateRole",
		Method:      http.MethodPatch,
		Path:        apiPrefix + "/roles/{id}",
		Summary:     "Частичное обновление роли",
		Tags:        []string{"Role"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
		Middlewares: huma.Middlewares{authentication},
	}, roleHandler.UpdateRole)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteRole",
		Method:        http.MethodDelete,
		Path:          apiPrefix + "/roles/{id}",
		Summary:       "Удаление роли",
		Tags:          []string{"Role"},
		DefaultStatus: http.StatusNoContent,
		Security:      []map[string][]string{{"bearerAuth": {}}},
		Middlewares:   huma.Middlewares{authentication},
	}, roleHandler.DeleteRole)
}
