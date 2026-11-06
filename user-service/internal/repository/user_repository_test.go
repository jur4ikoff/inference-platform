package repository_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jur4ikoff/user-service/internal/api"
	"github.com/jur4ikoff/user-service/internal/api/handler"
	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/infrastructure"
	"github.com/jur4ikoff/user-service/internal/repository"
	"github.com/jur4ikoff/user-service/internal/service"
	"github.com/jur4ikoff/user-service/internal/testutil"
	"golang.org/x/crypto/bcrypt"
)

// Техника: классы эквивалентности прав, ограничений уникальности и переходов состояний пользователя.
func TestUserRepositoryIntegration(t *testing.T) {
	dsn := os.Getenv("USER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("USER_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	schema := "user_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer root.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, migration := range []string{
		"000001_create_user_table.sql", "000002_create_project_table.sql",
		"000003_create_roles_table.sql", "000004_create_members_table.sql",
		"000005_create_models_table.sql",
	} {
		ddl, err := os.ReadFile("../../../infra/postgres/migrations/" + migration)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(ddl)); err != nil {
			t.Fatalf("apply %s: %v", migration, err)
		}
	}
	repo := repository.NewUserRepository(pool)
	now := time.Now().UTC()
	bootstrapCandidate := domain.User{ID: domain.UserID(uuid.NewString()), Login: "bootstrap", Email: "bootstrap@example.com", Firstname: "Bootstrap", Lastname: "Admin", PasswordHash: "first-hash", PlatformRole: domain.PlatformRoleAdmin, CreatedAt: now, UpdatedAt: now}
	bootstrapped, existed, err := repo.BootstrapAdmin(ctx, bootstrapCandidate)
	if err != nil || existed || bootstrapped.PlatformRole != domain.PlatformRoleAdmin {
		t.Fatalf("bootstrap admin: user=%+v existed=%t err=%v", bootstrapped, existed, err)
	}
	retry := bootstrapCandidate
	retry.ID, retry.PasswordHash = domain.UserID(uuid.NewString()), "must-not-replace"
	bootstrapped, existed, err = repo.BootstrapAdmin(ctx, retry)
	if err != nil || !existed || bootstrapped.PasswordHash != "first-hash" {
		t.Fatalf("bootstrap retry: user=%+v existed=%t err=%v", bootstrapped, existed, err)
	}
	actor := domain.User{ID: domain.UserID(uuid.NewString()), Login: "first", Email: "first@example.com", Firstname: "First", Lastname: "User", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	other := actor
	other.ID = domain.UserID(uuid.NewString())
	other.Email = "other@example.com"
	other.Login = "other"
	for _, u := range []domain.User{actor, other} {
		if err = repo.Create(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	// A model keeps its author snapshot after the user's account is deleted.
	former := actor
	former.ID = domain.UserID(uuid.NewString())
	former.Email = "former@example.com"
	former.Login = "former"
	if err = repo.Create(ctx, former); err != nil {
		t.Fatal(err)
	}
	modelID := uuid.New()
	if _, err = pool.Exec(ctx, `INSERT INTO models (id, created_at, updated_at, name, description, username, project) VALUES ($1, now(), now(), 'model', '', $2, 'project')`, modelID, former.Login); err != nil {
		t.Fatal(err)
	}
	if err = repo.Delete(ctx, former.ID); err != nil {
		t.Fatalf("delete model author: %v", err)
	}
	var savedUsername string
	if err = pool.QueryRow(ctx, `SELECT username FROM models WHERE id=$1`, modelID).Scan(&savedUsername); err != nil || savedUsername != former.Login {
		t.Fatalf("model after author deletion: username=%q error=%v", savedUsername, err)
	}
	duplicate := other
	duplicate.ID = domain.UserID(uuid.NewString())
	duplicate.Email = "FIRST@example.com"
	duplicate.Login = "duplicate"
	if err = repo.Create(ctx, duplicate); !errors.Is(err, domain.ErrEmailAlreadyTaken) {
		t.Fatalf("duplicate: %v", err)
	}
	// Concurrent duplicate registrations must be decided by the database constraint.
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		candidate := actor
		candidate.ID = domain.UserID(uuid.NewString())
		candidate.Email = "race@example.com"
		candidate.Login = fmt.Sprintf("race%d", i)
		go func() { results <- repo.Create(ctx, candidate) }()
	}
	successes, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		e := <-results
		if e == nil {
			successes++
		} else if errors.Is(e, domain.ErrEmailAlreadyTaken) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent duplicate: success=%d conflict=%d", successes, conflicts)
	}
	users, err := repo.ListVisible(ctx, actor.ID, 0, 10)
	if err != nil || len(users) != 1 || users[0].ID != actor.ID {
		t.Fatalf("visibility: %+v %v", users, err)
	}
	if _, err = repo.GetVisible(ctx, actor.ID, other.ID); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("foreign profile: %v", err)
	}
	project := uuid.New()
	_, err = pool.Exec(ctx, "INSERT INTO projects(id,created_at,updated_at,name,description) VALUES ($1,now(),now(),'demo','')", project)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []domain.User{actor, other} {
		_, err = pool.Exec(ctx, "INSERT INTO members(id,created_at,updated_at,user_id,project_id,role_id) SELECT $1,now(),now(),$2,$3,id FROM roles WHERE system_key='teamlead'", uuid.New(), string(u.ID), project)
		if err != nil {
			t.Fatal(err)
		}
	}
	users, err = repo.ListVisible(ctx, actor.ID, 0, 10)
	if err != nil || len(users) != 2 {
		t.Fatalf("shared visibility: %+v %v", users, err)
	}
	// A failed audit must roll the update back.
	_, err = pool.Exec(ctx, `CREATE FUNCTION reject_user_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action = 'user.updated' THEN RAISE EXCEPTION 'audit unavailable'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_user_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_user_audit()`)
	if err != nil {
		t.Fatal(err)
	}
	blocked := "MustRollback"
	if _, err = repo.Update(ctx, other.ID, domain.UserUpdate{Firstname: &blocked}, now); err == nil {
		t.Fatal("update unexpectedly succeeded without audit")
	}
	unchanged, err := repo.FindByEmail(ctx, other.Email)
	if err != nil || unchanged.Firstname != other.Firstname {
		t.Fatalf("audit rollback: %+v %v", unchanged, err)
	}
	_, err = pool.Exec(ctx, `DROP TRIGGER reject_user_audit ON audit_events; DROP FUNCTION reject_user_audit()`)
	if err != nil {
		t.Fatal(err)
	}
	name := "Changed"
	updated, err := repo.Update(ctx, actor.ID, domain.UserUpdate{Firstname: &name}, now.Add(time.Second))
	if err != nil || updated.Firstname != name {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if err = repo.Delete(ctx, actor.ID); err != nil {
		t.Fatal(err)
	}
	if err = repo.Delete(ctx, other.ID); !errors.Is(err, domain.ErrLastTeamlead) {
		t.Fatalf("last teamlead: %v", err)
	}

	// Concurrent deletion of the final administrators must preserve one.
	// The bootstrap scenario created an administrator earlier in this test.
	// Leave only the two accounts below as administrators for this scenario.
	if _, err = pool.Exec(ctx, "UPDATE users SET platform_role='user' WHERE id=$1", string(bootstrapped.ID)); err != nil {
		t.Fatal(err)
	}
	admins := make([]domain.User, 2)
	for i := range admins {
		admins[i] = actor
		admins[i].ID = domain.UserID(uuid.NewString())
		admins[i].Email = uuid.NewString() + "@example.com"
		admins[i].Login = fmt.Sprintf("admin%d", i)
		if err = repo.Create(ctx, admins[i]); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, "UPDATE users SET platform_role='admin' WHERE id=$1", string(admins[i].ID)); err != nil {
			t.Fatal(err)
		}
	}
	adminResults := make(chan error, 2)
	for _, u := range admins {
		go func(id domain.UserID) { adminResults <- repo.Delete(ctx, id) }(u.ID)
	}
	deleted, protected := 0, 0
	for i := 0; i < 2; i++ {
		e := <-adminResults
		if e == nil {
			deleted++
		} else if errors.Is(e, domain.ErrLastAdministrator) {
			protected++
		} else {
			t.Fatal(e)
		}
	}
	if deleted != 1 || protected != 1 {
		t.Fatalf("concurrent admins: deleted=%d protected=%d", deleted, protected)
	}
	if _, err = pool.Exec(ctx, "UPDATE users SET platform_role='user' WHERE platform_role='admin'"); err != nil {
		t.Fatal(err)
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := infrastructure.NewTokenManager(privateKey, "integration-key", "integration", "platform")
	if err != nil {
		t.Fatal(err)
	}
	auth := service.NewUserService(repo, infrastructure.NewPasswordHasher(bcrypt.MinCost), tokens, infrastructure.IDGenerator{}, infrastructure.Clock{}, time.Hour)
	projects := service.NewProjectService(repository.NewProjectRepository(pool))
	roles := service.NewRoleService(repository.NewRoleRepository(pool))
	platformRoles := service.NewPlatformRoleService(repo)
	router := api.NewRouter(handler.NewAuthHandler(auth), handler.NewUserHandler(auth, platformRoles), handler.NewProjectHandler(projects), handler.NewRoleHandler(roles), "test")
	request := func(method, path, body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		return resp
	}
	_, err = pool.Exec(ctx, `CREATE FUNCTION normalize_registration() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.email = 'web@example.com' THEN NEW.firstname = upper(NEW.firstname); END IF; RETURN NEW; END $$; CREATE TRIGGER normalize_registration BEFORE INSERT ON users FOR EACH ROW EXECUTE FUNCTION normalize_registration()`)
	if err != nil {
		t.Fatal(err)
	}
	registered := request(http.MethodPost, "/api/v1/auth/register", `{"login":"web","firstname":"Web","lastname":"User","email":"web@example.com","password":"password-12345"}`, "")
	if registered.Code != 201 {
		t.Fatalf("register: %d %s", registered.Code, registered.Body.String())
	}
	if !strings.Contains(registered.Body.String(), `"firstname":"WEB"`) {
		t.Fatalf("registration did not return the saved row: %s", registered.Body.String())
	}
	if _, err = pool.Exec(ctx, `DROP TRIGGER normalize_registration ON users; DROP FUNCTION normalize_registration()`); err != nil {
		t.Fatal(err)
	}
	var profile struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(registered.Body.Bytes(), &profile); err != nil {
		t.Fatal(err)
	}
	login := request(http.MethodPost, "/api/v1/auth/login", `{"login":"web","password":"password-12345"}`, "")
	if login.Code != 200 {
		t.Fatalf("login: %d %s", login.Code, login.Body.String())
	}
	var token struct {
		Value string `json:"access_token"`
	}
	if err = json.Unmarshal(login.Body.Bytes(), &token); err != nil {
		t.Fatal(err)
	}
	// Exercise project and membership authorization through HTTP and PostgreSQL.
	check := func(method, path, body, bearer string, want int) *httptest.ResponseRecorder {
		t.Helper()
		response := request(method, path, body, bearer)
		if response.Code != want {
			t.Fatalf("%s %s: got %d, want %d: %s", method, path, response.Code, want, response.Body.String())
		}
		return response
	}
	otherToken, err := tokens.CreateAccessToken(service.Session{UserID: other.ID, Username: other.Login, PlatformRole: domain.PlatformRoleUser, AuthTime: time.Now().UTC()}, nil, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	// Database role changes invalidate the old role snapshot in a JWT.
	issueCurrentToken := func(id domain.UserID, username string) string {
		t.Helper()
		state, stateErr := repo.GetAuthState(ctx, id)
		if stateErr != nil {
			t.Fatal(stateErr)
		}
		now := time.Now().UTC()
		value, tokenErr := tokens.CreateAccessToken(service.Session{
			UserID: id, Username: username, PlatformRole: state.PlatformRole,
			AuthVersion: state.AuthVersion, AuthTime: now,
		}, nil, now.Add(time.Minute))
		if tokenErr != nil {
			t.Fatal(tokenErr)
		}
		return value
	}
	check(http.MethodPost, "/api/v1/projects", `{"name":"invalid_name","description":"x"}`, token.Value, 400)
	if _, err = pool.Exec(ctx, "UPDATE roles SET system_key=NULL WHERE system_key='teamlead'"); err != nil {
		t.Fatal(err)
	}
	check(http.MethodPost, "/api/v1/projects", `{"name":"missing-role","description":"x"}`, token.Value, 500)
	if _, err = pool.Exec(ctx, "UPDATE roles SET system_key='teamlead' WHERE name='teamlead'"); err != nil {
		t.Fatal(err)
	}
	var missingCount int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM projects WHERE name='missing-role'").Scan(&missingCount); err != nil || missingCount != 0 {
		t.Fatalf("project without teamlead was persisted: %d %v", missingCount, err)
	}
	check(http.MethodPost, "/api/v1/projects", `{"name":"sprint-two","description":"first"}`, token.Value, 201)
	check(http.MethodPost, "/api/v1/projects", `{"name":"sprint-two","description":"again"}`, token.Value, 409)
	check(http.MethodGet, "/api/v1/projects/sprint-two", "", otherToken, 404)
	missingUser := uuid.NewString()
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		body := ""
		if method == http.MethodPut {
			body = `{"role_id":1}`
		}
		for _, project := range []string{"sprint-two", "no-such-project"} {
			existing := check(method, "/api/v1/projects/"+project+"/members/"+profile.ID, body, otherToken, 404)
			missing := check(method, "/api/v1/projects/"+project+"/members/"+missingUser, body, otherToken, 404)
			if existing.Body.String() != missing.Body.String() {
				t.Fatalf("%s %s leaked user existence: %s vs %s", method, project, existing.Body, missing.Body)
			}
		}
	}
	if _, err = pool.Exec(ctx, "UPDATE users SET platform_role='analyst' WHERE id=$1", string(other.ID)); err != nil {
		t.Fatal(err)
	}
	otherToken = issueCurrentToken(other.ID, other.Login)
	check(http.MethodGet, "/api/v1/projects/sprint-two", "", otherToken, 200)
	check(http.MethodGet, "/api/v1/projects/sprint-two/members", "", otherToken, 403)
	check(http.MethodPut, "/api/v1/projects/sprint-two/members/"+profile.ID, `{"role_id":1}`, otherToken, 403)
	check(http.MethodPut, "/api/v1/projects/sprint-two/members/"+missingUser, `{"role_id":1}`, otherToken, 403)
	check(http.MethodPatch, "/api/v1/projects/sprint-two", `{"description":"no"}`, otherToken, 403)
	if _, err = pool.Exec(ctx, "UPDATE users SET platform_role='user' WHERE id=$1", string(other.ID)); err != nil {
		t.Fatal(err)
	}
	otherToken = issueCurrentToken(other.ID, other.Login)
	var teamlead, developer int
	if err = pool.QueryRow(ctx, "SELECT id FROM roles WHERE system_key='teamlead'").Scan(&teamlead); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "SELECT id FROM roles WHERE system_key='developer'").Scan(&developer); err != nil {
		t.Fatal(err)
	}
	otherPath := "/api/v1/projects/sprint-two/members/" + string(other.ID)
	check(http.MethodPut, "/api/v1/projects/sprint-two/members/"+missingUser, fmt.Sprintf(`{"role_id":%d}`, developer), token.Value, 404)
	check(http.MethodDelete, "/api/v1/projects/sprint-two/members/"+profile.ID, "", token.Value, 409)
	check(http.MethodDelete, "/api/v1/users/"+profile.ID, "", token.Value, 409)
	check(http.MethodPut, otherPath, fmt.Sprintf(`{"role_id":%d}`, developer), token.Value, 201)
	check(http.MethodPut, otherPath, fmt.Sprintf(`{"role_id":%d}`, developer), token.Value, 200)
	var memberAuditCount int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE action='member.assigned' AND subject_id=$1 AND details->>'role_id'=$2", string(other.ID), fmt.Sprint(developer)).Scan(&memberAuditCount); err != nil || memberAuditCount != 2 {
		t.Fatalf("member audit: %d %v", memberAuditCount, err)
	}
	check(http.MethodGet, "/api/v1/projects/sprint-two/members?limit=1", "", otherToken, 200)
	check(http.MethodPatch, "/api/v1/projects/sprint-two", `{"description":"no"}`, otherToken, 403)
	check(http.MethodGet, "/api/v1/users/"+string(other.ID), "", token.Value, 200)
	check(http.MethodPut, otherPath, fmt.Sprintf(`{"role_id":%d}`, teamlead), token.Value, 200)
	check(http.MethodDelete, "/api/v1/projects/sprint-two/members/"+profile.ID, "", token.Value, 204)
	check(http.MethodDelete, otherPath, "", otherToken, 409)
	check(http.MethodPatch, "/api/v1/projects/sprint-two", `{"name":"renamed"}`, otherToken, 422)
	if _, err = pool.Exec(ctx, `CREATE FUNCTION reject_project_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action = 'project.updated' THEN RAISE EXCEPTION 'audit unavailable'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_project_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_project_audit()`); err != nil {
		t.Fatal(err)
	}
	check(http.MethodPatch, "/api/v1/projects/sprint-two", `{"description":"must roll back"}`, otherToken, 500)
	if result := check(http.MethodGet, "/api/v1/projects/sprint-two", "", otherToken, 200); !strings.Contains(result.Body.String(), `"description":"first"`) {
		t.Fatalf("project update was not rolled back: %s", result.Body.String())
	}
	if _, err = pool.Exec(ctx, `DROP TRIGGER reject_project_audit ON audit_events; DROP FUNCTION reject_project_audit()`); err != nil {
		t.Fatal(err)
	}
	check(http.MethodPatch, "/api/v1/projects/sprint-two", `{"description":""}`, otherToken, 200)
	check(http.MethodDelete, "/api/v1/projects/sprint-two", "", otherToken, 204)
	check(http.MethodGet, "/api/v1/projects/sprint-two", "", otherToken, 404)
	check(http.MethodPost, "/api/v1/projects", `{"name":"sprint-two","description":"again"}`, token.Value, 409)
	check(http.MethodPost, "/api/v1/projects", `{"name":"race-two","description":""}`, token.Value, 201)
	check(http.MethodPut, "/api/v1/projects/race-two/members/"+string(other.ID), fmt.Sprintf(`{"role_id":%d}`, teamlead), token.Value, 201)
	resultsRace := make(chan int, 2)
	go func() {
		resultsRace <- request(http.MethodDelete, "/api/v1/projects/race-two/members/"+profile.ID, "", token.Value).Code
	}()
	go func() {
		resultsRace <- request(http.MethodDelete, "/api/v1/projects/race-two/members/"+string(other.ID), "", otherToken).Code
	}()
	first, second := <-resultsRace, <-resultsRace
	if !((first == 204 && second == 409) || (first == 409 && second == 204)) {
		t.Fatalf("concurrent teamlead removal: %d, %d", first, second)
	}
	var remainingTeamleads int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM members m JOIN projects p ON p.id=m.project_id JOIN roles r ON r.id=m.role_id WHERE p.name='race-two' AND r.system_key='teamlead'").Scan(&remainingTeamleads); err != nil || remainingTeamleads != 1 {
		t.Fatalf("remaining teamleads: %d %v", remainingTeamleads, err)
	}
	if _, err = pool.Exec(ctx, "UPDATE projects SET deleted_at=now() WHERE name='race-two'"); err != nil {
		t.Fatal(err)
	}
	get := request(http.MethodGet, "/api/v1/users/"+profile.ID, "", token.Value)
	if get.Code != 200 || get.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("get: %d %s", get.Code, get.Body.String())
	}
	list := request(http.MethodGet, "/api/v1/users?offset=0&limit=2", "", token.Value)
	var listBody struct {
		Users []struct {
			ID string `json:"id"`
		} `json:"users"`
		Next *int `json:"nextpagetoken"`
	}
	if list.Code != 200 {
		t.Fatalf("list: %d %s", list.Code, list.Body.String())
	}
	if err = json.Unmarshal(list.Body.Bytes(), &listBody); err != nil {
		t.Fatal(err)
	}
	if len(listBody.Users) != 1 || listBody.Users[0].ID != profile.ID || listBody.Next != nil {
		t.Fatalf("list scope: %+v", listBody)
	}
	foreign := request(http.MethodGet, "/api/v1/users/"+string(other.ID), "", token.Value)
	if foreign.Code != 404 {
		t.Fatalf("foreign: %d %s", foreign.Code, foreign.Body.String())
	}

	for _, u := range users {
		if u.ID == actor.ID && u.PlatformRole != domain.PlatformRoleUser {
			t.Fatalf("own role: %q", u.PlatformRole)
		}
		if u.ID == other.ID && u.PlatformRole != "" {
			t.Fatalf("colleague role leaked: %q", u.PlatformRole)
		}
	}
	forbidden := request(http.MethodPatch, "/api/v1/users/"+string(other.ID), `{"firstname":"forbidden"}`, token.Value)
	if forbidden.Code != 403 {
		t.Fatalf("foreign update: %d %s", forbidden.Code, forbidden.Body.String())
	}
	actorCtx := service.WithPrincipal(ctx, service.Principal{UserID: domain.UserID(profile.ID)})
	forbiddenName := "forbidden"
	if _, err = auth.UpdateUser(actorCtx, other.ID, service.UpdateUserInput{Firstname: &forbiddenName}); !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("service authorization: %v", err)
	}
	if _, err = pool.Exec(ctx, "UPDATE users SET platform_role='admin' WHERE id=$1", profile.ID); err != nil {
		t.Fatal(err)
	}
	token.Value = issueCurrentToken(domain.UserID(profile.ID), "web")
	adminView := request(http.MethodGet, "/api/v1/users/"+string(other.ID), "", token.Value)
	if adminView.Code != 200 || !strings.Contains(adminView.Body.String(), `"platform_role":"user"`) {
		t.Fatalf("admin profile visibility: %d %s", adminView.Code, adminView.Body.String())
	}
	adminUpdate := request(http.MethodPatch, "/api/v1/users/"+string(other.ID), `{"firstname":"Admin edited"}`, token.Value)
	if adminUpdate.Code != 200 {
		t.Fatalf("admin update: %d %s", adminUpdate.Code, adminUpdate.Body.String())
	}
	// Only a fresh global admin can manage the shared role catalogue and platform roles.
	check(http.MethodPost, "/api/v1/roles", `{"name":"operator","can_read_project":true,"can_start_model":true}`, otherToken, 403)
	createdRole := check(http.MethodPost, "/api/v1/roles", `{"name":"operator","can_read_project":true,"can_start_model":true}`, token.Value, 201)
	var catalogRole struct {
		ID             int  `json:"id"`
		CanReadProject bool `json:"can_read_project"`
		CanStartModel  bool `json:"can_start_model"`
	}
	if err = json.Unmarshal(createdRole.Body.Bytes(), &catalogRole); err != nil || catalogRole.ID <= 0 || !catalogRole.CanReadProject || !catalogRole.CanStartModel {
		t.Fatalf("create role: %+v %v", catalogRole, err)
	}
	check(http.MethodGet, "/api/v1/roles/"+fmt.Sprint(catalogRole.ID), "", otherToken, 200)
	check(http.MethodPatch, "/api/v1/roles/"+fmt.Sprint(catalogRole.ID), `{"unknown":true}`, token.Value, 422)
	check(http.MethodPatch, "/api/v1/roles/"+fmt.Sprint(catalogRole.ID), `{"can_start_model":null}`, token.Value, 422)
	check(http.MethodPatch, "/api/v1/roles/"+fmt.Sprint(catalogRole.ID), `{"can_start_model":false}`, token.Value, 200)
	var beforeCanStart, afterCanStart string
	if err = pool.QueryRow(ctx, `SELECT details->'before'->>'can_start_model', details->'after'->>'can_start_model' FROM audit_events WHERE action='role.updated' AND details->>'role_id'=$1 ORDER BY created_at DESC LIMIT 1`, fmt.Sprint(catalogRole.ID)).Scan(&beforeCanStart, &afterCanStart); err != nil || beforeCanStart != "true" || afterCanStart != "false" {
		t.Fatalf("role audit diff: before=%q after=%q err=%v", beforeCanStart, afterCanStart, err)
	}
	check(http.MethodPatch, "/api/v1/roles/"+fmt.Sprint(teamlead), `{"all_permissions":false}`, token.Value, 409)
	check(http.MethodPost, "/api/v1/projects", `{"name":"role-audit","description":""}`, token.Value, 201)
	check(http.MethodPut, "/api/v1/projects/role-audit/members/"+string(other.ID), fmt.Sprintf(`{"role_id":%d}`, catalogRole.ID), token.Value, 201)
	check(http.MethodDelete, "/api/v1/roles/"+fmt.Sprint(catalogRole.ID), "", token.Value, 409)
	transientRole := check(http.MethodPost, "/api/v1/roles", `{"name":"transient"}`, token.Value, 201)
	var transient struct {
		ID int `json:"id"`
	}
	if err = json.Unmarshal(transientRole.Body.Bytes(), &transient); err != nil || transient.ID <= 0 {
		t.Fatalf("transient role: %+v %v", transient, err)
	}
	check(http.MethodDelete, "/api/v1/roles/"+fmt.Sprint(transient.ID), "", token.Value, 204)
	check(http.MethodGet, "/api/v1/roles/"+fmt.Sprint(transient.ID), "", otherToken, 404)
	var authBefore, authAfter, authAgain int64
	if err = pool.QueryRow(ctx, "SELECT auth_version FROM users WHERE id=$1", string(other.ID)).Scan(&authBefore); err != nil {
		t.Fatal(err)
	}
	check(http.MethodPut, "/api/v1/users/"+string(other.ID)+"/platform-role", `{"platform_role":"analyst"}`, token.Value, 200)
	otherToken = issueCurrentToken(other.ID, other.Login)
	if err = pool.QueryRow(ctx, "SELECT auth_version FROM users WHERE id=$1", string(other.ID)).Scan(&authAfter); err != nil || authAfter != authBefore+1 {
		t.Fatalf("platform role version: %d %d %v", authBefore, authAfter, err)
	}
	var oldPlatformRole, newPlatformRole string
	if err = pool.QueryRow(ctx, `SELECT details->>'old_platform_role', details->>'new_platform_role' FROM audit_events WHERE action='user.platform_role_updated' AND object_id=$1 ORDER BY created_at DESC LIMIT 1`, string(other.ID)).Scan(&oldPlatformRole, &newPlatformRole); err != nil || oldPlatformRole != "user" || newPlatformRole != "analyst" {
		t.Fatalf("platform role audit diff: old=%q new=%q err=%v", oldPlatformRole, newPlatformRole, err)
	}
	check(http.MethodPut, "/api/v1/users/"+string(other.ID)+"/platform-role", `{"platform_role":"analyst"}`, token.Value, 200)
	if err = pool.QueryRow(ctx, "SELECT auth_version FROM users WHERE id=$1", string(other.ID)).Scan(&authAgain); err != nil || authAgain != authAfter {
		t.Fatalf("idempotent platform role version: %d %d %v", authAfter, authAgain, err)
	}
	check(http.MethodPut, "/api/v1/users/"+profile.ID+"/platform-role", `{"platform_role":"user"}`, token.Value, 409)
	check(http.MethodPut, "/api/v1/users/"+profile.ID+"/platform-role", `{"platform_role":"user"}`, otherToken, 403)
	if _, err = pool.Exec(ctx, "UPDATE projects SET deleted_at=now() WHERE name='role-audit'"); err != nil {
		t.Fatal(err)
	}
	lastAdmin := request(http.MethodDelete, "/api/v1/users/"+profile.ID, "", token.Value)
	if lastAdmin.Code != 409 {
		t.Fatalf("last admin: %d %s", lastAdmin.Code, lastAdmin.Body.String())
	}
	if _, err = pool.Exec(ctx, "UPDATE users SET platform_role='user' WHERE id=$1", profile.ID); err != nil {
		t.Fatal(err)
	}
	token.Value = issueCurrentToken(domain.UserID(profile.ID), "web")
	nullPatch := request(http.MethodPatch, "/api/v1/users/"+profile.ID, `{"firstname":null}`, token.Value)
	if nullPatch.Code < 400 {
		t.Fatalf("null patch: %d", nullPatch.Code)
	}
	update := request(http.MethodPatch, "/api/v1/users/"+profile.ID, `{"firstname":"Updated"}`, token.Value)
	if update.Code != 200 {
		t.Fatalf("patch: %d %s", update.Code, update.Body.String())
	}
	removed := request(http.MethodDelete, "/api/v1/users/"+profile.ID, "", token.Value)
	if removed.Code != 204 {
		t.Fatalf("delete: %d %s", removed.Code, removed.Body.String())
	}
	var auditCount int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE object_id=$1 AND action IN ('user.created','user.updated','user.deleted')", profile.ID).Scan(&auditCount); err != nil || auditCount != 3 {
		t.Fatalf("audit: %d %v", auditCount, err)
	}

}

