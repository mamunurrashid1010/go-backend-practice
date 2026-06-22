package notes

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/sync/singleflight"

	"day32/internal/audit"
	"day32/internal/cache"
	"day32/internal/dbtx"
	"day32/internal/logging"
)

// Service — Day 32 wires a *cache.Cache and a singleflight.Group.
// Get is cache-aside (Redis -> DB -> Redis fill) with concurrent
// misses on the same key coalesced. Write methods (Update/Patch/
// Delete) invalidate AFTER tx commit — see README §2 for why that
// order is the canonical one.
type Service struct {
	repo  Repository
	audit audit.Repository
	tx    *dbtx.Transactor

	cache *cache.Cache
	ttl   time.Duration
	sf    singleflight.Group
}

func NewService(
	repo Repository,
	auditRepo audit.Repository,
	tx *dbtx.Transactor,
	c *cache.Cache,
	ttl time.Duration,
) *Service {
	return &Service{repo: repo, audit: auditRepo, tx: tx, cache: c, ttl: ttl}
}

func (s *Service) List(ctx context.Context, userID int64, f ListFilter) (ListPage, error) {
	return s.repo.List(ctx, userID, f)
}

// Get — cache-aside. 1) try Redis. 2) on miss, fetch via singleflight
// so a thundering herd on one key only hits Postgres once. 3) fill
// the cache on the way out (best-effort; a write failure does not
// fail the request).
func (s *Service) Get(ctx context.Context, userID, id int64) (Note, error) {
	key := noteKey(userID, id)

	var cached Note
	hit, err := s.cache.GetJSON(ctx, key, &cached)
	if err != nil {
		// Bad cache value is a "miss + log it" — don't propagate.
		logging.From(ctx).WarnContext(ctx, "cache get failed", slog.String("key", key), slog.Any("err", err))
	}
	if hit {
		return cached, nil
	}

	v, err, _ := s.sf.Do(key, func() (any, error) {
		return s.repo.Get(ctx, userID, id)
	})
	if err != nil {
		return Note{}, err
	}
	n := v.(Note)

	if err := s.cache.SetJSON(ctx, key, n, s.ttl); err != nil {
		logging.From(ctx).WarnContext(ctx, "cache set failed", slog.String("key", key), slog.Any("err", err))
	}
	return n, nil
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
	if err == nil {
		// New row — no existing cache entry, but populating now would
		// race with a delete and is rarely useful. Skip.
	}
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
	if err == nil {
		s.invalidate(ctx, userID, id)
	}
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
	if err == nil {
		s.invalidate(ctx, userID, id)
	}
	return out, err
}

func (s *Service) Delete(ctx context.Context, userID, id int64) error {
	err := s.tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.repo.Delete(ctx, userID, id); err != nil {
			return err
		}
		return s.audit.Log(ctx, userID, audit.ActionNoteDeleted, audit.TargetNote, id, nil)
	})
	if err == nil {
		s.invalidate(ctx, userID, id)
	}
	return err
}

// invalidate is best-effort. A Redis blip shouldn't fail a successful
// write, but we log it because stale cache is a real bug to chase.
func (s *Service) invalidate(ctx context.Context, userID, id int64) {
	key := noteKey(userID, id)
	if err := s.cache.Delete(ctx, key); err != nil {
		logging.From(ctx).ErrorContext(ctx, "cache invalidate failed",
			slog.String("key", key), slog.Any("err", err))
	}
}

func noteKey(userID, id int64) string {
	return fmt.Sprintf("note:%d:%d", userID, id)
}
