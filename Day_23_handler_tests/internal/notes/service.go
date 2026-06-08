package notes

import "context"

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) List(ctx context.Context, userID int64, f ListFilter) ([]Note, error) {
	return s.repo.List(ctx, userID, f)
}
func (s *Service) Get(ctx context.Context, userID, id int64) (Note, error) {
	return s.repo.Get(ctx, userID, id)
}
func (s *Service) Create(ctx context.Context, userID int64, in CreateRequest) (Note, error) {
	return s.repo.Create(ctx, userID, in)
}
func (s *Service) Update(ctx context.Context, userID, id int64, in UpdateRequest) (Note, error) {
	return s.repo.Update(ctx, userID, id, in)
}
func (s *Service) Patch(ctx context.Context, userID, id int64, in PatchRequest) (Note, error) {
	if in.Title == nil && in.Body == nil {
		return Note{}, ErrNothingToUpdate
	}
	return s.repo.Patch(ctx, userID, id, in)
}
func (s *Service) Delete(ctx context.Context, userID, id int64) error {
	return s.repo.Delete(ctx, userID, id)
}
