package rules_test

import (
	"testing"
	"time"

	"docknudge/internal/config"
	"docknudge/internal/events"
	"docknudge/internal/rules"
)

func TestDieSuppressedAfterExpectedKillSignal(t *testing.T) {
	engine := rules.New(testConfig(), "host-a")
	base := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)

	if got := engine.Process(event(base, "kill", withSignal(15))); len(got) != 0 {
		t.Fatalf("kill should not alert: %+v", got)
	}
	if got := engine.Process(event(base.Add(5*time.Second), "die", withExitCode(1))); len(got) != 0 {
		t.Fatalf("die should be suppressed after kill: %+v", got)
	}
}

func TestUnhealthyRequiresHealthyTransitionToRearm(t *testing.T) {
	engine := rules.New(testConfig(), "host-a")
	base := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)

	if got := engine.Process(event(base, "health_status", withHealth("unhealthy"))); len(got) != 1 {
		t.Fatalf("first unhealthy should alert, got %d", len(got))
	}
	if got := engine.Process(event(base.Add(time.Second), "health_status", withHealth("unhealthy"))); len(got) != 0 {
		t.Fatalf("repeat unhealthy should not alert, got %d", len(got))
	}
	if got := engine.Process(event(base.Add(2*time.Second), "health_status", withHealth("healthy"))); len(got) != 0 {
		t.Fatalf("healthy transition should not alert, got %d", len(got))
	}
	if got := engine.Process(event(base.Add(3*time.Second), "health_status", withHealth("unhealthy"))); len(got) != 1 {
		t.Fatalf("unhealthy should rearm after healthy transition, got %d", len(got))
	}
}

func TestRestartBurstThresholdCrossingAndRearm(t *testing.T) {
	engine := rules.New(testConfig(), "host-a")
	base := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)

	if got := engine.Process(event(base, "restart")); len(got) != 0 {
		t.Fatalf("first restart should not alert")
	}
	if got := engine.Process(event(base.Add(time.Minute), "restart")); len(got) != 0 {
		t.Fatalf("second restart should not alert")
	}
	if got := engine.Process(event(base.Add(2*time.Minute), "restart")); len(got) != 1 {
		t.Fatalf("threshold crossing should alert once, got %d", len(got))
	}
	if got := engine.Process(event(base.Add(3*time.Minute), "restart")); len(got) != 0 {
		t.Fatalf("restart burst should not re-fire while still above threshold")
	}
	if got := engine.Process(event(base.Add(20*time.Minute), "restart")); len(got) != 0 {
		t.Fatalf("restart after window rollover should reset state")
	}
	if got := engine.Process(event(base.Add(21*time.Minute), "restart")); len(got) != 0 {
		t.Fatalf("second restart after reset should not alert")
	}
	if got := engine.Process(event(base.Add(22*time.Minute), "restart")); len(got) != 1 {
		t.Fatalf("third restart after reset should alert again, got %d", len(got))
	}
}

func TestUnexpectedStopUsesEvidenceAndInactivityEviction(t *testing.T) {
	engine := rules.New(testConfig(), "host-a")
	base := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)

	engine.Process(event(base, "kill", withSignal(15)))
	if got := engine.Process(event(base.Add(5*time.Second), "stop")); len(got) != 0 {
		t.Fatalf("stop after expected kill should be suppressed")
	}
	if got := engine.Process(event(base.Add(2*time.Hour), "stop")); len(got) != 1 {
		t.Fatalf("stop after inactivity eviction should alert, got %d", len(got))
	}
}

func testConfig() config.Config {
	cfg := config.Default()
	cfg.Version = 1
	cfg.Channels = map[string]config.Channel{
		"slack_ops": {Type: "slack", WebhookURL: "https://hooks.slack.test/services/abc"},
	}
	cfg.Routes.Default.SendTo = []string{"slack_ops"}
	return cfg
}

type eventOption func(*events.Event)

func event(ts time.Time, action string, opts ...eventOption) events.Event {
	ev := events.Event{
		Timestamp:     ts,
		Action:        action,
		ContainerID:   "container-1234567890ab",
		ContainerName: "api",
	}
	for _, opt := range opts {
		opt(&ev)
	}
	return ev
}

func withSignal(signal int) eventOption {
	return func(ev *events.Event) {
		ev.Signal = &signal
	}
}

func withExitCode(code int) eventOption {
	return func(ev *events.Event) {
		ev.ExitCode = &code
	}
}

func withHealth(status string) eventOption {
	return func(ev *events.Event) {
		ev.HealthStatus = status
	}
}
