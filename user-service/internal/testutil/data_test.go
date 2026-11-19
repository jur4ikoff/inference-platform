package testutil

import (
	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/jur4ikoff/user-service/internal/domain"
	"testing"
)

// Техника: разбиение по классам эквивалентности ролей.
func TestUserMotherCreatesNamedRoles(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {

		mother := UserMother{}
		if mother.Regular().PlatformRole != domain.PlatformRoleUser || mother.Admin().PlatformRole != domain.PlatformRoleAdmin {
			allureCtx.Fatal("unexpected user roles")
		}

	})
}
