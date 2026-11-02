package domain

// ProjectID identifies a project.
type ProjectID string

// RoleID identifies a project role.
type RoleID int

type Project struct {
	ID          ProjectID
	Name        string
	Description string
}
