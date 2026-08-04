package anemo

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"venti/internal/bard"
)

type fakeBard struct {
	id        int64
	dead      int32
	songs     int32
	birthTime time.Time
	lastSong  time.Time
	restCalls int32
}

func (f *fakeBard) Sing(ctx context.Context, path string, lyrics []string, melody []byte) ([]byte, error) {
	return []byte("ok"), nil
}

func (f *fakeBard) Rest() error {
	atomic.AddInt32(&f.restCalls, 1)
	atomic.StoreInt32(&f.dead, 1)
	return nil
}

func (f *fakeBard) IsHealthy() bool { return atomic.LoadInt32(&f.dead) == 0 }
func (f *fakeBard) GetSongsCount() int {
	return int(atomic.LoadInt32(&f.songs))
}
func (f *fakeBard) GetBirthTime() time.Time    { return f.birthTime }
func (f *fakeBard) GetLastSongTime() time.Time { return f.lastSong }

type fakeLogger struct{}

func (fakeLogger) Debug(msg string, args ...interface{}) {}
func (fakeLogger) Info(msg string, args ...interface{})  {}
func (fakeLogger) Warn(msg string, args ...interface{})  {}
func (fakeLogger) Error(msg string, args ...interface{}) {}

func newTestPool(t *testing.T, cfg *PowerConfig) (*AnemoPower, *int64) {
	t.Helper()
	var counter int64
	factory := func() (bard.Bard, error) {
		id := atomic.AddInt64(&counter, 1)
		now := time.Now()
		return &fakeBard{id: id, birthTime: now, lastSong: now}, nil
	}
	pool, err := NewAnemoPower(cfg, factory, fakeLogger{})
	if err != nil {
		t.Fatalf("NewAnemoPower: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool, &counter
}

func TestPoolSummonsMinBards(t *testing.T) {
	pool, counter := newTestPool(t, &PowerConfig{
		MinBards: 3, MaxBards: 10,
		IdleTimeout: 60 * time.Second, MaxLifetime: time.Hour, MaxSongsPerBard: 100,
	})
	if got := atomic.LoadInt64(counter); got != 3 {
		t.Errorf("summoned %d bards, want 3", got)
	}
	if got := atomic.LoadInt32(&pool.activeBards); got != 3 {
		t.Errorf("activeBards = %d, want 3", got)
	}
}

func TestPoolCallReleaseRoundtrip(t *testing.T) {
	pool, _ := newTestPool(t, &PowerConfig{
		MinBards: 1, MaxBards: 10,
		IdleTimeout: 60 * time.Second, MaxLifetime: time.Hour, MaxSongsPerBard: 100,
	})

	b, err := pool.CallBard(context.Background())
	if err != nil {
		t.Fatalf("CallBard: %v", err)
	}
	pool.ReleaseBard(b)

	// Бард должен вернуться в таверну живым
	b2, err := pool.CallBard(context.Background())
	if err != nil {
		t.Fatalf("CallBard after release: %v", err)
	}
	if b2 != b {
		t.Errorf("got different bard after release")
	}
	pool.ReleaseBard(b2)
}

func TestPoolCallTimeout(t *testing.T) {
	pool, _ := newTestPool(t, &PowerConfig{
		MinBards: 1, MaxBards: 1,
		IdleTimeout: 60 * time.Second, MaxLifetime: time.Hour, MaxSongsPerBard: 100,
	})

	// Забираем единственного барда
	b, err := pool.CallBard(context.Background())
	if err != nil {
		t.Fatalf("CallBard: %v", err)
	}

	// Второй вызов должен упереться в таймаут
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := pool.CallBard(ctx); err == nil {
		t.Errorf("expected timeout error, got nil")
	}
	pool.ReleaseBard(b)
}

func TestPoolApplyConfigUpdates(t *testing.T) {
	pool, counter := newTestPool(t, &PowerConfig{
		MinBards: 1, MaxBards: 10,
		IdleTimeout: 60 * time.Second, MaxLifetime: time.Hour, MaxSongsPerBard: 100,
	})

	newCfg := &PowerConfig{
		MinBards: 3, MaxBards: 5,
		IdleTimeout: 10 * time.Second, MaxLifetime: time.Hour, MaxSongsPerBard: 50,
	}
	if err := pool.ApplyConfig(newCfg); err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}

	if got := atomic.LoadInt64(counter); got != 3 {
		t.Errorf("after ApplyConfig %d bards summoned, want 3", got)
	}
	if pool.config.MinBards != 3 || pool.config.MaxBards != 5 {
		t.Errorf("config not updated: %+v", pool.config)
	}
	if pool.config.IdleTimeout != 10*time.Second || pool.config.MaxSongsPerBard != 50 {
		t.Errorf("tunables not updated: %+v", pool.config)
	}
}

func TestPoolApplyConfigInvalid(t *testing.T) {
	pool, _ := newTestPool(t, &PowerConfig{
		MinBards: 1, MaxBards: 10,
		IdleTimeout: 60 * time.Second, MaxLifetime: time.Hour, MaxSongsPerBard: 100,
	})

	if err := pool.ApplyConfig(&PowerConfig{MinBards: 10, MaxBards: 2}); err == nil {
		t.Errorf("expected error for min > max")
	}
}

func TestPoolCloseRestsBards(t *testing.T) {
	cfg := &PowerConfig{
		MinBards: 2, MaxBards: 10,
		IdleTimeout: 60 * time.Second, MaxLifetime: time.Hour, MaxSongsPerBard: 100,
	}
	var counter int64
	factory := func() (bard.Bard, error) {
		id := atomic.AddInt64(&counter, 1)
		now := time.Now()
		return &fakeBard{id: id, birthTime: now, lastSong: now}, nil
	}
	pool, err := NewAnemoPower(cfg, factory, fakeLogger{})
	if err != nil {
		t.Fatalf("NewAnemoPower: %v", err)
	}

	bards := []bard.Bard{}
	for i := 0; i < 2; i++ {
		b, err := pool.CallBard(context.Background())
		if err != nil {
			t.Fatalf("CallBard: %v", err)
		}
		bards = append(bards, b)
		pool.ReleaseBard(b)
	}

	pool.Close()

	for _, b := range bards {
		if b.IsHealthy() {
			t.Errorf("bard %p still healthy after Close", b)
		}
	}
}
