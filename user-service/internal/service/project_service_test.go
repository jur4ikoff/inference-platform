package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jur4ikoff/user-service/internal/domain"
)

func TestProjectServiceAddUserToProject(t *testing.T) {
	repository := &fakeProjectRepository{}
	projects := NewProjectService(repository)

	input := AddUserToProjectInput{
		ProjectID: "0197eb5a-37c6-7e48-8cd6-0c19edcd72f1",
		UserID:    "0197eb5a-37c6-7e48-8cd6-0c19edcd72f2",
		RoleID:    2,
	}
	if err := projects.AddUserToProject(context.Background(), input); err != nil {
		t.Fatalf("AddUserToProject() error = %v", err)
	}
	if repository.input != input {
		t.Fatalf("repository input = %#v, want %#v", repository.input, input)
	}
}

func TestProjectServiceAddUserToProjectRejectsInvalidInput(t *testing.T) {
	repository := &fakeProjectRepository{}
	projects := NewProjectService(repository)

	err := projects.AddUserToProject(context.Background(), AddUserToProjectInput{
		ProjectID: "project-id",
		UserID:    "user-id",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("AddUserToProject() error = %v, want %v", err, ErrInvalidInput)
	}
	if repository.called {
		t.Fatal("repository must not be called for invalid input")
	}
}

type fakeProjectRepository struct {
	ProjectRepository
	called bool
	input  AddUserToProjectInput
}

func (r *fakeProjectRepository) AddUserToProject(_ context.Context, projectID domain.ProjectID, userID domain.UserID, roleID domain.RoleID) error {
	r.called = true
	r.input = AddUserToProjectInput{ProjectID: projectID, UserID: userID, RoleID: roleID}
	return nil
}
