// Package testutil provides fixtures and configurable test doubles.
package testutil

import (
	"context"
	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/service"
	"github.com/jur4ikoff/utils/authjwt"
	"time"
)

// MockUserRepository delegates each call to the corresponding Func field.
type MockUserRepository struct {
	CreateFunc      func(ctx context.Context, u domain.User) error
	FindByEmailFunc func(ctx context.Context, email string) (domain.User, error)
	FindByLoginFunc func(ctx context.Context, login string) (domain.User, error)
	ListFunc        func(ctx context.Context, offset, limit int) ([]domain.User, error)
	UpdateFunc      func(ctx context.Context, id domain.UserID, changes domain.UserUpdate, updatedAt time.Time) (domain.User, error)
	DeleteFunc      func(ctx context.Context, id domain.UserID) error
}

var _ service.UserRepository = (*MockUserRepository)(nil)

func (m *MockUserRepository) Create(ctx context.Context, u domain.User) error {
	return m.CreateFunc(ctx, u)
}
func (m *MockUserRepository) FindByEmail(ctx context.Context, email string) (domain.User, error) {
	return m.FindByEmailFunc(ctx, email)
}
func (m *MockUserRepository) FindByLogin(ctx context.Context, login string) (domain.User, error) {
	return m.FindByLoginFunc(ctx, login)
}
func (m *MockUserRepository) List(ctx context.Context, offset, limit int) ([]domain.User, error) {
	return m.ListFunc(ctx, offset, limit)
}
func (m *MockUserRepository) Update(ctx context.Context, id domain.UserID, changes domain.UserUpdate, updatedAt time.Time) (domain.User, error) {
	return m.UpdateFunc(ctx, id, changes, updatedAt)
}
func (m *MockUserRepository) Delete(ctx context.Context, id domain.UserID) error {
	return m.DeleteFunc(ctx, id)
}

// MockProjectRepository delegates each call to the corresponding Func field.
type MockProjectRepository struct {
	CreateFunc       func(ctx context.Context, actor domain.UserID, name, description string) (domain.Project, error)
	ListFunc         func(ctx context.Context, actor domain.UserID, name string, offset, limit int) ([]domain.Project, error)
	GetFunc          func(ctx context.Context, actor domain.UserID, name string) (domain.Project, error)
	UpdateFunc       func(ctx context.Context, actor domain.UserID, name, description string) (domain.Project, error)
	DeleteFunc       func(ctx context.Context, actor domain.UserID, name string) error
	MembersFunc      func(ctx context.Context, actor domain.UserID, name string, offset, limit int) ([]domain.Member, error)
	ChangeMemberFunc func(ctx context.Context, actor domain.UserID, name string, target domain.UserID, role domain.RoleID, remove bool) (bool, error)
}

var _ service.ProjectRepository = (*MockProjectRepository)(nil)

func (m *MockProjectRepository) Create(ctx context.Context, actor domain.UserID, name, description string) (domain.Project, error) {
	return m.CreateFunc(ctx, actor, name, description)
}
func (m *MockProjectRepository) List(ctx context.Context, actor domain.UserID, name string, offset, limit int) ([]domain.Project, error) {
	return m.ListFunc(ctx, actor, name, offset, limit)
}
func (m *MockProjectRepository) Get(ctx context.Context, actor domain.UserID, name string) (domain.Project, error) {
	return m.GetFunc(ctx, actor, name)
}
func (m *MockProjectRepository) Update(ctx context.Context, actor domain.UserID, name, description string) (domain.Project, error) {
	return m.UpdateFunc(ctx, actor, name, description)
}
func (m *MockProjectRepository) Delete(ctx context.Context, actor domain.UserID, name string) error {
	return m.DeleteFunc(ctx, actor, name)
}
func (m *MockProjectRepository) Members(ctx context.Context, actor domain.UserID, name string, offset, limit int) ([]domain.Member, error) {
	return m.MembersFunc(ctx, actor, name, offset, limit)
}
func (m *MockProjectRepository) ChangeMember(ctx context.Context, actor domain.UserID, name string, target domain.UserID, role domain.RoleID, remove bool) (bool, error) {
	return m.ChangeMemberFunc(ctx, actor, name, target, role, remove)
}

// MockRoleRepository delegates each call to the corresponding Func field.
type MockRoleRepository struct {
	CreateFunc func(ctx context.Context, actor domain.UserID, role domain.Role) (domain.Role, error)
	GetFunc    func(ctx context.Context, id domain.RoleID) (domain.Role, error)
	ListFunc   func(ctx context.Context, name string, offset, limit int) ([]domain.Role, error)
	UpdateFunc func(ctx context.Context, actor domain.UserID, id domain.RoleID, update domain.RoleUpdate) (domain.Role, error)
	DeleteFunc func(ctx context.Context, actor domain.UserID, id domain.RoleID) error
}

