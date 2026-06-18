package notes

import (
	"context"

	"day30/internal/audit"
	"day30/internal/dbtx"
)

type Service struct {
	repo  Repository
	audit audit.Repository
	tx    *dbtx.Transactor
}

func NewService(repo Repository, auditRepo audit.Repository, tx *dbtx.Transactor) *Service {
	return &Service{repo: repo, audit: auditRepo, tx: tx}
}

func (s *Service) List(ctx context.Context, userID int64, f ListFilter) (ListPage, error) {
	return s.repo.List(ctx, userID, f)
}
func (s *Service) Get(ctx context.Context, userID, id int64) (Note, error) {
	return s.repo.Get(ctx, userID, id)
}

func (s *Service) Create(ctx context.Context, userID int64, in CreateRequest) (Note, error) {
	var out Note
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		n, err := s.repo.Create(ctx, userID, in)
		if err != nil {
			return err
		}
		out = n
		return s.audit.Log(ctx, userID, audit.ActionNoteCreated, audit.TargetNote, n.ID, map[string]any{
			"title": n.Title,
		})
	})
	return out, err
}

func (s *Service) Update(ctx context.Context, userID, id int64, in UpdateRequest) (Note, error) {
	var out Note
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		n, err := s.repo.Update(ctx, userID, id, in)
		if err != nil {
			return err
		}
		out = n
		return s.audit.Log(ctx, userID, audit.ActionNoteUpdated, audit.TargetNote, n.ID, map[string]any{
			"title": n.Title,
		})
	})
	return out, err
}

func (s *Service) Patch(ctx context.Context, userID, id int64, in PatchRequest) (Note, error) {
	if in.Title == nil && in.Body == nil {
		return Note{}, ErrNothingToUpdate
	}
	var out Note
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		n, err := s.repo.Patch(ctx, userID, id, in)
		if err != nil {
			return err
		}
		out = n
		meta := map[string]any{}
		if in.Title != nil {
			meta["title_changed"] = true
		}
		if in.Body != nil {
			meta["body_changed"] = true
		}
		return s.audit.Log(ctx, userID, audit.ActionNotePatched, audit.TargetNote, n.ID, meta)
	})
	return out, err
}

func (s *Service) Delete(ctx context.Context, userID, id int64) error {
	return s.tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.repo.Delete(ctx, userID, id); err != nil {
			return err
		}
		return s.audit.Log(ctx, userID, audit.ActionNoteDeleted, audit.TargetNote, id, nil)
	})
}
