package service_test

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/jur4ikoff/user-service/internal/domain"
	"github.com/jur4ikoff/user-service/internal/service"
)

func TestListUsersPages(t *testing.T) {
	repo := &memoryUsers{listUsers: []domain.User{
		{ID: "first"}, {ID: "second"}, {ID: "third"}, {ID: "fourth"}, {ID: "fifth"},
	}}
	users := service.NewUserService(repo, nil, nil, nil, nil, 0)
	query := service.ListUsersInput{Offset: 1, Limit: 2}
	for i, want := range [][]domain.UserID{{"second", "third"}, {"fourth", "fifth"}} {
		page, err := users.ListUsers(context.Background(), query)
		if err != nil {
			t.Fatal(err)
		}
		var ids []domain.UserID
		for _, user := range page.Users {
			ids = append(ids, user.ID)
		}
		if !reflect.DeepEqual(ids, want) || repo.listOffset != 1+i*2 || repo.listLimit != 3 {
			t.Fatalf("unexpected page: ids=%v offset=%d fetch limit=%d", ids, repo.listOffset, repo.listLimit)
		}
		if (page.NextPageToken != nil) != (i == 0) {
			t.Fatalf("unexpected next offset: %v", page.NextPageToken)
		}
		if page.NextPageToken != nil {
			if *page.NextPageToken != query.Offset+query.Limit {
				t.Fatal("incorrect next offset")
			}
			query.Offset = *page.NextPageToken
		}
	}

	page, err := users.ListUsers(context.Background(), service.ListUsersInput{Offset: 5, Limit: 2})
	if err != nil || page.Users == nil || len(page.Users) != 0 || page.NextPageToken != nil {
		t.Fatalf("offset beyond list must produce an empty final page: %+v, %v", page, err)
	}
	page, err = users.ListUsers(context.Background(), service.ListUsersInput{})
	if err != nil || len(page.Users) != 5 || repo.listLimit != 21 || page.NextPageToken != nil {
		t.Fatalf("default page: %+v, %v", page, err)
	}
}

func TestListUsersRejectsInvalidPagination(t *testing.T) {
	for _, input := range []service.ListUsersInput{
		{Offset: -1}, {Limit: -1}, {Limit: 101}, {Offset: math.MaxInt},
	} {
		repo := &memoryUsers{}
		users := service.NewUserService(repo, nil, nil, nil, nil, 0)
		if _, err := users.ListUsers(context.Background(), input); !errors.Is(err, service.ErrInvalidInput) || repo.listCalls != 0 {
			t.Errorf("invalid pagination reached storage: input=%+v error=%v calls=%d", input, err, repo.listCalls)
		}
	}
}