// repositoryFixture simulates one SQL transaction without a PostgreSQL server.
func repositoryFixture(t *testing.T, missing string, failure error) (*testutil.MockDatabase, *testutil.MockTx) {
	t.Helper()
	user := testutil.UserMother{}.Regular()
	id := pgtype.UUID{Bytes: uuid.MustParse(string(user.ID)), Valid: true}
	stamp := pgtype.Timestamptz{Time: user.CreatedAt, Valid: true}
	shortUser := []any{id, stamp, stamp, user.Firstname, user.Lastname, user.Email, user.PasswordHash, "user", int64(1)}
	longUser := []any{id, stamp, stamp, user.Firstname, user.Lastname, user.Email, user.Login, user.PasswordHash, "user", int64(1)}
	role := []any{int32(1), "developer", pgtype.Text{}, false, true, true, true, true, true, true, true, true, true, true}
	project := []any{id, "demo", "description"}
	row := func(_ context.Context, sql string, _ ...interface{}) pgx.Row {
		if failure != nil {
			return testutil.Row{Err: failure}
		}
		if missing != "" && strings.Contains(sql, "-- name: "+missing+" ") {
			return testutil.Row{Err: pgx.ErrNoRows}
		}
		var values []any
		switch {
		case strings.Contains(sql, "-- name: FindUserByEmailForUpdate "):
			return testutil.Row{Err: pgx.ErrNoRows}
		case strings.Contains(sql, "-- name: CreateUser "), strings.Contains(sql, "-- name: CreateAdminUser "), strings.Contains(sql, "-- name: FindUserByLogin "):
			values = longUser
		case strings.Contains(sql, "-- name: FindUserByEmail "), strings.Contains(sql, "-- name: GetUserByID "), strings.Contains(sql, "-- name: UpdateUser "), strings.Contains(sql, "-- name: UpdatePlatformRole "):
			values = shortUser
		case strings.Contains(sql, "-- name: GetAuthState "), strings.Contains(sql, "-- name: UpdatePasswordHashIfCurrent "):
			values = []any{id, "user", int64(1)}
		case strings.Contains(sql, "-- name: GetVisibleUser "):
			values = []any{id, user.Email, user.Firstname, user.Lastname, "user"}
		case strings.Contains(sql, "-- name: LockUser "):
			values = []any{"admin"}
		case strings.Contains(sql, "-- name: CountOrphanedProjects "), strings.Contains(sql, "-- name: CountAdmins "):
			values = []any{int64(0)}
		case strings.Contains(sql, "-- name: CountOtherAdmins "), strings.Contains(sql, "-- name: CountTeamleads "):
			values = []any{int64(1)}
		case strings.Contains(sql, "-- name: InsertProject "), strings.Contains(sql, "-- name: LockProject "), strings.Contains(sql, "-- name: UpdateProjectDescription "):
			values = project
		case strings.Contains(sql, "-- name: GetProjectAccess "):
			values = []any{"admin", false, false, false, false, false, false}
		case strings.Contains(sql, "-- name: GetProjectTokenGrant "):
			values = []any{"demo", "developer", false, true, false, false, false, false, true, false, false, false, false}
		case strings.Contains(sql, "-- name: GetMemberRoleKey "):
			return testutil.Row{Err: pgx.ErrNoRows}
		case strings.Contains(sql, "-- name: GetRoleSystemKey "):
			values = []any{"developer"}
		case strings.Contains(sql, "-- name: CreateRole "), strings.Contains(sql, "-- name: GetRole "), strings.Contains(sql, "-- name: LockRole "), strings.Contains(sql, "-- name: UpdateRole "):
			values = role
		default:
			t.Fatalf("unexpected QueryRow: %s", sql)
		}
		return testutil.Row{Values: values}
	}
	query := func(_ context.Context, sql string, _ ...interface{}) (pgx.Rows, error) {
		if failure != nil {
			return nil, failure
		}
		var data [][]any
		switch {
		case strings.Contains(sql, "-- name: ListUsers "):
			data = [][]any{{id, user.Email, user.Firstname, user.Lastname}}
		case strings.Contains(sql, "-- name: ListVisibleUsers "):
			data = [][]any{{id, user.Email, user.Firstname, user.Lastname, "user"}}
		case strings.Contains(sql, "-- name: ListVisibleProjects "):
			data = [][]any{project}
		case strings.Contains(sql, "-- name: ListProjectMembers "):
			data = [][]any{{id, user.Email, user.Firstname, user.Lastname, int32(1), "developer"}}
		case strings.Contains(sql, "-- name: ListRoles "):
			data = [][]any{role}
		case strings.Contains(sql, "-- name: LockUserProjects "):
		default:
			t.Fatalf("unexpected Query: %s", sql)
		}
		if missing != "" && strings.Contains(sql, "-- name: "+missing+" ") {
			data = nil
		}
		return &testutil.Rows{Data: data}, nil
	}
	exec := func(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
		if failure != nil {
			return pgconn.CommandTag{}, failure
		}
		return pgconn.NewCommandTag("UPDATE 1"), nil
	}
	shared := testutil.MockDBTX{QueryRowFunc: row, QueryFunc: query, ExecFunc: exec}
	tx := &testutil.MockTx{MockDBTX: shared, CommitFunc: func(context.Context) error { return nil }, RollbackFunc: func(context.Context) error { return nil }}
	db := &testutil.MockDatabase{MockDBTX: shared, BeginFunc: func(context.Context) (pgx.Tx, error) {
		if failure != nil {
			return nil, failure
		}
		return tx, nil
	}}
	return db, tx
}

