package notes

import (
	"context"
	"errors"
	"testing"
)

// fakeRepository — updated for Day 26: List returns ListPage.
type fakeRepository struct {
	ListPage   ListPage
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

func (r *fakeRepository) List(_ context.Context, userID int64, f ListFilter) (ListPage, error) {
	r.ListCalls++
	r.LastListUserID, r.LastListFilter = userID, f
	return r.ListPage, r.ListErr
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

func TestService_Patch_NothingToUpdate(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo)
	_, err := svc.Patch(context.Background(), 1, 99, PatchRequest{})
	if !errors.Is(err, ErrNothingToUpdate) {
		t.Fatalf("want ErrNothingToUpdate, got %v", err)
	}
	if repo.PatchCalls != 0 {
		t.Errorf("repo.Patch should NOT be called, got %d", repo.PatchCalls)
	}
}

func TestService_Get_NotFound(t *testing.T) {
	repo := &fakeRepository{GetErr: ErrNotFound}
	svc := NewService(repo)
	_, err := svc.Get(context.Background(), 1, 42)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestService_List_PropagatesFilter(t *testing.T) {
	repo := &fakeRepository{
		ListPage: ListPage{
			Items:  []Note{{ID: 5}, {ID: 4}},
			NextID: 3,
		},
	}
	svc := NewService(repo)
	got, err := svc.List(context.Background(), 1, ListFilter{Limit: 2, AfterID: 7, SortDesc: true, Search: "x"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got.Items) != 2 || got.NextID != 3 {
		t.Errorf("page: %+v", got)
	}
	if repo.LastListFilter.AfterID != 7 || repo.LastListFilter.Search != "x" || !repo.LastListFilter.SortDesc {
		t.Errorf("filter not propagated: %+v", repo.LastListFilter)
	}
}

func TestCursor_RoundTrip(t *testing.T) {
	for _, id := range []int64{1, 42, 1_000_000} {
		s := encodeCursor(id)
		got, err := decodeCursor(s)
		if err != nil {
			t.Fatalf("decode(%q): %v", s, err)
		}
		if got != id {
			t.Errorf("id=%d -> %q -> %d", id, s, got)
		}
	}
}

func TestCursor_InvalidIsRejected(t *testing.T) {
	cases := []string{"!!!", "abc", "MA"} // last is base64("0") which is non-positive
	for _, c := range cases {
		if _, err := decodeCursor(c); err == nil {
			t.Errorf("%q should fail", c)
		}
	}
}
