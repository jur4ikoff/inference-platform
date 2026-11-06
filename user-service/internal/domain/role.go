package domain

import "errors"

type Role struct {
	ID               RoleID
	Name             string
	AllPermissions   bool
	CanReadProject   bool
	CanUpdateProject bool
	CanDeleteProject bool
	CanManageMembers bool
	CanCreateModel   bool
	CanReadModel     bool
	CanUpdateModel   bool
	CanDeleteModel   bool
	CanStartModel    bool
	CanStopModel     bool
	SystemKey        string
}

type RoleUpdate struct {
	Name             *string
	AllPermissions   *bool
	CanReadProject   *bool
	CanUpdateProject *bool
	CanDeleteProject *bool
	CanManageMembers *bool
	CanCreateModel   *bool
	CanReadModel     *bool
	CanUpdateModel   *bool
	CanDeleteModel   *bool
	CanStartModel    *bool
	CanStopModel     *bool
}

var (
	ErrRoleAlreadyExists = errors.New("role name already exists")
	ErrRoleAssigned      = errors.New("role is assigned to project members")
	ErrSystemRole        = errors.New("system role is protected")
)
