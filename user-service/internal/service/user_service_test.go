package service_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/infrastructure"
	"github.com/jur4ikoff/user-service/internal/service"
	"github.com/jur4ikoff/user-service/internal/testutil"
	"github.com/jur4ikoff/utils/authjwt"
	"golang.org/x/crypto/bcrypt"
	"strings"
	"testing"
	"time"
)

// Техника: граничные значения длины пароля.
func TestPasswordMinimumLength(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {

		for _, tc := range []struct {
			password string
			valid    bool
		}{{"1234567", false}, {"12345678", true}, {"пароль12", true}} {
			t.Run(tc.password, func(t *testing.T) {
				allure.Wrap(t, func(allureCtx *allure.Context) {

					repository := &testutil.MockUserRepository{CreateFunc: func(context.Context, domain.User) error { return nil }}
					svc := service.NewUserService(repository, &testutil.MockPasswordHasher{HashFunc: func(string) (string, error) { return "hash", nil }}, nil, &testutil.MockIDGenerator{NewUserIDFunc: func() domain.UserID { return "id" }}, &testutil.MockClock{NowFunc: func() time.Time { return time.Now() }}, time.Minute)
					_, err := svc.Register(context.Background(), service.RegisterInput{Login: "alice", Email: "alice@example.com", Firstname: "Alice", Lastname: "Example", Password: tc.password})
					if tc.valid && err != nil || !tc.valid && !errors.Is(err, service.ErrInvalidInput) {
						allureCtx.Fatalf("err=%v", err)
					}

				})
			})
		}

	})
}

// Техника: переходы состояний профиля и учётных данных.
func TestUpdateUserChangesProfileAndCredentials(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {

		ctx := context.Background()
		repo := &memoryUsers{}
		hasher := infrastructure.NewPasswordHasher(bcrypt.MinCost)
		clock := &fixedClock{now: time.Now().Add(-time.Minute)}
		private, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			allureCtx.Fatal(err)
		}
		tokens, err := infrastructure.NewTokenManager(private, "test-key", "test", "platform")
		if err != nil {
			allureCtx.Fatal(err)
		}
		users := service.NewUserService(repo, hasher, tokens, infrastructure.IDGenerator{}, clock, time.Hour)
		registered, err := users.Register(ctx, service.RegisterInput{Login: "ivan", Firstname: " Юрий ", Lastname: " Попов ", Email: "user@example.com", Password: "old-password-123"})
		if err != nil {
			allureCtx.Fatal(err)
		}
		before := repo.user
		clock.now = clock.now.Add(time.Minute)
		email, password, firstname, lastname := " NEW@EXAMPLE.COM ", "new-password-456", " Юрий ", " Попов "
		updated, err := users.UpdateUser(ctx, registered.ID, service.UpdateUserInput{
			Email: &email, Password: &password, Firstname: &firstname, Lastname: &lastname,
		})
		if err != nil {
			allureCtx.Fatal(err)
		}
		if updated.Email != "new@example.com" || updated.Firstname != "Юрий" || updated.Lastname != "Попов" {
			allureCtx.Fatalf("unexpected updated profile: %+v", updated)
		}
		if repo.user.PasswordHash == before.PasswordHash || repo.user.PasswordHash == password || hasher.Compare(repo.user.PasswordHash, password) != nil {
			allureCtx.Fatal("new password must be stored as a fresh bcrypt hash")
		}
		if updated.ID != before.ID || updated.CreatedAt != before.CreatedAt || !updated.UpdatedAt.Equal(clock.now) || updated.UpdatedAt.Location() != time.UTC {
			allureCtx.Fatal("update must preserve ID and creation time and update the timestamp in UTC")
		}
		if firstname != " Юрий " || lastname != " Попов " {
			allureCtx.Fatal("normalization changed the caller's input")
		}

	})
}

// Техника: классы эквивалентности ошибочных входных данных и ошибок зависимостей.
func TestUpdateUserFailures(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {

		invalidEmail, short, long, multibyte := "invalid", "short", strings.Repeat("a", 73), strings.Repeat("я", 73)
		password := "new-password-456"
		blankFirstname := ""
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
			{name: "blank firstname", input: service.UpdateUserInput{Firstname: &blankFirstname}, wantErr: service.ErrInvalidInput},
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
				allure.Wrap(t, func(allureCtx *allure.Context) {

					before := domain.User{ID: "user-id", Email: "user@example.com", PasswordHash: "old-hash", Firstname: "Юрий"}
					repo := &memoryUsers{user: before, updateErr: test.updateErr}
					var hasher service.PasswordHasher = infrastructure.NewPasswordHasher(bcrypt.MinCost)
					if test.hashErr != nil {
						hasher = &failingHasher{PasswordHasher: hasher, err: test.hashErr}
					}
					users := service.NewUserService(repo, hasher, nil, nil, &fixedClock{now: time.Now()}, 0)
					_, err := users.UpdateUser(context.Background(), before.ID, test.input)
					if !errors.Is(err, test.wantErr) || repo.updates != test.updates || repo.user != before {
						allureCtx.Fatalf("error = %v, writes = %d; want %v, %d with unchanged user", err, repo.updates, test.wantErr, test.updates)
					}

				})
			})
		}

	})
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

