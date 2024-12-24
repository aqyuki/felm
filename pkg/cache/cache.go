package cache

import (
	"errors"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/samber/lo"
)

var (
	ErrNotFound = errors.New("not found")
)

type Cache[T any] struct {
	cache             *cache.Cache
	defaultExpiration time.Duration
}

func New[T any](exp time.Duration) *Cache[T] {
	return &Cache[T]{
		cache:             cache.New(exp, exp),
		defaultExpiration: exp,
	}
}

func (c *Cache[T]) Set(key string, value T) error {
	c.cache.Set(key, value, c.defaultExpiration)
	return nil
}

func (c *Cache[T]) Get(key string) (T, error) {
	value, found := c.cache.Get(key)
	if !found {
		return lo.Empty[T](), ErrNotFound
	}
	return value.(T), nil
}
