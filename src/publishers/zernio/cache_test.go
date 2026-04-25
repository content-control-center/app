package zernio

import (
	"context"
	"sync"
	"testing"
	"time"
)

// memStore is a tiny in-memory SettingsStore for tests. The cache and
// bootstrap tests use it directly so they don't need SQLite.
type memStore struct {
	mu       sync.Mutex
	data     map[string]string
	getCount map[string]int
}

func newMemStore() *memStore {
	return &memStore{data: make(map[string]string), getCount: make(map[string]int)}
}

func (m *memStore) Get(_ context.Context, key string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getCount[key]++
	v, ok := m.data[key]
	return v, ok, nil
}

func (m *memStore) Set(_ context.Context, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func (m *memStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *memStore) gets(key string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getCount[key]
}

func TestCachedSettingsStorePassesThroughNonZernioKeys(t *testing.T) {
	inner := newMemStore()
	_ = inner.Set(context.Background(), "other.key", "v")
	c := NewCachedSettingsStore(inner)

	for i := 0; i < 5; i++ {
		v, ok, err := c.Get(context.Background(), "other.key")
		if err != nil || !ok || v != "v" {
			t.Fatalf("unexpected get: %v %v %v", v, ok, err)
		}
	}
	if got := inner.gets("other.key"); got != 5 {
		t.Errorf("inner gets for non-zernio key: got %d want 5 (no caching)", got)
	}
}

func TestCachedSettingsStoreCachesZernioKeys(t *testing.T) {
	inner := newMemStore()
	_ = inner.Set(context.Background(), SettingProfileID, "abc")
	c := NewCachedSettingsStoreTTL(inner, 50*time.Millisecond)

	for i := 0; i < 5; i++ {
		v, ok, err := c.Get(context.Background(), SettingProfileID)
		if err != nil || !ok || v != "abc" {
			t.Fatalf("unexpected get: %v %v %v", v, ok, err)
		}
	}
	if got := inner.gets(SettingProfileID); got != 1 {
		t.Errorf("inner gets within TTL: got %d want 1", got)
	}

	time.Sleep(60 * time.Millisecond)
	_, _, _ = c.Get(context.Background(), SettingProfileID)
	if got := inner.gets(SettingProfileID); got != 2 {
		t.Errorf("inner gets after TTL: got %d want 2", got)
	}
}

func TestCachedSettingsStoreInvalidatesOnSet(t *testing.T) {
	inner := newMemStore()
	_ = inner.Set(context.Background(), SettingProfileID, "abc")
	c := NewCachedSettingsStore(inner)

	_, _, _ = c.Get(context.Background(), SettingProfileID) // primes cache

	// External-looking write through the cached store should drop the
	// cache entry; the next Get goes to the inner store.
	if err := c.Set(context.Background(), SettingProfileID, "xyz"); err != nil {
		t.Fatal(err)
	}

	v, _, _ := c.Get(context.Background(), SettingProfileID)
	if v != "xyz" {
		t.Errorf("after Set: got %q want xyz", v)
	}
	if got := inner.gets(SettingProfileID); got != 2 {
		t.Errorf("inner gets after invalidation: got %d want 2", got)
	}
}

func TestCachedSettingsStoreInvalidatesOnDelete(t *testing.T) {
	inner := newMemStore()
	_ = inner.Set(context.Background(), SettingProfileID, "abc")
	c := NewCachedSettingsStore(inner)

	_, _, _ = c.Get(context.Background(), SettingProfileID)
	if err := c.Delete(context.Background(), SettingProfileID); err != nil {
		t.Fatal(err)
	}

	_, ok, _ := c.Get(context.Background(), SettingProfileID)
	if ok {
		t.Errorf("after Delete: should report missing")
	}
}
