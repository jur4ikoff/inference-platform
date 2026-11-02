package service_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/infrastructure"
	"github.com/jur4ikoff/user-service/internal/service"
	"golang.org/x/crypto/bcrypt"
)

func TestRegisterStoresNormalizedEmailAndHashedPassword(t *testing.T) {
	repo := &memoryUsers{}
	hasher := infrastructure.NewPasswordHasher(bcrypt.MinCost)
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	users := service.NewUserService(repo, hasher, nil, infrastructure.IDGenerator{}, &fixedClock{now: now}, time.Hour)
	registered, err := users.Register(context.Background(), service.RegisterInput{Firstname: " Иван ", Lastname: " Попов ", Email: " USER@EXAMPLE.COM ", Password: "password-12345"})
	if err != nil {
		t.Fatal(err)
	}
	if registered.ID == "" || registered.ID != repo.user.ID || registered.Email != "user@example.com" {
		t.Fatalf("unexpected registered user: %#v", registered)
	}
	if registered.Firstname != "Иван" || registered.Lastname != "Попов" || repo.user.Firstname != registered.Firstname || repo.user.Lastname != registered.Lastname {
		t.Fatal("registration must preserve normalized first and last names")
	}
	if repo.user.PasswordHash == "password-12345" || hasher.Compare(repo.user.PasswordHash, "password-12345") != nil {
		t.Fatal("password must be stored as a bcrypt hash")
	}
	if !repo.user.CreatedAt.Equal(now) || repo.user.CreatedAt.Location() != time.UTC || repo.user.UpdatedAt != repo.user.CreatedAt {
		t.Fatal("registration timestamps must use UTC")
	}
}

func TestRegisterRequiresNames(t *testing.T) {
	for _, names := range [][2]string{{"", "Попов"}, {"Иван", ""}, {" \t", "Попов"}, {"Иван", "\n "}} {
		repo := &memoryUsers{}
		users := service.NewUserService(repo, nil, nil, nil, nil, 0)
		_, err := users.Register(context.Background(), service.RegisterInput{
			Firstname: names[0], Lastname: names[1], Email: "user@example.com", Password: "password-12345",
		})
		if !errors.Is(err, service.ErrInvalidInput) || repo.user.ID != "" {
			t.Fatalf("incomplete registration: error=%v, saved user=%+v", err, repo.user)
		}
	}
}

func TestRegisterPropagatesDuplicateEmail(t *testing.T) {
	repo := &memoryUsers{createErr: domain.ErrEmailAlreadyTaken}
	users := service.NewUserService(repo, infrastructure.NewPasswordHasher(bcrypt.MinCost), nil, infrastructure.IDGenerator{}, &fixedClock{now: time.Now()}, time.Hour)
	_, err := users.Register(context.Background(), service.RegisterInput{Firstname: " Иван ", Lastname: " Попов ", Email: "user@example.com", Password: "password-12345"})
	if !errors.Is(err, domain.ErrEmailAlreadyTaken) {
		t.Fatalf("error = %v, want duplicate email", err)
	}
}

func TestListUsers(t *testing.T) {
	storageErr := errors.New("storage failure")
	for _, tt := range []struct {
		name  string
		users []domain.User
		err   error
		want  []service.RegisteredUser
	}{
		{"users", []domain.User{
			{ID: "first", Email: "user@example.com", PasswordHash: "private hash"},
			{ID: "second", Email: "admin@example.com", PasswordHash: "another hash"},
		}, nil, []service.RegisteredUser{
			{ID: "first", Email: "user@example.com"},
			{ID: "second", Email: "admin@example.com"},
		}},
		{"empty", nil, nil, []service.RegisteredUser{}},
		{"storage error", nil, storageErr, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &memoryUsers{listUsers: tt.users, listErr: tt.err}
			users := service.NewUserService(repo, nil, nil, nil, nil, 0)
			got, err := users.ListUsers(context.Background(), service.ListUsersInput{})
			if !errors.Is(err, tt.err) || !reflect.DeepEqual(got.Users, tt.want) {
				t.Fatalf("users = %#v, error = %v; want %#v, %v", got.Users, err, tt.want, tt.err)
			}
		})
	}
}