// Техника: классы эквивалентности результатов БД и ошибок зависимостей.
func TestUserRepositoryUnit(t *testing.T) {
	for _, method := range []string{"create", "create saved", "bootstrap", "find email", "find login", "list", "update", "delete", "auth state", "update hash", "actor role", "set role", "get visible", "list visible"} {
		for _, scenario := range []string{"success", "database error"} {
			t.Run(method+"/"+scenario, func(t *testing.T) {
				failure := errors.New("database error")
				var injected error
				if scenario == "database error" {
					injected = failure
				}
				db, _ := repositoryFixture(t, "", injected)
				repo := repository.NewUserRepository(db)
				u := testutil.UserMother{}.Regular()
				ctx := context.Background()
				var err error
				switch method {
				case "create":
					err = repo.Create(ctx, u)
				case "create saved":
					_, err = repo.CreateSaved(ctx, u)
				case "bootstrap":
					_, _, err = repo.BootstrapAdmin(ctx, u)
				case "find email":
					_, err = repo.FindByEmail(ctx, u.Email)
				case "find login":
					_, err = repo.FindByLogin(ctx, u.Login)
				case "list":
					_, err = repo.List(ctx, 0, 10)
				case "update":
					name := "Alice"
					_, err = repo.Update(ctx, u.ID, domain.UserUpdate{Firstname: &name}, u.UpdatedAt)
				case "delete":
					err = repo.Delete(ctx, u.ID)
				case "auth state":
					_, err = repo.GetAuthState(ctx, u.ID)
				case "update hash":
					_, _, err = repo.UpdatePasswordHashIfCurrent(ctx, u.ID, "old", "new", u.UpdatedAt)
				case "actor role":
					_, err = repo.ActorRole(ctx, u.ID)
				case "set role":
					_, err = repo.SetPlatformRole(ctx, u.ID, u.ID, domain.PlatformRoleUser)
				case "get visible":
					_, err = repo.GetVisible(ctx, u.ID, u.ID)
				case "list visible":
					_, err = repo.ListVisible(ctx, u.ID, 0, 10)
				}
				if scenario == "success" && err != nil {
					t.Fatalf("err=%v", err)
				}
				if scenario == "database error" && !errors.Is(err, failure) {
					t.Fatalf("err=%v", err)
				}
			})
		}
	}
}

