package domain

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
}
