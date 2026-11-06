package testutil

import (
	"github.com/jur4ikoff/user-service/internal/domain"
	"time"
)

var fixtureTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// UserBuilder builds independent values; defaults are valid for ordinary tests.
type UserBuilder struct{ value domain.User }

func NewUserBuilder() *UserBuilder {
	return &UserBuilder{value: domain.User{ID: "0197eb5a-37c6-7e48-8cd6-0c19edcd72f1", Login: "alice", Email: "alice@example.com", Firstname: "Alice", Lastname: "Example", PasswordHash: "test-hash", PlatformRole: domain.PlatformRoleUser, CreatedAt: fixtureTime, UpdatedAt: fixtureTime}}
}
func (b *UserBuilder) WithID(id domain.UserID) *UserBuilder { b.value.ID = id; return b }
func (b *UserBuilder) WithLogin(login string) *UserBuilder  { b.value.Login = login; return b }
func (b *UserBuilder) WithEmail(email string) *UserBuilder  { b.value.Email = email; return b }
func (b *UserBuilder) WithRole(role domain.PlatformRole) *UserBuilder {
	b.value.PlatformRole = role
	return b
}
func (b *UserBuilder) WithPasswordHash(hash string) *UserBuilder {
	b.value.PasswordHash = hash
	return b
}
func (b *UserBuilder) Build() domain.User { return b.value }

// UserMother supplies named common user fixtures.
type UserMother struct{}

func (UserMother) Regular() domain.User { return NewUserBuilder().Build() }
func (UserMother) Admin() domain.User {
	return NewUserBuilder().WithLogin("admin").WithEmail("admin@example.com").WithRole(domain.PlatformRoleAdmin).Build()
}
func ProjectFixture() domain.Project {
	return domain.Project{ID: "0197eb5a-37c6-7e48-8cd6-0c19edcd72f2", Name: "demo", Description: "Test project"}
}
func RoleFixture() domain.Role {
	return domain.Role{ID: 1, Name: "developer", CanReadModel: true, CanCreateModel: true}
}

type ProjectBuilder struct{ value domain.Project }

func NewProjectBuilder() *ProjectBuilder                             { return &ProjectBuilder{value: ProjectFixture()} }
func (b *ProjectBuilder) WithName(name string) *ProjectBuilder       { b.value.Name = name; return b }
func (b *ProjectBuilder) WithID(id domain.ProjectID) *ProjectBuilder { b.value.ID = id; return b }
func (b *ProjectBuilder) Build() domain.Project                      { return b.value }

type ProjectMother struct{}

func (ProjectMother) Demo() domain.Project { return NewProjectBuilder().Build() }

type RoleBuilder struct{ value domain.Role }

func NewRoleBuilder() *RoleBuilder                       { return &RoleBuilder{value: RoleFixture()} }
func (b *RoleBuilder) WithName(name string) *RoleBuilder { b.value.Name = name; return b }
func (b *RoleBuilder) WithAllPermissions(enabled bool) *RoleBuilder {
	b.value.AllPermissions = enabled
	return b
}
func (b *RoleBuilder) Build() domain.Role { return b.value }

type RoleMother struct{}

func (RoleMother) Developer() domain.Role { return NewRoleBuilder().Build() }
func (RoleMother) Administrator() domain.Role {
	return NewRoleBuilder().WithName("administrator").WithAllPermissions(true).Build()
}
