package testutil

import (
	"github.com/jur4ikoff/user-service/internal/domain"
	"testing"
)

// Техника: разбиение по классам эквивалентности ролей.
func TestUserMotherCreatesNamedRoles(t *testing.T) {
	mother := UserMother{}
	if mother.Regular().PlatformRole != domain.PlatformRoleUser || mother.Admin().PlatformRole != domain.PlatformRoleAdmin {
		t.Fatal("unexpected user roles")
	}
}
