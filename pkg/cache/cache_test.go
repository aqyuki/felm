package cache

import (
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	t.Parallel()

	exp := 1 * time.Minute
	cache := New[int](exp)

	if cache == nil {
		t.Error("expected cache to be not nil but received nil")
	}
}

func TestSet(t *testing.T) {
	t.Parallel()

	exp := 1 * time.Minute
	cache := New[int](exp)

	if err := cache.Set("key", 1); err != nil {
		t.Errorf("expected err to be nil but received %v", err)
	}
}

func TestGet(t *testing.T) {
	t.Parallel()

	t.Run("found key", func(t *testing.T) {
		t.Parallel()

		exp := 1 * time.Minute
		cache := New[int](exp)
		if err := cache.Set("key", 1); err != nil {
			t.Errorf("expected err to be nil but received %v", err)
		}

		value, err := cache.Get("key")
		if err != nil {
			t.Error("expected err to be nil but received", err)
		}
		if value != 1 {
			t.Errorf("expected value to be 1 but received %v", value)
		}
	})

	t.Run("not found key", func(t *testing.T) {
		t.Parallel()

		exp := 1 * time.Minute
		cache := New[int](exp)

		_, err := cache.Get("key")
		if err != ErrNotFound {
			t.Errorf("expected err to be %v but received %v", ErrNotFound, err)
		}
	})
}
