package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

type RegisterInput struct {
	Body RegisterRequest
}

type RegisterRequest struct {
	Login     string `json:"login" minLength:"3" maxLength:"32" pattern:"^[a-z][a-z0-9_-]{2,31}$" doc:"Уникальный внешний логин пользователя" example:"yurii_popov"`
	Firstname string `json:"firstname" minLength:"1" pattern:"\\S" doc:"Имя пользователя" example:"Юрий"`
	Lastname  string `json:"lastname" minLength:"1" pattern:"\\S" doc:"Фамилия пользователя" example:"Попов"`
	Email     string `json:"email" format:"email" doc:"Email пользователя" example:"user@example.com"`
	Password  string `json:"password" format:"password" writeOnly:"true" minLength:"8" maxLength:"72" doc:"Пароль пользователя"`
}

type RegisterOutput struct {
	Body RegisteredUserResponse
}

type RegisteredUserResponse struct {
	Login        string `json:"login" doc:"Уникальный внешний логин пользователя"`
	PlatformRole string `json:"platform_role,omitempty" doc:"Роль платформы для владельца и администратора"`
	Firstname    string `json:"firstname" doc:"Имя пользователя"`
	Lastname     string `json:"lastname" doc:"Фамилия пользователя"`
	ID           string `json:"id" doc:"Идентификатор пользователя" example:"0197eb5a-37c6-7e48-8cd6-0c19edcd72f1"`
	Email        string `json:"email" format:"email" doc:"Email пользователя" example:"user@example.com"`
}

type LoginInput struct {
	Body LoginRequest
}

type LoginRequest struct {
	Login    string `json:"login" minLength:"3" maxLength:"32" pattern:"^[a-z][a-z0-9_-]{2,31}$" doc:"Логин пользователя" example:"yurii_popov"`
	Password string `json:"password" format:"password" minLength:"8" maxLength:"72" doc:"Пароль пользователя"`
}

type LoginOutput struct {
	Body AccessTokenResponse
}

type ProjectTokenInput struct {
	Body struct {
		ProjectName string `json:"project_name" minLength:"1" doc:"Имя проекта для ограниченного гранта"`
	}
}

type ProjectTokenOutput struct{ Body AccessTokenResponse }

type UpdateUserInput struct {
	ID   string `path:"id" format:"uuid" doc:"Идентификатор пользователя"`
	Body UpdateUserRequest
}

type UpdateUserRequest struct {
	Firstname *string `json:"firstname,omitempty" doc:"Имя пользователя"`
	Lastname  *string `json:"lastname,omitempty" doc:"Фамилия пользователя"`
	Email     *string `json:"email,omitempty" format:"email" doc:"Новый email пользователя" example:"user@example.com"`
	Password  *string `json:"password,omitempty" format:"password" writeOnly:"true" minLength:"8" maxLength:"72" doc:"Новый пароль пользователя"`
}

func (r *UpdateUserRequest) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for name, raw := range fields {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("%s cannot be null", name)
		}
	}
	type plain UpdateUserRequest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var decoded plain
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*r = UpdateUserRequest(decoded)
	return nil
}

type UpdateUserOutput struct {
	Body UpdatedUserResponse
}

type UpdatedUserResponse struct {
	RegisteredUserResponse
	CreatedAt time.Time `json:"created_at" doc:"Дата создания пользователя"`
	UpdatedAt time.Time `json:"updated_at" doc:"Дата обновления пользователя"`
}

type DeleteUserInput struct {
	ID string `path:"id" format:"uuid" doc:"Идентификатор пользователя"`
}

// DeleteUserOutput is intentionally empty: successful deletion returns 204.
type DeleteUserOutput struct{}

type GetUserInput struct {
	ID string `path:"id" format:"uuid" doc:"Идентификатор пользователя"`
}
type GetUserByLoginInput struct {
	Login string `path:"login" minLength:"3" maxLength:"32" pattern:"^[a-z][a-z0-9_-]{2,31}$"`
}

type GetUserOutput struct {
	Body RegisteredUserResponse
}

type ListUsersInput struct {
	Offset int `query:"offset" minimum:"0" default:"0" doc:"Смещение от начала списка"`
	Limit  int `query:"limit" minimum:"1" maximum:"100" default:"20" doc:"Размер страницы"`
}

type ListUsersOutput struct {
	Body ListUsersResponse
}

type ListUsersResponse struct {
	Users         []RegisteredUserResponse `json:"users" doc:"Пользователи на текущей странице"`
	NextPageToken *int                     `json:"nextpagetoken" doc:"Следующий offset; null на последней странице"`
}

