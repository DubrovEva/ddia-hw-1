package store

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ddia/hw1/internal/models"
)

func TestCreateGet(t *testing.T) {
	ctx := t.Context()
	s := New()

	a, err := s.Create(ctx, "a", "1")
	require.NoError(t, err)
	b, err := s.Create(ctx, "b", "2")
	require.NoError(t, err)
	assert.NotEqual(t, a.ID, b.ID)

	got, err := s.Get(ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, a, got)
}

func TestGetMissing(t *testing.T) {
	_, err := New().Get(t.Context(), 42)
	assert.ErrorIs(t, err, models.ErrNotFound)
}

func TestUpdate(t *testing.T) {
	ctx := t.Context()
	s := New()
	it, err := s.Create(ctx, "a", "1")
	require.NoError(t, err)
	want := models.Item{ID: it.ID, Name: "a2", Value: "2"}

	upd, err := s.Update(ctx, it.ID, "a2", "2")
	require.NoError(t, err)
	assert.Equal(t, want, upd)

	got, err := s.Get(ctx, it.ID)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestUpdateMissing(t *testing.T) {
	_, err := New().Update(t.Context(), 999, "x", "y")
	assert.ErrorIs(t, err, models.ErrNotFound)
}

func TestListOrdered(t *testing.T) {
	ctx := t.Context()
	s := New()
	for _, n := range []string{"a", "b", "c"} {
		_, err := s.Create(ctx, n, "")
		require.NoError(t, err)
	}

	list, err := s.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 3)
	assert.IsIncreasing(t, []int64{list[0].ID, list[1].ID, list[2].ID})
}

func TestConcurrentCreate(t *testing.T) {
	ctx := t.Context()
	s := New()
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			_, err := s.Create(ctx, "x", "y")
			assert.NoError(t, err)
		})
	}
	wg.Wait()

	list, err := s.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 100)
}

func TestCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	s := New()

	_, err := s.Create(ctx, "a", "1")
	assert.ErrorIs(t, err, context.Canceled)
	_, err = s.Get(ctx, 1)
	assert.ErrorIs(t, err, context.Canceled)
	_, err = s.Update(ctx, 1, "a", "1")
	assert.ErrorIs(t, err, context.Canceled)
	_, err = s.List(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}
