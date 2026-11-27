package service_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"os"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jur4ikoff/user-service/internal/infrastructure"
	"github.com/jur4ikoff/user-service/internal/repository"
	"github.com/jur4ikoff/user-service/internal/service"
)

// AAA: real service, password hashing, JWT signing and PostgreSQL repository.
func TestServiceIntegration(t *testing.T) {
	dsn := os.Getenv("USER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("USER_TEST_DATABASE_URL is required for integration")
	}
	allure.Wrap(t, func(t *allure.Context) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer pool.Close()
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		tokens, err := infrastructure.NewTokenManager(key, "test-key", "user-service", "inference-platform")
		if err != nil {
			t.Fatal(err)
		}
		repo := repository.NewUserRepository(pool)
		svc := service.NewUserService(repo, infrastructure.NewPasswordHasher(1), tokens, infrastructure.IDGenerator{}, infrastructure.Clock{}, time.Minute)
		const password = "integration-user-password"
		registered, err := svc.Register(ctx, service.RegisterInput{Login: "integrationuser", Email: "integration@example.test", Firstname: "Integration", Lastname: "User", Password: password})
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelCleanup()
			if _, err := pool.Exec(cleanupCtx, "DELETE FROM audit_events WHERE object_id=$1", string(registered.ID)); err != nil {
				t.Error(err)
			}
			if _, err := pool.Exec(cleanupCtx, "DELETE FROM users WHERE id=$1", string(registered.ID)); err != nil {
				t.Error(err)
			}
		}()
		saved, err := repo.FindByLogin(ctx, registered.Login)
		if err != nil {
			t.Fatal(err)
		}
		if saved.ID != registered.ID || saved.Email != registered.Email || saved.PasswordHash == password {
			t.Fatalf("incorrect saved user: %+v", saved)
		}
		token, err := svc.Login(ctx, service.LoginInput{Login: registered.Login, Password: password})
		if err != nil || token.Value == "" {
			t.Fatalf("login: token=%+v error=%v", token, err)
		}
		principal, err := svc.Verify(ctx, token.Value)
		if err != nil || principal.UserID != saved.ID {
			t.Fatalf("verify: principal=%+v error=%v", principal, err)
		}
	})
}
