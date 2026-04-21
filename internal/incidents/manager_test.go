package incidents_test

import (
	"testing"
	"time"

	"docknudge/internal/incidents"
	"docknudge/internal/notifiers"
)

func TestManagerCooldownAndSuppression(t *testing.T) {
	now := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
	manager := incidents.New(10 * time.Minute)

	oom := alert("oom", now)
	if !manager.Allow(oom) {
		t.Fatal("expected initial oom alert to be allowed")
	}
	manager.Record(oom)

	if manager.Allow(alert("die", now.Add(30*time.Second))) {
		t.Fatal("die should be suppressed after oom")
	}
	if manager.Allow(alert("unexpected_stop", now.Add(30*time.Second))) {
		t.Fatal("unexpected stop should be suppressed after oom")
	}
	if manager.Allow(alert("oom", now.Add(5*time.Minute))) {
		t.Fatal("oom should be blocked by cooldown")
	}
	if !manager.Allow(alert("oom", now.Add(11*time.Minute))) {
		t.Fatal("oom should be allowed after cooldown")
	}
}

func alert(rule string, at time.Time) notifiers.Alert {
	return notifiers.Alert{
		RuleName:         rule,
		ContainerID:      "container-123",
		ContainerIDShort: "container-12",
		OccurredAt:       at,
	}
}
