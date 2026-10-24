package store

import (
	"cmp"
	"context"
	"slices"
	"sync"

	"ddia/hw1/internal/models"
)

type Store struct {
	mu     sync.RWMutex
	nextID int64
	items  map[int64]models.Item
}

func New() *Store {
	return &Store{nextID: 1, items: make(map[int64]models.Item)}
}

func (s *Store) Create(ctx context.Context, name, value string) (models.Item, error) {
	if err := ctx.Err(); err != nil {
		return models.Item{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	it := models.Item{ID: s.nextID, Name: name, Value: value}
	s.items[it.ID] = it
	s.nextID++

	return it, nil
}

func (s *Store) Get(ctx context.Context, id int64) (models.Item, error) {
	if err := ctx.Err(); err != nil {
		return models.Item{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	it, ok := s.items[id]
	if !ok {
		return models.Item{}, models.ErrNotFound
	}

	return it, nil
}

func (s *Store) Update(ctx context.Context, id int64, name, value string) (models.Item, error) {
	if err := ctx.Err(); err != nil {
		return models.Item{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.items[id]; !ok {
		return models.Item{}, models.ErrNotFound
	}
	it := models.Item{ID: id, Name: name, Value: value}
	s.items[id] = it

	return it, nil
}

func (s *Store) List(ctx context.Context) ([]models.Item, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]models.Item, 0, len(s.items))
	for _, it := range s.items {
		out = append(out, it)
	}
	slices.SortFunc(out, func(a, b models.Item) int { return cmp.Compare(a.ID, b.ID) })

	return out, nil
}