// Техника: класс отсутствующей записи и пустого результата.
func TestUserRepositoryMissing(t *testing.T) {
	cases := []struct {
		method, query string
		want          error
	}{
		{"find email", "FindUserByEmail", domain.ErrUserNotFound}, {"find login", "FindUserByLogin", domain.ErrUserNotFound}, {"list", "ListUsers", nil}, {"update", "UpdateUser", domain.ErrUserNotFound}, {"delete", "LockUser", domain.ErrUserNotFound}, {"auth state", "GetAuthState", domain.ErrUserNotFound}, {"update hash", "UpdatePasswordHashIfCurrent", nil}, {"actor role", "GetAuthState", domain.ErrUserNotFound}, {"set role", "LockUser", service.ErrInvalidToken}, {"get visible", "GetVisibleUser", domain.ErrUserNotFound}, {"list visible", "ListVisibleUsers", nil},
	}
	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			db, _ := repositoryFixture(t, tc.query, nil)
			repo := repository.NewUserRepository(db)
			user := testutil.UserMother{}.Regular()
			ctx := context.Background()
			var err error
			switch tc.method {
			case "find email":
				_, err = repo.FindByEmail(ctx, user.Email)
			case "find login":
				_, err = repo.FindByLogin(ctx, user.Login)
			case "list":
				var rows []domain.User
				rows, err = repo.List(ctx, 0, 10)
				if len(rows) != 0 {
					t.Fatalf("rows=%+v", rows)
				}
			case "update":
				_, err = repo.Update(ctx, user.ID, domain.UserUpdate{}, user.UpdatedAt)
			case "delete":
				err = repo.Delete(ctx, user.ID)
			case "auth state":
				_, err = repo.GetAuthState(ctx, user.ID)
			case "update hash":
				var changed bool
				_, changed, err = repo.UpdatePasswordHashIfCurrent(ctx, user.ID, "old", "new", user.UpdatedAt)
				if changed {
					t.Fatal("stale hash updated")
				}
			case "actor role":
				_, err = repo.ActorRole(ctx, user.ID)
			case "set role":
				_, err = repo.SetPlatformRole(ctx, user.ID, user.ID, domain.PlatformRoleUser)
			case "get visible":
				_, err = repo.GetVisible(ctx, user.ID, user.ID)
			case "list visible":
				var rows []domain.User
				rows, err = repo.ListVisible(ctx, user.ID, 0, 10)
				if len(rows) != 0 {
					t.Fatalf("rows=%+v", rows)
				}
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("error=%v want=%v", err, tc.want)
			}
		})
	}
}

