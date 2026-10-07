package translate

import (
	"context"
	"sync"
)

type cacheKey struct{ text, target string }

type cached struct {
	inner Translator
	mu    sync.Mutex
	m     map[cacheKey]Result
}

// Cached wraps t with an in-memory cache keyed by text and target. Errors are
// not cached. Entries live as long as the returned Translator.
func Cached(t Translator) Translator {
	return &cached{inner: t, m: map[cacheKey]Result{}}
}

func (c *cached) Translate(ctx context.Context, text, target string) (Result, error) {
	k := cacheKey{text, target}
	c.mu.Lock()
	res, ok := c.m[k]
	c.mu.Unlock()
	if ok {
		return res, nil
	}
	res, err := c.inner.Translate(ctx, text, target)
	if err != nil {
		return Result{}, err
	}
	c.mu.Lock()
	c.m[k] = res
	c.mu.Unlock()
	return res, nil
}
