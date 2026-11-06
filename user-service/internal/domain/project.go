package domain

import "errors"

// ProjectID identifies a project.
type ProjectID string

// RoleID identifies a project role.
type RoleID int

type Project struct {
	ID          ProjectID
	Name        string
	Description string
}

type Member struct {
	UserID                     UserID
	Email, Firstname, Lastname string
	RoleID                     RoleID
	RoleName                   string
}

var (
	ErrProjectNotFound = errors.New("project not found")
	ErrProjectExists   = errors.New("project already exists")
	ErrMemberNotFound  = errors.New("member not found")
	ErrRoleNotFound    = errors.New("role not found")
)