func (r *memoryUsers) FindByLogin(_ context.Context, login string) (domain.User, error) {
	if r.findErr != nil {
		return domain.User{}, r.findErr
	}
	if r.user.Login != login {
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

type visibleRepo struct {
	*testutil.MockUserRepository
	user domain.User
}

func (r *visibleRepo) GetVisible(context.Context, domain.UserID, domain.UserID) (domain.User, error) {
	return r.user, nil
}
func (r *visibleRepo) ActorRole(context.Context, domain.UserID) (string, error) { return "user", nil }

type tokenManager struct {
	*testutil.MockTokenManager
	err error
}

func (m *tokenManager) PublicJWKSet() (authjwt.JWKSet, error) { return authjwt.JWKSet{}, m.err }

// Техника: классы эквивалентности данных и прав доступа.
func TestUserUseCases(t *testing.T) {
	allure.Wrap(t, func(allureCtx *allure.Context) {

		for _, method := range []string{"register", "login", "verify", "keys", "get", "get login", "authorize", "list", "delete"} {
			for _, valid := range []bool{true, false} {
				t.Run(method+map[bool]string{true: "/valid", false: "/invalid"}[valid], func(t *testing.T) {
					allure.Wrap(t, func(allureCtx *allure.Context) {

						u := testutil.UserMother{}.Regular()
						failure := errors.New("failure")
						repo := &visibleRepo{MockUserRepository: &testutil.MockUserRepository{CreateFunc: func(context.Context, domain.User) error { return nil }, FindByLoginFunc: func(context.Context, string) (domain.User, error) { return u, nil }, ListFunc: func(context.Context, int, int) ([]domain.User, error) { return []domain.User{u}, nil }, DeleteFunc: func(context.Context, domain.UserID) error { return nil }}, user: u}
						hash := &testutil.MockPasswordHasher{HashFunc: func(string) (string, error) { return "hash", nil }, CompareFunc: func(_, password string) error {
							if password != "valid-password" {
								return failure
							}
							return nil
						}}
						tokens := &tokenManager{MockTokenManager: &testutil.MockTokenManager{CreateAccessTokenFunc: func(service.Session, *authjwt.ProjectGrant, time.Time) (string, error) { return "token", nil }, ParseAccessTokenFunc: func(raw string) (service.Principal, error) {
							if raw != "token" {
								return service.Principal{}, failure
							}
							return service.Principal{UserID: u.ID}, nil
						}}}
						ids := &testutil.MockIDGenerator{NewUserIDFunc: func() domain.UserID { return u.ID }}
						clock := &testutil.MockClock{NowFunc: func() time.Time { return time.Now() }}
						svc := service.NewUserService(repo, hash, tokens, ids, clock, time.Minute)
						ctx := context.Background()
						auth := service.WithPrincipal(ctx, service.Principal{UserID: u.ID})
						var err, want error
						switch method {
						case "register":
							input := service.RegisterInput{Login: "alice", Email: u.Email, Firstname: u.Firstname, Lastname: u.Lastname, Password: "valid-password"}
							if !valid {
								input.Login = "!"
								want = service.ErrInvalidInput
							}
							_, err = svc.Register(ctx, input)
						case "login":
							input := service.LoginInput{Login: "alice", Password: "valid-password"}
							if !valid {
								input.Password = "bad"
								want = service.ErrInvalidCredentials
							}
							_, err = svc.Login(ctx, input)
						case "verify":
							raw := "token"
							if !valid {
								raw = "bad"
								want = service.ErrInvalidToken
							}
							_, err = svc.Verify(ctx, raw)
						case "keys":
							if !valid {
								tokens.err = failure
								want = failure
							}
							_, err = svc.PublicJWKSet()
						case "get":
							if !valid {
								auth = ctx
								want = service.ErrInvalidToken
							}
							_, err = svc.GetUser(auth, u.ID)
						case "get login":
							login := "alice"
							if !valid {
								login = "!"
								want = service.ErrInvalidToken
							}
							_, err = svc.GetUserByLogin(auth, login)
						case "authorize":
							target := u.ID
							if !valid {
								target = "other"
								want = service.ErrForbidden
							}
							err = svc.AuthorizeUser(auth, target)
						case "list":
							limit := 1
							if !valid {
								limit = 101
								want = service.ErrInvalidInput
							}
							_, err = svc.ListUsers(ctx, service.ListUsersInput{Limit: limit})
						case "delete":
							if !valid {
								auth = service.WithPrincipal(ctx, service.Principal{UserID: "other"})
								want = service.ErrForbidden
							}
							err = svc.DeleteUser(auth, u.ID)
						}
						if !errors.Is(err, want) {
							allureCtx.Fatalf("err=%v want=%v", err, want)
						}

					})
				})
			}
		}

	})
}
