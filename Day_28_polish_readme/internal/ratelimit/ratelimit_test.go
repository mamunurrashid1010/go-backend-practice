package ratelimit

import (
	"testing"
	"time"
)

func TestAllow_BurstThenRefill(t *testing.T) {
	l := New(10, 3, time.Minute)
	defer l.Stop()

	for i := 0; i < 3; i++ {
		if !l.Allow("k") {
			t.Fatalf("burst slot %d denied", i)
		}
	}
	if l.Allow("k") {
		t.Fatalf("4th request should be denied (bucket empty)")
	}
	time.Sleep(120 * time.Millisecond)
	if !l.Allow("k") {
		t.Fatalf("after refill, request should be allowed")
	}
}

func TestAllow_KeysAreIndependent(t *testing.T) {
	l := New(10, 1, time.Minute)
	defer l.Stop()

	if !l.Allow("alice") {
		t.Fatalf("alice burst should succeed")
	}
	if l.Allow("alice") {
		t.Fatalf("alice should be exhausted")
	}
	if !l.Allow("bob") {
		t.Fatalf("bob should NOT be affected by alice's bucket")
	}
}

func TestTokens_UnknownKeyReturnsBurst(t *testing.T) {
	l := New(10, 5, time.Minute)
	defer l.Stop()

	if got := l.Tokens("never-seen"); got != 5 {
		t.Fatalf("unknown key tokens: want 5, got %d", got)
	}
}

func TestCleanup_EvictsStaleVisitors(t *testing.T) {
	l := New(10, 5, 200*time.Millisecond)
	defer l.Stop()

	l.Allow("evict-me")
	if l.Size() != 1 {
		t.Fatalf("size before eviction: want 1, got %d", l.Size())
	}
	time.Sleep(500 * time.Millisecond)
	if l.Size() != 0 {
		t.Fatalf("size after eviction: want 0, got %d", l.Size())
	}
}
