package todo

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// Store is a concurrent-safe in-memory store of Todo records.
// Every method takes the mutex with `defer Unlock` — no exceptions.
//
// On Day 12 this whole file gets replaced by a Postgres implementation,
// and nothing else in the project changes. That's the lesson.
type Store struct {
	mu     sync.Mutex
	nextID int
	items  map[int]Todo
}

func NewStore() *Store {
	return &Store{nextID: 1, items: make(map[int]Todo)}
}

// List returns todos matching the filter, sorted by ID for stable output.
func (s *Store) List(f ListFilter) []Todo {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Todo, 0, len(s.items))
	search := strings.ToLower(f.Search)
	for _, t := range s.items {
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
	return out
}

func (s *Store) Get(id int) (Todo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.items[id]
	return t, ok
}

func (s *Store) Create(req CreateRequest) Todo {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	t := Todo{
		ID:        s.nextID,
		Title:     req.Title,
		Done:      req.Done,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.items[s.nextID] = t
	s.nextID++
	return t
}

// Update replaces title + done. Returns (zero, false) if id not found.
func (s *Store) Update(id int, req UpdateRequest) (Todo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.items[id]
	if !ok {
		return Todo{}, false
	}
	t.Title = req.Title
	t.Done = req.Done
	t.UpdatedAt = time.Now().UTC()
	s.items[id] = t
	return t, true
}

// Patch applies only the fields the client provided (non-nil pointers).
func (s *Store) Patch(id int, req PatchRequest) (Todo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.items[id]
	if !ok {
		return Todo{}, false
	}
	if req.Title != nil {
		t.Title = *req.Title
	}
	if req.Done != nil {
		t.Done = *req.Done
	}
	t.UpdatedAt = time.Now().UTC()
	s.items[id] = t
	return t, true
}

func (s *Store) Delete(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return false
	}
	delete(s.items, id)
	return true
}
