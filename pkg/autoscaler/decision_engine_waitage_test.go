package autoscaler

import (
	"context"
	"strings"
	"testing"
	"time"
)

func newTestEngine(waitSeconds int) *DecisionEngine {
	return NewDecisionEngine(&Config{
		Enabled:            true,
		StarvationTime:     300,
		ScaleUpWaitSeconds: waitSeconds,
	}, NewResourceCalculator(nil, nil, nil))
}

func baseEP() *EndpointConfig {
	return &EndpointConfig{
		Name:               "ep",
		MinReplicas:        0,
		MaxReplicas:        10,
		Replicas:           0,
		ScaleUpThreshold:   3,
		ScaleUpWaitSeconds: 60,
		ScaleUpCooldown:    0,
		PendingKnown:       true,
		Priority:           50,
	}
}

func unlimited() *Resources {
	return &Resources{GPUCount: -1, CPUCores: -1, MemoryGB: -1}
}

// T1: pending=0, no wait → no scale
func TestShouldScaleUp_T1_NoPending(t *testing.T) {
	e := newTestEngine(60)
	ep := baseEP()
	ep.PendingTasks = 0
	if d := e.shouldScaleUp(context.Background(), ep, unlimited()); d != nil {
		t.Fatalf("expected nil, got %+v", d)
	}
}

// T2: pending >= threshold, age not reached → scale via queue
func TestShouldScaleUp_T2_QueuePath(t *testing.T) {
	e := newTestEngine(60)
	ep := baseEP()
	ep.PendingTasks = 3
	ep.OldestPendingSince = time.Now().Add(-10 * time.Second) // age < 60
	d := e.shouldScaleUp(context.Background(), ep, unlimited())
	if d == nil || !d.Approved {
		t.Fatalf("expected approved scale, got %+v", d)
	}
	if !strings.Contains(d.Reason, "queue") {
		t.Fatalf("reason=%s", d.Reason)
	}
}

// T3: pending=1 < threshold, age >= wait, min=0 → scale wait-age, Desired>=1
func TestShouldScaleUp_T3_WaitAgePath(t *testing.T) {
	e := newTestEngine(60)
	ep := baseEP()
	ep.PendingTasks = 1
	ep.ScaleUpThreshold = 3
	ep.OldestPendingSince = time.Now().Add(-120 * time.Second)
	d := e.shouldScaleUp(context.Background(), ep, unlimited())
	if d == nil || !d.Approved {
		t.Fatalf("expected approved wait-age scale, got %+v", d)
	}
	if d.DesiredReplicas < 1 {
		t.Fatalf("desired=%d", d.DesiredReplicas)
	}
	if !strings.Contains(d.Reason, "wait-age") {
		t.Fatalf("reason=%s", d.Reason)
	}
}

// T4: age not reached and pending < threshold → no scale
func TestShouldScaleUp_T4_BelowBoth(t *testing.T) {
	e := newTestEngine(60)
	ep := baseEP()
	ep.PendingTasks = 1
	ep.OldestPendingSince = time.Now().Add(-10 * time.Second)
	if d := e.shouldScaleUp(context.Background(), ep, unlimited()); d != nil {
		t.Fatalf("expected nil, got %+v", d)
	}
}

// T5: at maxReplicas → no scale
func TestShouldScaleUp_T5_AtMax(t *testing.T) {
	e := newTestEngine(60)
	ep := baseEP()
	ep.Replicas = 10
	ep.MaxReplicas = 10
	ep.PendingTasks = 100
	ep.OldestPendingSince = time.Now().Add(-time.Hour)
	if d := e.shouldScaleUp(context.Background(), ep, unlimited()); d != nil {
		t.Fatalf("expected nil, got %+v", d)
	}
}

// T6: cooldown → no scale
func TestShouldScaleUp_T6_Cooldown(t *testing.T) {
	e := newTestEngine(60)
	ep := baseEP()
	ep.PendingTasks = 5
	ep.ScaleUpCooldown = 300
	ep.LastScaleTime = time.Now()
	if d := e.shouldScaleUp(context.Background(), ep, unlimited()); d != nil {
		t.Fatalf("expected nil, got %+v", d)
	}
}

// T7: oldest age from persisted created_at survives "restart" (we just pass OldestPendingSince)
func TestShouldScaleUp_T7_AgeContinuous(t *testing.T) {
	e := newTestEngine(60)
	ep := baseEP()
	ep.PendingTasks = 1
	ep.ScaleUpThreshold = 99
	// Simulate control-plane restart: FirstPendingTime would reset, but OldestPendingSince is from DB
	ep.FirstPendingTime = time.Time{}
	ep.OldestPendingSince = time.Now().Add(-90 * time.Second)
	d := e.shouldScaleUp(context.Background(), ep, unlimited())
	if d == nil {
		t.Fatal("expected scale from persisted oldest age")
	}
	if !strings.Contains(d.Reason, "wait-age") {
		t.Fatalf("reason=%s", d.Reason)
	}
}

// T8: pending query failed → do not scale up blindly
func TestShouldScaleUp_T8_PendingUnknown(t *testing.T) {
	e := newTestEngine(60)
	ep := baseEP()
	ep.PendingKnown = false
	ep.PendingTasks = 0
	if d := e.shouldScaleUp(context.Background(), ep, unlimited()); d != nil {
		t.Fatalf("expected nil when pending unknown, got %+v", d)
	}
}

func TestShouldScaleUp_QueueAndWaitAgeReason(t *testing.T) {
	e := newTestEngine(60)
	ep := baseEP()
	ep.PendingTasks = 5
	ep.OldestPendingSince = time.Now().Add(-120 * time.Second)
	d := e.shouldScaleUp(context.Background(), ep, unlimited())
	if d == nil {
		t.Fatal("expected decision")
	}
	if !strings.Contains(d.Reason, "queue+wait-age") {
		t.Fatalf("reason=%s", d.Reason)
	}
}
