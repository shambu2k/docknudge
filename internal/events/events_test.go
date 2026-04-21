package events_test

import (
	"testing"
	"time"

	mobyevents "github.com/docker/docker/api/types/events"

	"docknudge/internal/events"
)

func TestNormalizeHealthStatusAndAttributes(t *testing.T) {
	msg := mobyevents.Message{
		Action:   "health_status: unhealthy",
		ID:       "fallback-id",
		TimeNano: time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC).UnixNano(),
		Actor: mobyevents.Actor{
			ID: "container-id-1234567890ab",
			Attributes: map[string]string{
				"name":     "api",
				"image":    "my-image:latest",
				"exitCode": "137",
				"signal":   "9",
			},
		},
	}

	event, err := events.Normalize(msg)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	if event.Action != "health_status" || event.HealthStatus != "unhealthy" {
		t.Fatalf("unexpected health normalization: %+v", event)
	}
	if event.ContainerID != "container-id-1234567890ab" {
		t.Fatalf("ContainerID = %q", event.ContainerID)
	}
	if event.ContainerName != "api" || event.Image != "my-image:latest" {
		t.Fatalf("unexpected metadata: %+v", event)
	}
	if event.ExitCode == nil || *event.ExitCode != 137 {
		t.Fatalf("ExitCode = %v", event.ExitCode)
	}
	if event.Signal == nil || *event.Signal != 9 {
		t.Fatalf("Signal = %v", event.Signal)
	}
}
