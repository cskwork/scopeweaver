package server

import (
	"context"
	"errors"
	"iter"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Autumn-27/norma/llm"
)

// fakeProvider records the peak number of concurrent Stream calls and holds each
// call open for hold, so a test can observe whether an admission semaphore bounds
// the overlap.
type fakeProvider struct {
	mu       sync.Mutex
	inFlight int
	peak     int
	hold     time.Duration
}

func (f *fakeProvider) Stream(ctx context.Context, req llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {
		f.mu.Lock()
		f.inFlight++
		if f.inFlight > f.peak {
			f.peak = f.inFlight
		}
		f.mu.Unlock()
		select {
		case <-time.After(f.hold):
		case <-ctx.Done():
		}
		f.mu.Lock()
		f.inFlight--
		f.mu.Unlock()
		yield(llm.StreamEvent{Type: llm.SETextDelta, Text: "ok"}, nil)
	}
}

func (f *fakeProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	for range f.Stream(ctx, req) {
	}
	return llm.Message{Role: "assistant"}, "stop", llm.Usage{}, nil
}

func (f *fakeProvider) Peak() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.peak
}

// TestConcurrencyProviderBoundsInFlight verifies the semaphore never lets more
// than cap requests overlap, regardless of how many goroutines stream at once.
func TestConcurrencyProviderBoundsInFlight(t *testing.T) {
	const capN = 2
	fake := &fakeProvider{hold: 50 * time.Millisecond}
	p := &concurrencyProvider{inner: fake, sem: make(chan struct{}, capN)}

	var wg sync.WaitGroup
	var got atomic.Int64
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range p.Stream(context.Background(), llm.CompletionRequest{}) {
				got.Add(1)
			}
		}()
	}
	wg.Wait()

	if fake.Peak() > capN {
		t.Errorf("peak concurrency %d exceeded cap %d", fake.Peak(), capN)
	}
	if got.Load() != 10 {
		t.Errorf("expected 10 completed streams, got %d", got.Load())
	}
}

// TestConcurrencyProviderUnlimitedWhenZeroCap verifies cap 0 passes through
// without bounding (the unlimited default).
func TestConcurrencyProviderUnlimitedWhenZeroCap(t *testing.T) {
	fake := &fakeProvider{hold: 30 * time.Millisecond}
	p := &concurrencyProvider{inner: fake, sem: make(chan struct{}, 0)} // unlimited

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range p.Stream(context.Background(), llm.CompletionRequest{}) {
			}
		}()
	}
	wg.Wait()
	if fake.Peak() < 2 {
		t.Errorf("expected overlapping streams with cap 0, peak was %d", fake.Peak())
	}
}

// TestConcurrencyProviderHonorsContextCancel verifies a waiter blocked on a full
// semaphore returns ctx.Err() instead of hanging.
func TestConcurrencyProviderHonorsContextCancel(t *testing.T) {
	fake := &fakeProvider{hold: 2 * time.Second}
	p := &concurrencyProvider{inner: fake, sem: make(chan struct{}, 1)}

	// Occupy the only slot with a long stream.
	go func() {
		for range p.Stream(context.Background(), llm.CompletionRequest{}) {
		}
	}()
	time.Sleep(50 * time.Millisecond) // let the first stream acquire

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	errCh := make(chan error, 1)
	go func() {
		var streamErr error
		for _, err := range p.Stream(ctx, llm.CompletionRequest{}) {
			if err != nil {
				streamErr = err
			}
		}
		errCh <- streamErr
	}()
	err := <-errCh
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context deadline exceeded, got %v", err)
	}
	if time.Since(start) > 1*time.Second {
		t.Errorf("cancel took too long: %v", time.Since(start))
	}
}
