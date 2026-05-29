package todo

import "context"

// Service holds business rules. Day 15 removed the Title == "" checks from
// Create/Update — the validator owns format rules now. What's left here are
// rules the validator can't express.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) List(ctx context.Context, f ListFilter) ([]Todo, error) { return s.repo.List(ctx, f) }
func (s *Service) Get(ctx context.Context, id int64) (Todo, error)         { return s.repo.Get(ctx, id) }

func (s *Service) Create(ctx context.Context, in CreateRequest) (Todo, error) {
	return s.repo.Create(ctx, in)
}

func (s *Service) Update(ctx context.Context, id int64, in UpdateRequest) (Todo, error) {
	return s.repo.Update(ctx, id, in)
}

func (s *Service) Patch(ctx context.Context, id int64, in PatchRequest) (Todo, error) {
	// Cross-field business rule the validator can't express: at least one
	// field must be provided.
	if in.Title == nil && in.Done == nil {
		return Todo{}, ErrNothingToUpdate
	}
	return s.repo.Patch(ctx, id, in)
}

func (s *Service) Delete(ctx context.Context, id int64) error { return s.repo.Delete(ctx, id) }
