package notes

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// ---------------------------------------------------------------------
// fakeRepository — test double that satisfies the Repository interface.
//
// Each *Note / *Err field configures what the corresponding method returns.
// Last* fields capture what arguments the service called it with.
// *Calls counters let us assert the method was invoked exactly once.
//
// Notice this lives in the SAME package as Service. That's a "white-box"
// test — we can touch unexported types (CreateRequest, etc.) directly.
// ---------------------------------------------------------------------

type fakeRepository struct {
	// returns
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

	// call counters
	ListCalls, GetCalls, CreateCalls, UpdateCalls, PatchCalls, DeleteCalls int

	// captured args (last call wins; fine for our tests)
	LastListUserID, LastGetUserID, LastCreateUserID                                     int64
	LastUpdateUserID, LastPatchUserID, LastDeleteUserID                                 int64
	LastGetID, LastUpdateID, LastPatchID, LastDeleteID                                  int64
	LastListFilter                                                                      ListFilter
	LastCreateIn                                                                        CreateRequest
	LastUpdateIn                                                                        UpdateRequest
	LastPatchIn                                                                         PatchRequest
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

// Compile-time check that fakeRepository satisfies Repository. If it
// doesn't, the build fails right here instead of in a confusing test error.
var _ Repository = (*fakeRepository)(nil)

// strPtr / boolPtr — readable pointer literals for PatchRequest fields.
func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }

// ---------------------------------------------------------------------
// The actual tests.
// ---------------------------------------------------------------------

// TestService_Patch_NothingToUpdate — the single business rule the service
// owns (the rest is repo delegation). Empty PatchRequest -> ErrNothingToUpdate,
// and the repo is NOT called.
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

// TestService_Patch_Delegates — when at least one field is provided, the
// service passes through to the repo with the right args.
func TestService_Patch_Delegates(t *testing.T) {
	want := Note{ID: 7, UserID: 42, Title: "after", Body: "body"}
	repo := &fakeRepository{PatchNote: want}
	svc := NewService(repo)

	got, err := svc.Patch(context.Background(), 42, 7, PatchRequest{Title: strPtr("after")})

	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != want {
		t.Errorf("note: want %+v, got %+v", want, got)
	}
	if repo.PatchCalls != 1 {
		t.Errorf("repo.Patch calls: want 1, got %d", repo.PatchCalls)
	}
	if repo.LastPatchUserID != 42 || repo.LastPatchID != 7 {
		t.Errorf("repo.Patch args: want (42, 7), got (%d, %d)", repo.LastPatchUserID, repo.LastPatchID)
	}
}

// TestService_Get — table-driven: same setup shape, different rows.
func TestService_Get(t *testing.T) {
	tests := []struct {
		name      string
		repoNote  Note
		repoErr   error
		wantErr   error
		wantTitle string
	}{
		{
			name:      "found",
			repoNote:  Note{ID: 1, UserID: 1, Title: "hello"},
			wantTitle: "hello",
		},
		{
			name:    "not found is propagated",
			repoErr: ErrNotFound,
			wantErr: ErrNotFound,
		},
		{
			name:    "wrapped not found still matches via errors.Is",
			repoErr: fmt.Errorf("get failed: %w", ErrNotFound),
			wantErr: ErrNotFound,
		},
		{
			name:    "unknown error bubbles",
			repoErr: errors.New("boom"),
			wantErr: errors.New("boom"), // checked by message below, not Is
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepository{GetNote: tc.repoNote, GetErr: tc.repoErr}
			svc := NewService(repo)

			got, err := svc.Get(context.Background(), 1, 42)

			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("want err %v, got nil", tc.wantErr)
				}
				// Use errors.Is for sentinel errors; fall back to message
				// equality for one-off ad-hoc errors.
				if errors.Is(tc.wantErr, ErrNotFound) {
					if !errors.Is(err, ErrNotFound) {
						t.Fatalf("want ErrNotFound (wrapped or not), got %v", err)
					}
				} else if err.Error() != tc.wantErr.Error() {
					t.Fatalf("want err %q, got %q", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got.Title != tc.wantTitle {
				t.Errorf("title: want %q, got %q", tc.wantTitle, got.Title)
			}
			if repo.LastGetUserID != 1 || repo.LastGetID != 42 {
				t.Errorf("repo args: want (1, 42), got (%d, %d)", repo.LastGetUserID, repo.LastGetID)
			}
		})
	}
}

// TestService_Create — service is pure delegation today. Confirm it.
func TestService_Create(t *testing.T) {
	want := Note{ID: 1, UserID: 7, Title: "new", Body: "b"}
	repo := &fakeRepository{CreateNote: want}
	svc := NewService(repo)

	got, err := svc.Create(context.Background(), 7, CreateRequest{Title: "new", Body: "b"})

	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != want {
		t.Errorf("note: want %+v, got %+v", want, got)
	}
	if repo.CreateCalls != 1 {
		t.Errorf("repo.Create calls: want 1, got %d", repo.CreateCalls)
	}
	if repo.LastCreateUserID != 7 {
		t.Errorf("repo userID: want 7, got %d", repo.LastCreateUserID)
	}
}

// TestService_List — delegates with filter intact.
func TestService_List(t *testing.T) {
	rows := []Note{{ID: 1, Title: "a"}, {ID: 2, Title: "b"}}
	repo := &fakeRepository{ListNotes: rows}
	svc := NewService(repo)

	got, err := svc.List(context.Background(), 1, ListFilter{Search: "x", Limit: 5})

	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 rows, got %d", len(got))
	}
	if repo.LastListFilter.Search != "x" || repo.LastListFilter.Limit != 5 {
		t.Errorf("filter not propagated: %+v", repo.LastListFilter)
	}
}

// TestService_Delete — error path.
func TestService_Delete(t *testing.T) {
	repo := &fakeRepository{DeleteErr: ErrNotFound}
	svc := NewService(repo)

	err := svc.Delete(context.Background(), 1, 99)

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if repo.DeleteCalls != 1 {
		t.Errorf("repo.Delete calls: want 1, got %d", repo.DeleteCalls)
	}
}