type CreateProjectInput struct {
	Body CreateProjectRequest
}

type CreateProjectRequest struct {
	Name        string `json:"name" minLength:"1" doc:"Название проекта"`
	Description string `json:"description" doc:"Описание проекта"`
}

type GetProjectInput struct {
	Name string `path:"project_name" doc:"Имя проекта"`
}

type ProjectOutput struct {
	Body ProjectResponse
}

type ProjectResponse struct {
	ID          string `json:"id" format:"uuid" doc:"Идентификатор проекта"`
	Name        string `json:"name" doc:"Название проекта"`
	Description string `json:"description" doc:"Описание проекта"`
}

type UpdateProjectInput struct {
	Name string `path:"project_name" doc:"Имя проекта"`
	Body UpdateProjectRequest
}

type UpdateProjectRequest struct {
	Description *string `json:"description,omitempty" doc:"Новое описание проекта"`
}

type UpdateProjectOutput struct {
	Body ProjectResponse
}

type DeleteProjectInput struct {
	Name string `path:"project_name" doc:"Имя проекта"`
}

// DeleteProjectOutput is intentionally empty: successful deletion returns 204.
type DeleteProjectOutput struct{}

type ListProjectsInput struct {
	Name   string `query:"name" doc:"Точное название проекта для фильтрации" example:"demo-project"`
	Offset int    `query:"offset" minimum:"0" default:"0" doc:"Смещение от начала списка"`
	Limit  int    `query:"limit" minimum:"1" maximum:"100" default:"20" doc:"Размер страницы"`
}

type ListProjectsOutput struct {
	Body ListProjectsResponse
}

type ListProjectsResponse struct {
	Projects      []ProjectResponse `json:"projects" doc:"Проекты на текущей странице"`
	NextPageToken *int              `json:"nextpagetoken" doc:"Следующий offset; null на последней странице"`
}

type MemberPathInput struct {
	Name   string `path:"project_name"`
	UserID string `path:"user_id" format:"uuid"`
}
type PutMemberInput struct {
	Name   string `path:"project_name"`
	UserID string `path:"user_id" format:"uuid"`
	Body   struct {
		RoleID int `json:"role_id" minimum:"1"`
	}
}
type PutMemberOutput struct{ Status int }
type DeleteMemberOutput struct{}
type ListMembersInput struct {
	Name   string `path:"project_name"`
	Offset int    `query:"offset" minimum:"0" default:"0"`
	Limit  int    `query:"limit" minimum:"1" maximum:"100" default:"20"`
}
type MemberResponse struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	Firstname string `json:"firstname"`
	Lastname  string `json:"lastname"`
	RoleID    int    `json:"role_id"`
	RoleName  string `json:"role_name"`
}
type ListMembersOutput struct {
	Body struct {
		Members       []MemberResponse `json:"members"`
		NextPageToken *int             `json:"nextpagetoken"`
	}
}

type CreateRoleInput struct {
	Body CreateRoleRequest
}

type CreateRoleRequest struct {
	Name             string `json:"name" minLength:"1" doc:"Название роли" example:"editor"`
	AllPermissions   bool   `json:"all_permissions,omitempty" doc:"Имеет ли роль все разрешения"`
	CanReadProject   bool   `json:"can_read_project,omitempty" doc:"Просмотр проекта"`
	CanUpdateProject bool   `json:"can_update_project,omitempty" doc:"Изменение проекта"`
	CanDeleteProject bool   `json:"can_delete_project,omitempty" doc:"Удаление проекта"`
	CanManageMembers bool   `json:"can_manage_members,omitempty" doc:"Управление участниками проекта и их ролями"`
	CanCreateModel   bool   `json:"can_create_model,omitempty" doc:"Создание модели в проекте"`
	CanReadModel     bool   `json:"can_read_model,omitempty" doc:"Просмотр моделей проекта"`
	CanUpdateModel   bool   `json:"can_update_model,omitempty" doc:"Изменение модели"`
	CanDeleteModel   bool   `json:"can_delete_model,omitempty" doc:"Удаление модели"`
	CanStartModel    bool   `json:"can_start_model,omitempty" doc:"Запуск модели"`
	CanStopModel     bool   `json:"can_stop_model,omitempty" doc:"Остановка модели"`
}

type GetRoleInput struct {
	ID int `path:"id" minimum:"1" doc:"Идентификатор роли"`
}

