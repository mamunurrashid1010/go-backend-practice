package notes

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// fakeRepository — shared by service_test.go AND handler_test.go.
// Same scope because both files are `package notes` _test.go files.
type fakeRepository struct {
	ListNotes  []Note
	ListErr    error
	GetNote    Note
	GetErr     error
	CreateNote Note
	CreateErr  error
	UpdateNote Note
	UpdateErr  error
	PatchNote  Note
	PatchErr   error
	DeleteErr  error

	ListCalls, GetCalls, CreateCalls, UpdateCalls, PatchCalls, DeleteCalls int

	LastListUserID, LastGetUserID, LastCreateUserID     int64
	LastUpdateUserID, LastPatchUserID, LastDeleteUserID int64
	LastGetID, LastUpdateID, LastPatchID, LastDeleteID  int64
	LastListFilter                                      ListFilter
	LastCreateIn                                        CreateRequest
	LastUpdateIn                                        UpdateRequest
	LastPatchIn                                         PatchRequest
}

func (r *fakeRepository) List(_ context.Context, userID int64, f ListFilter) ([]Note, error) {
	r.ListCalls++
	r.LastListUserID, r.LastListFilter = userID, f
	return r.ListNotes, r.ListErr
}

func (r *fakeRepository) Get(_ context.Context, userID, id int64) (Note, error) {
	r.GetCalls++
	r.LastGetUserID, r.LastGetID = userID, id
	return r.GetNote, r.GetErr
}

func (r *fakeRepository) Create(_ context.Context, userID int64, in CreateRequest) (Note, error) {
	r.CreateCalls++
	r.LastCreateUserID, r.LastCreateIn = userID, in
	return r.CreateNote, r.CreateErr
}

func (r *fakeRepository) Update(_ context.Context, userID, id int64, in UpdateRequest) (Note, error) {
	r.UpdateCalls++
	r.LastUpdateUserID, r.LastUpdateID, r.LastUpdateIn = userID, id, in
	return r.UpdateNote, r.UpdateErr
}

func (r *fakeRepository) Patch(_ context.Context, userID, id int64, in PatchRequest) (Note, error) {
	r.PatchCalls++
	r.LastPatchUserID, r.LastPatchID, r.LastPatchIn = userID, id, in
	return r.PatchNote, r.PatchErr
}

func (r *fakeRepository) Delete(_ context.Context, userID, id int64) error {
	r.DeleteCalls++
	r.LastDeleteUserID, r.LastDeleteID = userID, id
	return r.DeleteErr
}

var _ Repository = (*fakeRepository)(nil)

func strPtr(s string) *string { return &s }

// ---- Service tests (Day 22, carried) ---------------------------------

func TestService_Patch_NothingToUpdate(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo)

	_, err := svc.Patch(context.Background(), 1, 99, PatchRequest{})

	if !errors.Is(err, ErrNothingToUpdate) {
		t.Fatalf("want ErrNothingToUpdate, got %v", err)
	}
	if repo.PatchCalls != 0 {
		t.Errorf("repo.Patch should NOT be called, got %d calls", repo.PatchCalls)
	}
}

func TestService_Get(t *testing.T) {
	tests := []struct {
		name      string
		repoNote  Note
		repoErr   error
		wantErr   error
		wantTitle string
	}{
		{name: "found", repoNote: Note{ID: 1, Title: "hello"}, wantTitle: "hello"},
		{name: "not_found_is_propagated", repoErr: ErrNotFound, wantErr: ErrNotFound},
		{name: "wrapped_not_found_still_matches", repoErr: fmt.Errorf("get failed: %w", ErrNotFound), wantErr: ErrNotFound},
		{name: "unknown_error_bubbles", repoErr: errors.New("boom")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepository{GetNote: tc.repoNote, GetErr: tc.repoErr}
			svc := NewService(repo)

			got, err := svc.Get(context.Background(), 1, 42)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want %v, got %v", tc.wantErr, err)
				}
				return
			}
			if tc.repoErr != nil {
				if err == nil {
					t.Fatalf("want some error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got.Title != tc.wantTitle {
				t.Errorf("title: want %q, got %q", tc.wantTitle, got.Title)
			}
		})
	}
}

func TestService_Create(t *testing.T) {
	want := Note{ID: 1, UserID: 7, Title: "new"}
	repo := &fakeRepository{CreateNote: want}
	svc := NewService(repo)

	got, err := svc.Create(context.Background(), 7, CreateRequest{Title: "new"})

	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != want {
		t.Errorf("note: want %+v, got %+v", want, got)
	}
}