// Техника: классы эквивалентности чтения существующей записи.
func TestUserRepositoryMapsDatabaseResults(t *testing.T) {
	for _, method := range []string{"find email", "find login", "auth state", "get visible", "list", "list visible"} {
		t.Run(method, func(t *testing.T) {
			db, _ := repositoryFixture(t, "", nil)
			r := repository.NewUserRepository(db)
			user := testutil.UserMother{}.Regular()
			ctx := context.Background()
			var id domain.UserID
			var email string
			var err error
			switch method {
			case "find email":
				var v domain.User
				v, err = r.FindByEmail(ctx, user.Email)
				id, email = v.ID, v.Email
			case "find login":
				var v domain.User
				v, err = r.FindByLogin(ctx, user.Login)
				id, email = v.ID, v.Email
				if err == nil && v.Login != user.Login {
					t.Fatalf("login=%q", v.Login)
				}
			case "auth state":
				var v domain.AuthState
				v, err = r.GetAuthState(ctx, user.ID)
				id = v.UserID
				email = user.Email
				if err == nil && v.AuthVersion != 1 {
					t.Fatalf("auth version=%d", v.AuthVersion)
				}
			case "get visible":
				var v domain.User
				v, err = r.GetVisible(ctx, user.ID, user.ID)
				id, email = v.ID, v.Email
			case "list":
				var rows []domain.User
				rows, err = r.List(ctx, 0, 10)
				if err == nil && len(rows) == 1 {
					id, email = rows[0].ID, rows[0].Email
				}
			case "list visible":
				var rows []domain.User
				rows, err = r.ListVisible(ctx, user.ID, 0, 10)
				if err == nil && len(rows) == 1 {
					id, email = rows[0].ID, rows[0].Email
				}
			}
			if err != nil || id != user.ID || email != user.Email {
				t.Fatalf("id=%q email=%q err=%v", id, email, err)
			}
		})
	}
}