var _ service.RoleRepository = (*MockRoleRepository)(nil)

func (m *MockRoleRepository) Create(ctx context.Context, actor domain.UserID, role domain.Role) (domain.Role, error) {
	return m.CreateFunc(ctx, actor, role)
}
func (m *MockRoleRepository) Get(ctx context.Context, id domain.RoleID) (domain.Role, error) {
	return m.GetFunc(ctx, id)
}
func (m *MockRoleRepository) List(ctx context.Context, name string, offset, limit int) ([]domain.Role, error) {
	return m.ListFunc(ctx, name, offset, limit)
}
func (m *MockRoleRepository) Update(ctx context.Context, actor domain.UserID, id domain.RoleID, update domain.RoleUpdate) (domain.Role, error) {
	return m.UpdateFunc(ctx, actor, id, update)
}
func (m *MockRoleRepository) Delete(ctx context.Context, actor domain.UserID, id domain.RoleID) error {
	return m.DeleteFunc(ctx, actor, id)
}

// MockPlatformRoleRepository delegates each call to the corresponding Func field.
type MockPlatformRoleRepository struct {
	SetPlatformRoleFunc func(ctx context.Context, actor, target domain.UserID, role domain.PlatformRole) (domain.User, error)
}

var _ service.PlatformRoleRepository = (*MockPlatformRoleRepository)(nil)

func (m *MockPlatformRoleRepository) SetPlatformRole(ctx context.Context, actor, target domain.UserID, role domain.PlatformRole) (domain.User, error) {
	return m.SetPlatformRoleFunc(ctx, actor, target, role)
}

// MockProjectTokenRepository delegates each call to the corresponding Func field.
type MockProjectTokenRepository struct {
	ProjectTokenGrantFunc func(ctx context.Context, actor domain.UserID, name string) (authjwt.ProjectGrant, error)
}

var _ service.ProjectTokenRepository = (*MockProjectTokenRepository)(nil)

func (m *MockProjectTokenRepository) ProjectTokenGrant(ctx context.Context, actor domain.UserID, name string) (authjwt.ProjectGrant, error) {
	return m.ProjectTokenGrantFunc(ctx, actor, name)
}

// MockBootstrapAdminRepository delegates each call to the corresponding Func field.
type MockBootstrapAdminRepository struct {
	BootstrapAdminFunc func(ctx context.Context, user domain.User) (domain.User, bool, error)
}

var _ service.BootstrapAdminRepository = (*MockBootstrapAdminRepository)(nil)

func (m *MockBootstrapAdminRepository) BootstrapAdmin(ctx context.Context, user domain.User) (domain.User, bool, error) {
	return m.BootstrapAdminFunc(ctx, user)
}

// MockPasswordHasher delegates each call to the corresponding Func field.
type MockPasswordHasher struct {
	HashFunc    func(password string) (string, error)
	CompareFunc func(hash, password string) error
}

var _ service.PasswordHasher = (*MockPasswordHasher)(nil)

func (m *MockPasswordHasher) Hash(password string) (string, error) { return m.HashFunc(password) }
func (m *MockPasswordHasher) Compare(hash, password string) error {
	return m.CompareFunc(hash, password)
}

// MockTokenManager delegates each call to the corresponding Func field.
type MockTokenManager struct {
	CreateAccessTokenFunc func(session service.Session, project *authjwt.ProjectGrant, expiresAt time.Time) (string, error)
	ParseAccessTokenFunc  func(token string) (service.Principal, error)
}

var _ service.TokenManager = (*MockTokenManager)(nil)

func (m *MockTokenManager) CreateAccessToken(session service.Session, project *authjwt.ProjectGrant, expiresAt time.Time) (string, error) {
	return m.CreateAccessTokenFunc(session, project, expiresAt)
}
func (m *MockTokenManager) ParseAccessToken(token string) (service.Principal, error) {
	return m.ParseAccessTokenFunc(token)
}

type MockClock struct{ NowFunc func() time.Time }

func (m *MockClock) Now() time.Time { return m.NowFunc() }

var _ service.Clock = (*MockClock)(nil)

type MockIDGenerator struct{ NewUserIDFunc func() domain.UserID }

func (m *MockIDGenerator) NewUserID() domain.UserID { return m.NewUserIDFunc() }

var _ service.IDGenerator = (*MockIDGenerator)(nil)
