package alerts

import (
	"testing"
	"time"
)

// The retry schedule is what an operator relies on when a channel is down, so it
// is asserted in full rather than "increases".
func TestBackoffSchedule(t *testing.T) {
	want := []time.Duration{
		30 * time.Second,
		time.Minute,
		2 * time.Minute,
		4 * time.Minute,
		8 * time.Minute,
		16 * time.Minute,
		32 * time.Minute,
		time.Hour, // capped
		time.Hour,
	}
	for i, expected := range want {
		if got := Backoff(i + 1); got != expected {
			t.Errorf("Backoff(%d) = %v, want %v", i+1, got, expected)
		}
	}
	if Backoff(0) != 30*time.Second {
		t.Error("attempt 0 must behave like attempt 1")
	}
}

func TestNextAttemptStopsAtTheAllowance(t *testing.T) {
	now := time.Date(2026, 10, 30, 9, 15, 0, 0, time.UTC)

	next, terminal := NextAttempt(1, 3, now)
	if terminal {
		t.Fatal("first failure must be retried")
	}
	if next == nil || !next.Equal(now.Add(30*time.Second)) {
		t.Fatalf("next = %v", next)
	}

	if _, terminal := NextAttempt(2, 3, now); terminal {
		t.Fatal("second failure of three must still be retried")
	}

	next, terminal = NextAttempt(3, 3, now)
	if !terminal || next != nil {
		t.Fatalf("third failure of three must be terminal, got next=%v terminal=%v", next, terminal)
	}

	// One attempt allowed means one attempt, not two.
	if _, terminal := NextAttempt(1, 1, now); !terminal {
		t.Fatal("a single-attempt delivery must be terminal on its first failure")
	}
}

func TestShouldDeliverHonoursCooldown(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

	if !ShouldDeliver(now, time.Time{}, false, time.Hour) {
		t.Fatal("first observation must deliver")
	}
	if !ShouldDeliver(now, now.Add(-2*time.Hour), true, time.Hour) {
		t.Fatal("an observation after the cooldown must deliver")
	}
	if ShouldDeliver(now, now.Add(-30*time.Minute), true, time.Hour) {
		t.Fatal("an observation inside the cooldown must be suppressed")
	}
	if ShouldDeliver(now, now.Add(-3599*time.Second), true, time.Hour) {
		t.Fatal("one second short of the cooldown must still suppress")
	}
	if !ShouldDeliver(now, now.Add(-time.Hour), true, time.Hour) {
		t.Fatal("exactly the cooldown must deliver")
	}
	// Cooldown 0 means every new observation is delivered.
	if !ShouldDeliver(now, now.Add(-time.Second), true, 0) {
		t.Fatal("a zero cooldown must not suppress")
	}
}

// The fingerprint is the deduplication key, so it must separate facts that differ
// and agree on facts that do not.
func TestFingerprintSeparatesDistinctObservations(t *testing.T) {
	base := Fingerprint("rule-1", "NABIL", "2026-10-29", "close", "545")
	if base != Fingerprint("rule-1", "NABIL", "2026-10-29", "close", "545") {
		t.Fatal("the same observation must produce the same fingerprint")
	}
	others := []string{
		Fingerprint("rule-2", "NABIL", "2026-10-29", "close", "545"),
		Fingerprint("rule-1", "ADBL", "2026-10-29", "close", "545"),
		Fingerprint("rule-1", "NABIL", "2026-10-28", "close", "545"),
		Fingerprint("rule-1", "NABIL", "2026-10-29", "volume", "545"),
		Fingerprint("rule-1", "NABIL", "2026-10-29", "close", "546"),
	}
	for i, other := range others {
		if other == base {
			t.Errorf("fingerprint %d collides with the base observation", i)
		}
	}
	if len(base) != 64 {
		t.Fatalf("fingerprint length = %d, want a sha256 hex digest", len(base))
	}
}
