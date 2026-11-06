package repository_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jur4ikoff/model-service/internal/domain"
	"github.com/jur4ikoff/model-service/internal/repository"
	"github.com/jur4ikoff/model-service/internal/service"
	"github.com/jur4ikoff/model-service/internal/testutil"
)

// Техника: классы эквивалентности идентификаторов моделей и проектов.
func TestModelsAreIdentifiedByIDWithinProject(t *testing.T) {
	databaseURL := os.Getenv("MODEL_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set MODEL_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 1
	config.MinConns = 0
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	// The temporary table shadows the application's models table on this connection.
	// Closing the pool removes it without changing application data.
	_, err = pool.Exec(ctx, `CREATE TEMP TABLE models (
		id UUID PRIMARY KEY,
		created_at TIMESTAMPTZ NOT NULL,
		updated_at TIMESTAMPTZ NOT NULL,
		name TEXT NOT NULL,
		description TEXT NOT NULL,
		username TEXT NOT NULL,
		project TEXT NOT NULL,
		spec JSONB NOT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `CREATE UNIQUE INDEX models_project_name_key ON models (project, name)`); err != nil {
		t.Fatal(err)
	}
	runtime := &runtimeSpy{}
	models := service.NewModelService(repository.NewModelRepository(pool), runtime)
	projectA, projectB, missingProject := "demo-project", "another-project", "missing-project"
	create := func(project, name string) domain.Model {
		t.Helper()
		model, err := models.Create(ctx, service.CreateModelInput{
			Name: name, Project: project, Username: uuid.NewString(), Spec: validModelSpec(),
		})
		if err != nil {
			t.Fatal(err)
		}
		return model
	}
	a, b := create(projectA, "demo"), create(projectB, "demo")
	create(projectA, "other")

	for _, expected := range []domain.Model{a, b} {
		actual, err := models.Get(ctx, " "+expected.Project+" ", " "+expected.ID+" ")
		if err != nil || actual.ID != expected.ID {
			t.Fatalf("get project %s: model=%+v error=%v", expected.Project, actual, err)
		}
	}
	if _, err := models.Create(ctx, service.CreateModelInput{
		Name: "demo", Project: projectA, Username: uuid.NewString(), Spec: validModelSpec(),
	}); !errors.Is(err, domain.ErrModelAlreadyExists) {
		t.Fatalf("duplicate name in project: %v", err)
	}

	name, description := "renamed", "updated"
	updated, err := models.Update(ctx, projectA, a.ID, service.UpdateModelInput{Name: &name, Description: &description})
	if err != nil || updated.ID != a.ID || updated.Name != name || updated.Description != description {
		t.Fatalf("rename: model=%+v error=%v", updated, err)
	}
	byID, err := models.Get(ctx, projectA, a.ID)
	if err != nil || byID.ID != a.ID || byID.Name != name {
		t.Fatalf("ID should resolve after rename: model=%+v error=%v", byID, err)
	}
	if _, err := models.Get(ctx, projectB, a.ID); !errors.Is(err, domain.ErrModelNotFound) {
		t.Fatalf("ID should not resolve in another project: %v", err)
	}
	conflict := "other"
	if _, err := models.Update(ctx, projectA, a.ID, service.UpdateModelInput{Name: &conflict}); !errors.Is(err, domain.ErrModelAlreadyExists) {
		t.Fatalf("rename conflict: %v", err)
	}
	if _, err := models.Start(ctx, projectA, a.ID); err != nil || runtime.model.ID != a.ID {
		t.Fatalf("start selected wrong model: %+v error=%v", runtime.model, err)
	}

	if _, err := models.Get(ctx, missingProject, a.ID); !errors.Is(err, domain.ErrModelNotFound) {
		t.Fatalf("get in wrong project: %v", err)
	}
	if _, err := models.Update(ctx, missingProject, a.ID, service.UpdateModelInput{Description: &description}); !errors.Is(err, domain.ErrModelNotFound) {
		t.Fatalf("update in wrong project: %v", err)
	}
	if err := models.Delete(ctx, missingProject, a.ID); !errors.Is(err, domain.ErrModelNotFound) {
		t.Fatalf("delete in wrong project: %v", err)
	}
	runtime.model = domain.Model{}
	if _, err := models.Start(ctx, missingProject, a.ID); !errors.Is(err, domain.ErrModelNotFound) || runtime.model.ID != "" {
		t.Fatalf("start in wrong project reached runtime: %+v error=%v", runtime.model, err)
	}
	if err := models.Delete(ctx, projectA, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := models.Get(ctx, projectA, a.ID); !errors.Is(err, domain.ErrModelNotFound) {
		t.Fatalf("deleted model still resolves: %v", err)
	}
	remaining, err := models.Get(ctx, projectB, b.ID)
	if err != nil || remaining.ID != b.ID || remaining.Name != "demo" || remaining.Description != "" {
		t.Fatalf("another project was modified: %+v error=%v", remaining, err)
	}

	for _, selector := range [][2]string{{"", a.ID}, {projectA, " "}, {projectA, "demo"}} {
		if _, err := models.Get(ctx, selector[0], selector[1]); !errors.Is(err, service.ErrInvalidInput) {
			t.Errorf("get invalid selector: %v", err)
		}
		if _, err := models.Update(ctx, selector[0], selector[1], service.UpdateModelInput{Description: &description}); !errors.Is(err, service.ErrInvalidInput) {
			t.Errorf("update invalid selector: %v", err)
		}
		if err := models.Delete(ctx, selector[0], selector[1]); !errors.Is(err, service.ErrInvalidInput) {
			t.Errorf("delete invalid selector: %v", err)
		}
		if _, err := models.Start(ctx, selector[0], selector[1]); !errors.Is(err, service.ErrInvalidInput) {
			t.Errorf("start invalid selector: %v", err)
		}
	}
}

func validModelSpec() domain.ModelSpec {
	return domain.ModelSpec{
		ProviderType:        domain.ProviderHuggingFace,
		ProviderHuggingFace: &domain.HuggingFaceProvider{Repo: "org/model"},
	}
}

type runtimeSpy struct {
	model domain.Model
}

func (r *runtimeSpy) Start(_ context.Context, model domain.Model) error {
	r.model = model
	return nil
}

func (*runtimeSpy) CreateDeployment(context.Context, domain.Model) (domain.Deployment, error) {
	return domain.Deployment{}, nil
}

func (*runtimeSpy) GetDeployment(context.Context, domain.Model) (domain.Deployment, error) {
	return domain.Deployment{}, nil
}

func (*runtimeSpy) GetDeploymentsHistory(context.Context, string) ([]domain.Deployment, error) {
	return nil, nil
}

func (*runtimeSpy) StopDeployment(context.Context, domain.Deployment) error {
	return nil
}

func (*runtimeSpy) GetActiveDeployments(context.Context, string) ([]domain.Deployment, error) {
	return nil, nil
}

// Техника: классы эквивалентности результата запроса и ошибки зависимости.
func TestModelRepositoryUnit(t *testing.T) {
	for _, method := range []string{"create", "get", "list", "update", "delete"} {
		for _, scenario := range []string{"success", "missing", "database error"} {
			t.Run(method+"/"+scenario, func(t *testing.T) {
				model := testutil.NewModelBuilder().WithName("encoded-model").Build()
				id := uuid.MustParse(model.ID)
				saved := []any{pgtype.UUID{Bytes: id, Valid: true}, pgtype.Timestamptz{Time: model.CreatedAt, Valid: true}, pgtype.Timestamptz{Time: model.UpdatedAt, Valid: true}, model.Name, model.Description, model.Username, model.Project, []byte(`{"provider_type":"HUGGING_FACE","provider_hugging_face":{"repo":"org/model"}}`)}
				failure := errors.New("database error")
				var rowErr error
				if scenario == "missing" {
					rowErr = pgx.ErrNoRows
				}
				if scenario == "database error" {
					rowErr = failure
				}
				calls := 0
				db := &testutil.MockDBTX{
					QueryRowFunc: func(context.Context, string, ...interface{}) pgx.Row {
						calls++
						return testutil.Row{Values: saved, Err: rowErr}
					},
					QueryFunc: func(context.Context, string, ...interface{}) (pgx.Rows, error) {
						calls++
						if scenario == "database error" {
							return nil, failure
						}
						rows := &testutil.Rows{}
						if scenario == "success" {
							rows.Data = [][]any{saved}
						}
						return rows, nil
					},
					ExecFunc: func(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
						calls++
						if scenario == "database error" {
							return pgconn.CommandTag{}, failure
						}
						if scenario == "missing" {
							return pgconn.NewCommandTag("DELETE 0"), nil
						}
						return pgconn.NewCommandTag("DELETE 1"), nil
					},
				}
				repo := repository.NewModelRepository(db)
				ctx := context.Background()
				var err error
				var got domain.Model
				switch method {
				case "create":
					got, err = repo.Create(ctx, model)
				case "get":
					got, err = repo.Get(ctx, model.Project, model.ID)
				case "list":
					var rows []domain.Model
					rows, err = repo.List(ctx, model.Project, 0, 10)
					if err == nil && ((scenario == "success" && len(rows) != 1) || (scenario == "missing" && len(rows) != 0)) {
						t.Fatalf("rows=%+v", rows)
					}
				case "update":
					got, err = repo.Update(ctx, model.Project, model.ID, nil, nil, nil, time.Now())
				case "delete":
					err = repo.Delete(ctx, model.Project, model.ID)
				}
				if calls != 1 {
					t.Fatalf("database calls=%d", calls)
				}
				if scenario == "database error" && !errors.Is(err, failure) {
					t.Fatalf("err=%v", err)
				}
				if scenario == "missing" && method != "list" {
					want := domain.ErrModelNotFound
					if method == "create" {
						want = pgx.ErrNoRows
					}
					if !errors.Is(err, want) {
						t.Fatalf("err=%v", err)
					}
				}
				if scenario == "success" && (err != nil || (method != "delete" && method != "list" && got.ID != model.ID)) {
					t.Fatalf("model=%+v err=%v", got, err)
				}
			})
		}
	}
}

// Техника: классы эквивалентности ошибок при чтении набора строк.
func TestModelListReadErrors(t *testing.T) {
	for _, scenario := range []string{"scan", "iteration"} {
		t.Run(scenario, func(t *testing.T) {
			failure := errors.New("read failed")
			rows := &testutil.Rows{Data: [][]any{{}}}
			if scenario == "scan" {
				rows.ScanErr = failure
			} else {
				rows.Data = nil
				rows.IterErr = failure
			}
			db := &testutil.MockDBTX{QueryFunc: func(context.Context, string, ...interface{}) (pgx.Rows, error) { return rows, nil }}
			_, err := repository.NewModelRepository(db).List(context.Background(), "demo", 0, 10)
			if !errors.Is(err, failure) || !rows.Closed {
				t.Fatalf("err=%v closed=%v", err, rows.Closed)
			}
		})
	}
}

// Техника: класс конфликта уникального имени модели.
func TestModelCreateDuplicate(t *testing.T) {
	db := &testutil.MockDBTX{QueryRowFunc: func(context.Context, string, ...interface{}) pgx.Row {
		return testutil.Row{Err: &pgconn.PgError{Code: "23505", ConstraintName: "models_project_name_key"}}
	}}
	_, err := repository.NewModelRepository(db).Create(context.Background(), testutil.ModelMother{}.HuggingFace())
	if !errors.Is(err, domain.ErrModelAlreadyExists) {
		t.Fatalf("err=%v", err)
	}
}