// Техника: переходы состояния транзакции при успешной записи и ошибке SQL.
func TestUserCreateTransactionOutcome(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "rollback"}[fail], func(t *testing.T) {
			db, tx := repositoryFixture(t, "", nil)
			failure := errors.New("insert failed")
			commits, rollbacks := 0, 0
			tx.CommitFunc = func(context.Context) error { commits++; return nil }
			tx.RollbackFunc = func(context.Context) error { rollbacks++; return nil }
			if fail {
				tx.QueryRowFunc = func(context.Context, string, ...interface{}) pgx.Row { return testutil.Row{Err: failure} }
			}
			user := testutil.UserMother{}.Regular()
			_, err := repository.NewUserRepository(db).CreateSaved(context.Background(), user)
			if fail && (!errors.Is(err, failure) || commits != 0) || !fail && (err != nil || commits != 1) || rollbacks != 1 {
				t.Fatalf("err=%v commits=%d rollbacks=%d", err, commits, rollbacks)
			}
		})
	}
}

// Техника: классы эквивалентности нарушенных ограничений уникальности.
func TestUserCreateUniqueConflicts(t *testing.T) {
	for _, tc := range []struct {
		constraint string
		want       error
	}{{"users_email_lower_unique_idx", domain.ErrEmailAlreadyTaken}, {"users_login_lower_unique_idx", domain.ErrLoginAlreadyTaken}} {
		t.Run(tc.constraint, func(t *testing.T) {
			db, tx := repositoryFixture(t, "", nil)
			tx.QueryRowFunc = func(context.Context, string, ...interface{}) pgx.Row {
				return testutil.Row{Err: &pgconn.PgError{Code: "23505", ConstraintName: tc.constraint}}
			}
			user := testutil.UserMother{}.Regular()
			err := repository.NewUserRepository(db).Create(context.Background(), user)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
		})
	}
}

// Техника: переходы состояний — назначение уже установленной роли.
func TestSetPlatformRoleUnchanged(t *testing.T) {
	db, tx := repositoryFixture(t, "", nil)
	commits, rollbacks := 0, 0
	tx.CommitFunc = func(context.Context) error { commits++; return nil }
	tx.RollbackFunc = func(context.Context) error { rollbacks++; return nil }
	user := testutil.UserMother{}.Regular()
	result, err := repository.NewUserRepository(db).SetPlatformRole(context.Background(), user.ID, user.ID, domain.PlatformRoleAdmin)
	if err != nil || result.ID != user.ID || commits != 0 || rollbacks != 1 {
		t.Fatalf("result=%+v err=%v commits=%d rollbacks=%d", result, err, commits, rollbacks)
	}
}
