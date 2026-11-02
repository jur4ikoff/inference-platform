package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/infrastructure"
	"github.com/jur4ikoff/user-service/internal/service"
	"golang.org/x/crypto/bcrypt"
)

func TestUpdateUserChangesProfileAndCredentials(t *testing.T) {
	ctx := context.Background()
	repo := &memoryUsers{}
	hasher := infrastructure.NewPasswordHasher(bcrypt.MinCost)
	clock := &fixedClock{now: time.Now()}
	users := service.NewUserService(repo, hasher, infrastructure.NewTokenManager("test-secret", "test"), infrastructure.IDGenerator{}, clock, time.Hour)
	registered, err := users.Register(ctx, service.RegisterInput{Firstname: " Иван ", Lastname: " Попов ", Email: "user@example.com", Password: "old-password-123"})
	if err != nil {
		t.Fatal(err)
	}
	before := repo.user
	clock.now = clock.now.Add(time.Minute)
	email, password, firstname, lastname := " NEW@EXAMPLE.COM ", "new-password-456", " Иван ", " Попов "
	updated, err := users.UpdateUser(ctx, registered.ID, service.UpdateUserInput{
		Email: &email, Password: &password, Firstname: &firstname, Lastname: &lastname,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Email != "new@example.com" || updated.Firstname != "Иван" || updated.Lastname != "Попов" {
		t.Fatalf("unexpected updated profile: %+v", updated)
	}
	if repo.user.PasswordHash == before.PasswordHash || repo.user.PasswordHash == password || hasher.Compare(repo.user.PasswordHash, password) != nil {
		t.Fatal("new password must be stored as a fresh bcrypt hash")
	}
	if updated.ID != before.ID || updated.CreatedAt != before.CreatedAt || !updated.UpdatedAt.Equal(clock.now) || updated.UpdatedAt.Location() != time.UTC {
		t.Fatal("update must preserve ID and creation time and update the timestamp in UTC")
	}
	if _, err := users.Login(ctx, service.LoginInput{Email: updated.Email, Password: "old-password-123"}); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("old password login error = %v", err)
	}
	token, err := users.Login(ctx, service.LoginInput{Email: updated.Email, Password: password})
	if err != nil {
		t.Fatal(err)
	}
	principal, err := users.Verify(ctx, token.Value)
	if err != nil || principal.UserID != updated.ID {
		t.Fatalf("updated credentials must allow login without a global role: %+v, %v", principal, err)
	}
	if firstname != " Иван " || lastname != " Попов " {
		t.Fatal("normalization changed the caller's input")
	}
	before = repo.user
	empty := ""
	if _, err := users.UpdateUser(ctx, registered.ID, service.UpdateUserInput{Firstname: &empty}); err != nil {
		t.Fatal(err)
	}
	want := before
	want.Firstname = ""
	if repo.user != want {
		t.Fatalf("partial update changed omitted fields: got %+v, want %+v", repo.user, want)
	}
}

func TestUpdateUserFailures(t *testing.T) {
	invalidEmail, short, long, multibyte := "invalid", "short", strings.Repeat("a", 73), strings.Repeat("я", 37)
	password := "new-password-456"
	storageErr, hashErr := errors.New("storage failure"), errors.New("hash failure")
	for _, test := range []struct {
		name      string
		input     service.UpdateUserInput
		updateErr error
		hashErr   error
		wantErr   error
		updates   int
	}{
		{name: "empty patch", wantErr: service.ErrInvalidInput},
		{name: "invalid email", input: service.UpdateUserInput{Email: &invalidEmail}, wantErr: service.ErrInvalidInput},
		{name: "short password", input: service.UpdateUserInput{Password: &short}, wantErr: service.ErrInvalidInput},
		{name: "long password", input: service.UpdateUserInput{Password: &long}, wantErr: service.ErrInvalidInput},
		{name: "multibyte password", input: service.UpdateUserInput{Password: &multibyte}, wantErr: service.ErrInvalidInput},
		{name: "hash failure", input: service.UpdateUserInput{Password: &password}, hashErr: hashErr, wantErr: hashErr},
		{name: "duplicate email", input: service.UpdateUserInput{Password: &password}, updateErr: domain.ErrEmailAlreadyTaken, wantErr: domain.ErrEmailAlreadyTaken, updates: 1},
		{name: "missing user", input: service.UpdateUserInput{Password: &password}, updateErr: domain.ErrUserNotFound, wantErr: domain.ErrUserNotFound, updates: 1},
		{name: "storage failure", input: service.UpdateUserInput{Password: &password}, updateErr: storageErr, wantErr: storageErr, updates: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := domain.User{ID: "user-id", Email: "user@example.com", PasswordHash: "old-hash", Firstname: "Иван"}
			repo := &memoryUsers{user: before, updateErr: test.updateErr}
			var hasher service.PasswordHasher = infrastructure.NewPasswordHasher(bcrypt.MinCost)
			if test.hashErr != nil {
				hasher = &failingHasher{PasswordHasher: hasher, err: test.hashErr}
			}
			users := service.NewUserService(repo, hasher, nil, nil, &fixedClock{now: time.Now()}, 0)
			_, err := users.UpdateUser(context.Background(), before.ID, test.input)
			if !errors.Is(err, test.wantErr) || repo.updates != test.updates || repo.user != before {
				t.Fatalf("error = %v, writes = %d; want %v, %d with unchanged user", err, repo.updates, test.wantErr, test.updates)
			}
		})
	}
}

type memoryUsers struct {
	user       domain.User
	findErr    error
	updateErr  error
	updates    int
	createErr  error
	listUsers  []domain.User
	listErr    error
	listCalls  int
	listOffset int
	listLimit  int
}

func (r *memoryUsers) Create(_ context.Context, user domain.User) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.user = user
	return nil
}

func (r *memoryUsers) List(_ context.Context, offset, limit int) ([]domain.User, error) {
	r.listCalls++
	r.listOffset, r.listLimit = offset, limit
	if r.listErr != nil {
		return nil, r.listErr
	}
	if offset >= len(r.listUsers) {
		return nil, nil
	}
	end := min(offset+limit, len(r.listUsers))
	return r.listUsers[offset:end], nil
}

func (r *memoryUsers) FindByEmail(_ context.Context, email string) (domain.User, error) {
	if r.findErr != nil {
		return domain.User{}, r.findErr
	}
	if r.user.Email != email {
		return domain.User{}, domain.ErrUserNotFound
	}
	return r.user, nil
}

func (r *memoryUsers) Update(_ context.Context, id domain.UserID, changes domain.UserUpdate, updatedAt time.Time) (domain.User, error) {
	r.updates++
	if r.updateErr != nil {
		return domain.User{}, r.updateErr
	}
	if r.user.ID != id {
		return domain.User{}, domain.ErrUserNotFound
	}
	if changes.Email != nil {
		r.user.Email = *changes.Email
	}
	if changes.PasswordHash != nil {
		r.user.PasswordHash = *changes.PasswordHash
	}
	if changes.Firstname != nil {
		r.user.Firstname = *changes.Firstname
	}
	if changes.Lastname != nil {
		r.user.Lastname = *changes.Lastname
	}

	r.user.UpdatedAt = updatedAt
	return r.user, nil
}

func (r *memoryUsers) Delete(context.Context, domain.UserID) error { return nil }

type fixedClock struct{ now time.Time }

func (c *fixedClock) Now() time.Time { return c.now }

type failingHasher struct {
	service.PasswordHasher
	err error
}

func (h *failingHasher) Hash(string) (string, error) { return "", h.err }
