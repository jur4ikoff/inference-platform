package api

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"testing"

	"github.com/jur4ikoff/model-service/internal/domain"
	"github.com/jur4ikoff/model-service/internal/service"
)

func TestListModelPages(t *testing.T) {
	repo := &modelsListRepository{models: []domain.Model{
		{ID: "first", Project: "project-a"}, {ID: "other", Project: "project-b"},
		{ID: "second", Project: "project-a"}, {ID: "third", Project: "project-a"},
	}}
	router := NewHandler(service.NewModelService(repo, nil)).Router()
	for _, test := range []struct {
		query string
		count int
		first string
		next  int
	}{
		{"?offset=0&limit=2", 2, "first", 2},
		{"?offset=2&limit=2", 1, "third", 0},
		{"?offset=20&limit=2", 0, "", 0},
		{"", 3, "first", 0},
	} {
		response := apiRequest(t, router, http.MethodGet, "/api/v1/projects/project-a/models"+test.query, "", http.StatusOK)
		var page listModelsResponse
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if page.Models == nil || len(page.Models) != test.count || (test.count > 0 && page.Models[0].ID != test.first) {
			t.Fatalf("unexpected page: %s", response.Body.String())
		}
		if (test.next == 0 && page.NextPageToken != nil) || (test.next != 0 && (page.NextPageToken == nil || *page.NextPageToken != test.next)) {
			t.Fatalf("incorrect next offset: %s", response.Body.String())
		}
	}
	if repo.limit != 21 {
		t.Fatalf("default page should fetch 21 rows, got %d", repo.limit)
	}
	response := apiRequest(t, router, http.MethodGet, "/api/v1/projects/project-b/models?project=project-a", "", http.StatusOK)
	var otherProject listModelsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &otherProject); err != nil || len(otherProject.Models) != 1 || otherProject.Models[0].ID != "other" {
		t.Fatalf("list must select the project from its path: %s, error=%v", response.Body.String(), err)
	}
	for _, query := range []string{"?offset=-1", "?limit=0", "?limit=101", "?limit=invalid"} {
		apiRequest(t, router, http.MethodGet, "/api/v1/projects/project-a/models"+query, "", http.StatusUnprocessableEntity)
	}
	for _, input := range []service.ListModelsInput{{}, {Project: " "}, {Project: "project-a", Offset: -1}, {Project: "project-a", Limit: -1}, {Project: "project-a", Limit: 101}, {Project: "project-a", Offset: math.MaxInt}} {
		if _, err := service.NewModelService(repo, nil).List(context.Background(), input); !errors.Is(err, service.ErrInvalidInput) {
			t.Fatalf("invalid pagination accepted: %+v, error=%v", input, err)
		}
	}
}

type modelsListRepository struct {
	service.ModelRepository
	models []domain.Model
	limit  int
}

func (r *modelsListRepository) List(_ context.Context, project string, offset, limit int) ([]domain.Model, error) {
	r.limit = limit
	var models []domain.Model
	for _, model := range r.models {
		if project == "" || model.Project == project {
			models = append(models, model)
		}
	}
	if offset >= len(models) {
		return nil, nil
	}
	return models[offset:min(offset+limit, len(models))], nil
}
