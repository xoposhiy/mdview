package translate

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

type counting struct {
	calls atomic.Int32
	err   error
}

func (c *counting) Translate(_ context.Context, text, target string) (Result, error) {
	c.calls.Add(1)
	if c.err != nil {
		return Result{}, c.err
	}
	return Result{Text: text + "@" + target, Source: "EN", Target: target}, nil
}

func TestCachedHitAndMiss(t *testing.T) {
	ctx := context.Background()
	inner := &counting{}
	c := Cached(inner)

	first, _ := c.Translate(ctx, "a", "RU")
	second, _ := c.Translate(ctx, "a", "RU")
	if inner.calls.Load() != 1 || first != second {
		t.Errorf("same request: calls=%d first=%+v second=%+v", inner.calls.Load(), first, second)
	}
	c.Translate(ctx, "b", "RU")
	if inner.calls.Load() != 2 {
		t.Errorf("different text: calls=%d, want 2", inner.calls.Load())
	}
	c.Translate(ctx, "a", "DE")
	if inner.calls.Load() != 3 {
		t.Errorf("different target: calls=%d, want 3", inner.calls.Load())
	}
}

func TestCachedDoesNotKeepErrors(t *testing.T) {
	ctx := context.Background()
	inner := &counting{err: ErrTransient}
	c := Cached(inner)
	if _, err := c.Translate(ctx, "a", "RU"); !errors.Is(err, ErrTransient) {
		t.Fatalf("err = %v", err)
	}
	inner.err = nil
	res, err := c.Translate(ctx, "a", "RU")
	if err != nil || res.Text != "a@RU" || inner.calls.Load() != 2 {
		t.Errorf("retry: res=%+v err=%v calls=%d", res, err, inner.calls.Load())
	}
}

func TestCachedConcurrent(t *testing.T) {
	ctx := context.Background()
	c := Cached(&counting{})
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				text := string(rune('a' + (i+j)%5))
				if res, err := c.Translate(ctx, text, "RU"); err != nil || res.Text != text+"@RU" {
					t.Errorf("res=%+v err=%v", res, err)
				}
			}
		}(i)
	}
	wg.Wait()
}