type ListRolesInput struct {
	Name   string `query:"name" doc:"Необязательный фильтр по названию роли" example:"editor"`
	Offset int    `query:"offset" minimum:"0" default:"0" doc:"Смещение от начала списка"`
	Limit  int    `query:"limit" minimum:"1" maximum:"100" default:"20" doc:"Размер страницы"`
}

type UpdateRoleInput struct {
	ID   int `path:"id" minimum:"1" doc:"Идентификатор роли"`
	Body UpdateRoleRequest
}

type UpdateRoleRequest struct {
	Name             *string `json:"name,omitempty" doc:"Новое название роли"`
	AllPermissions   *bool   `json:"all_permissions,omitempty" doc:"Новое значение полного доступа"`
	CanReadProject   *bool   `json:"can_read_project,omitempty" doc:"Просмотр проекта"`
	CanUpdateProject *bool   `json:"can_update_project,omitempty" doc:"Изменение проекта"`
	CanDeleteProject *bool   `json:"can_delete_project,omitempty" doc:"Удаление проекта"`
	CanManageMembers *bool   `json:"can_manage_members,omitempty" doc:"Управление участниками проекта и их ролями"`
	CanCreateModel   *bool   `json:"can_create_model,omitempty" doc:"Создание модели в проекте"`
	CanReadModel     *bool   `json:"can_read_model,omitempty" doc:"Просмотр моделей проекта"`
	CanUpdateModel   *bool   `json:"can_update_model,omitempty" doc:"Изменение модели"`
	CanDeleteModel   *bool   `json:"can_delete_model,omitempty" doc:"Удаление модели"`
	CanStartModel    *bool   `json:"can_start_model,omitempty" doc:"Запуск модели"`
	CanStopModel     *bool   `json:"can_stop_model,omitempty" doc:"Остановка модели"`
}

func (r *UpdateRoleRequest) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for name, raw := range fields {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("%s cannot be null", name)
		}
	}
	type plain UpdateRoleRequest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var decoded plain
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*r = UpdateRoleRequest(decoded)
	return nil
}

type DeleteRoleInput struct {
	ID int `path:"id" minimum:"1" doc:"Идентификатор роли"`
}

// DeleteRoleOutput is intentionally empty: successful deletion returns 204.
type DeleteRoleOutput struct{}

type SetPlatformRoleInput struct {
	ID   string `path:"id" format:"uuid" doc:"Идентификатор пользователя"`
	Body SetPlatformRoleRequest
}
type SetPlatformRoleRequest struct {
	PlatformRole string `json:"platform_role" enum:"user,analyst,admin" doc:"Глобальная роль платформы"`
}

type RoleOutput struct {
	Body RoleResponse
}

type ListRolesOutput struct {
	Body ListRolesResponse
}

type ListRolesResponse struct {
	Roles         []RoleResponse `json:"roles" doc:"Роли на текущей странице"`
	NextPageToken *int           `json:"nextpagetoken" doc:"Следующий offset; null на последней странице"`
}

type RoleResponse struct {
	ID               int    `json:"id" doc:"Идентификатор роли"`
	Name             string `json:"name" doc:"Название роли"`
	AllPermissions   bool   `json:"all_permissions" doc:"Имеет ли роль все разрешения"`
	CanReadProject   bool   `json:"can_read_project" doc:"Просмотр проекта"`
	CanUpdateProject bool   `json:"can_update_project" doc:"Изменение проекта"`
	CanDeleteProject bool   `json:"can_delete_project" doc:"Удаление проекта"`
	CanManageMembers bool   `json:"can_manage_members" doc:"Управление участниками проекта и их ролями"`
	CanCreateModel   bool   `json:"can_create_model" doc:"Создание модели в проекте"`
	CanReadModel     bool   `json:"can_read_model" doc:"Просмотр моделей проекта"`
	CanUpdateModel   bool   `json:"can_update_model" doc:"Изменение модели"`
	CanDeleteModel   bool   `json:"can_delete_model" doc:"Удаление модели"`
	CanStartModel    bool   `json:"can_start_model" doc:"Запуск модели"`
	CanStopModel     bool   `json:"can_stop_model" doc:"Остановка модели"`
}

type AccessTokenResponse struct {
	AccessToken string    `json:"access_token" doc:"JWT access token"`
	TokenType   string    `json:"token_type" enum:"Bearer" doc:"Тип токена" example:"Bearer"`
	ExpiresAt   time.Time `json:"expires_at" format:"date-time" doc:"Время истечения токена"`
}
