package service_test

import (
	"context"
	"errors"
	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/service"
	"github.com/jur4ikoff/user-service/internal/testutil"
	"testing"
	"time"
)

// Техника: классы эквивалентности регистрационных данных.
func TestBootstrapAdminCases(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {
		for _, valid := range []bool{true, false} {
			t.Run(map[bool]string{true: "valid", false: "invalid"}[valid], func(t *testing.T) {
				allure.Wrap(t, func(allureCtx *allure.Context) {
					u := testutil.UserMother{}.Admin()
					repo := &testutil.MockBootstrapAdminRepository{BootstrapAdminFunc: func(_ context.Context, u domain.User) (domain.User, bool, error) { return u, false, nil }}
					svc := service.NewBootstrapAdminService(repo, &testutil.MockPasswordHasher{HashFunc: func(string) (string, error) { return "hash", nil }}, &testutil.MockIDGenerator{NewUserIDFunc: func() domain.UserID { return u.ID }}, &testutil.MockClock{NowFunc: func() time.Time { return time.Now() }})
					input := service.RegisterInput{Login: "admin", Email: u.Email, Firstname: u.Firstname, Lastname: u.Lastname, Password: "valid-password"}
					if !valid {
						input.Login = "!"
					}
					result, _, err := svc.Create(context.Background(), input)
					if valid && (err != nil || result.PlatformRole != "admin") {
						allureCtx.Fatalf("result=%+v err=%v", result, err)
					}
					if !valid && !errors.Is(err, service.ErrInvalidInput) {
						allureCtx.Fatalf("err=%v", err)
					}
				})
			})
		}
	})
}
