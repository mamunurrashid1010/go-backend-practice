package todo

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// Repository — UNCHANGED interface from Day 15.
type Repository interface {
	List(ctx context.Context, f ListFilter) ([]Todo, error)
	Get(ctx context.Context, id int64) (Todo, error)
	Create(ctx context.Context, in CreateRequest) (Todo, error)
	Update(ctx context.Context, id int64, in UpdateRequest) (Todo, error)
	Patch(ctx context.Context, id int64, in PatchRequest) (Todo, error)
	Delete(ctx context.Context, id int64) error
}

// InMemoryRepository — Day 16 adds duplicate-title detection so it returns
// the same *ConflictError the Postgres repo does. Implementations should be
// behaviourally interchangeable.
type InMemoryRepository struct {
	mu     sync.Mutex
	nextID int64
	items  map[int64]Todo
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{nextID: 1, items: make(map[int64]Todo)}
}

// titleTaken reports whether some OTHER row (id != excludeID) has this title.
func (r *InMemoryRepository) titleTaken(title string, excludeID int64) bool {
	for _, t := range r.items {
		if t.ID != excludeID && t.Title == title {
			return true
		}
	}
	return false
}

func (r *InMemoryRepository) List(_ context.Context, f ListFilter) ([]Todo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Todo, 0, len(r.items))
	search := strings.ToLower(f.Search)
	for _, t := range r.items {
		if f.Done != nil && t.Done != *f.Done {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(t.Title), search) {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

func (r *InMemoryRepository) Get(_ context.Context, id int64) (Todo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.items[id]
	if !ok {
		return Todo{}, ErrNotFound
	}
	return t, nil
}

func (r *InMemoryRepository) Create(_ context.Context, in CreateRequest) (Todo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.titleTaken(in.Title, 0) {
		return Todo{}, &ConflictError{Field: "title", Value: in.Title}
	}
	now := time.Now().UTC()
	t := Todo{ID: r.nextID, Title: in.Title, Done: in.Done, CreatedAt: now, UpdatedAt: now}
	r.items[r.nextID] = t
	r.nextID++
	return t, nil
}

func (r *InMemoryRepository) Update(_ context.Context, id int64, in UpdateRequest) (Todo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.items[id]
	if !ok {
		return Todo{}, ErrNotFound
	}
	if r.titleTaken(in.Title, id) {
		return Todo{}, &ConflictError{Field: "title", Value: in.Title}
	}
	t.Title = in.Title
	t.Done = in.Done
	t.UpdatedAt = time.Now().UTC()
	r.items[id] = t
	return t, nil
}

func (r *InMemoryRepository) Patch(_ context.Context, id int64, in PatchRequest) (Todo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.items[id]
	if !ok {
		return Todo{}, ErrNotFound
	}
	if in.Title != nil {
		if r.titleTaken(*in.Title, id) {
			return Todo{}, &ConflictError{Field: "title", Value: *in.Title}
		}
		t.Title = *in.Title
	}
	if in.Done != nil {
		t.Done = *in.Done
	}
	t.UpdatedAt = time.Now().UTC()
	r.items[id] = t
	return t, nil
}

func (r *InMemoryRepository) Delete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}
