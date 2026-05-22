package todo

import "context"

// Service holds the business rules of the To-Do feature. It takes a
// Repository so the storage can be swapped (in-memory, Postgres, mock)
// without changing this file.
//
// The handler depends on *Service. Day 22 will introduce a Service
// interface only if needed for handler tests.
type Service struct {
	repo Repository
}

// NewService is the canonical "accept interface, return struct" pattern:
// callers get a concrete *Service (with all its methods exposed); the
// service itself only knows about the Repository interface.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// ---- Read paths ---------------------------------------------------------

func (s *Service) List(ctx context.Context, f ListFilter) ([]Todo, error) {
	return s.repo.List(ctx, f)
}

func (s *Service) Get(ctx context.Context, id int64) (Todo, error) {
	return s.repo.Get(ctx, id)
}

// ---- Write paths --------------------------------------------------------

// Create validates the input, then persists. Validation lives here, not in
// the handler — the handler just translates HTTP → domain calls. If we
// later add a CLI or cron path, those callers get the same validation
// for free.
func (s *Service) Create(ctx context.Context, in CreateRequest) (Todo, error) {
	if in.Title == "" {
		return Todo{}, ErrEmptyTitle
	}
	return s.repo.Create(ctx, in)
}

func (s *Service) Update(ctx context.Context, id int64, in UpdateRequest) (Todo, error) {
	if in.Title == "" {
		return Todo{}, ErrEmptyTitle
	}
	return s.repo.Update(ctx, id, in)
}

// Patch is the partial-update path. The rules:
//   - at least one field must be provided
//   - if Title is provided, it must not be empty
func (s *Service) Patch(ctx context.Context, id int64, in PatchRequest) (Todo, error) {
	if in.Title == nil && in.Done == nil {
		return Todo{}, ErrNothingToUpdate
	}
	if in.Title != nil && *in.Title == "" {
		return Todo{}, ErrEmptyTitle
	}
	return s.repo.Patch(ctx, id, in)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}
