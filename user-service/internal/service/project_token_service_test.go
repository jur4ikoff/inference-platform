package service_test

import (
	"context"
	"errors"
	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/service"
	"github.com/jur4ikoff/user-service/internal/testutil"
	"github.com/jur4ikoff/utils/authjwt"
	"testing"
	"time"
)

// Техника: классы эквивалентности доступа и граничное время токена.
func TestProjectTokenCases(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {

		for _, valid := range []bool{true, false} {
			t.Run(map[bool]string{true: "valid", false: "anonymous"}[valid], func(t *testing.T) {
				allure.Wrap(t, func(allureCtx *allure.Context) {

					u := testutil.UserMother{}.Regular()
					now := time.Now().UTC()
					end := now.Add(time.Minute)
					repo := &testutil.MockProjectTokenRepository{ProjectTokenGrantFunc: func(_ context.Context, _ domain.UserID, n string) (authjwt.ProjectGrant, error) {
						return authjwt.ProjectGrant{Name: n, Role: "developer"}, nil
					}}
					tokens := &testutil.MockTokenManager{CreateAccessTokenFunc: func(s service.Session, g *authjwt.ProjectGrant, e time.Time) (string, error) {
						if s.Username != u.Login || !e.Equal(end) {
							allureCtx.Fatal("wrong token data")
						}
						return "token", nil
					}}
					svc := service.NewProjectTokenService(repo, tokens, time.Hour, &testutil.MockClock{NowFunc: func() time.Time { return now }})
					ctx := context.Background()
					if valid {
						ctx = service.WithPrincipal(ctx, service.Principal{UserID: u.ID, Username: u.Login, ExpiresAt: end})
					}
					token, err := svc.Create(ctx, "demo")
					if valid && (err != nil || token.Value != "token") {
						allureCtx.Fatalf("token=%+v err=%v", token, err)
					}
					if !valid && !errors.Is(err, service.ErrInvalidToken) {
						allureCtx.Fatalf("err=%v", err)
					}

				})
			})
		}

	})
}
