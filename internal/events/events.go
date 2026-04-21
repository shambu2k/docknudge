package events

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	mobyevents "github.com/docker/docker/api/types/events"
)

type Event struct {
	Timestamp     time.Time
	Action        string
	RawAction     string
	ContainerID   string
	ContainerName string
	Image         string
	ExitCode      *int
	Signal        *int
	HealthStatus  string
}

func (e Event) ShortContainerID() string {
	if len(e.ContainerID) <= 12 {
		return e.ContainerID
	}
	return e.ContainerID[:12]
}

func (e Event) Key() string {
	return fmt.Sprintf("%d:%s:%s", e.Timestamp.UnixNano(), e.Action, e.ContainerID)
}

func Normalize(msg mobyevents.Message) (Event, error) {
	containerID := msg.Actor.ID
	if containerID == "" {
		containerID = msg.ID
	}
	if containerID == "" {
		return Event{}, fmt.Errorf("docker event missing container id")
	}

	action := msg.Action
	healthStatus := ""
	if strings.HasPrefix(action, "health_status:") {
		healthStatus = strings.TrimSpace(strings.TrimPrefix(action, "health_status:"))
		action = "health_status"
	}

	exitCode, err := parseOptionalInt(msg.Actor.Attributes["exitCode"], msg.Actor.Attributes["exitcode"])
	if err != nil {
		return Event{}, fmt.Errorf("parse exit code: %w", err)
	}
	signal, err := parseOptionalInt(msg.Actor.Attributes["signal"])
	if err != nil {
		return Event{}, fmt.Errorf("parse signal: %w", err)
	}

	ts := time.Unix(msg.Time, 0)
	if msg.TimeNano > 0 {
		ts = time.Unix(0, msg.TimeNano)
	}

	return Event{
		Timestamp:     ts.UTC(),
		Action:        action,
		RawAction:     msg.Action,
		ContainerID:   containerID,
		ContainerName: msg.Actor.Attributes["name"],
		Image:         msg.Actor.Attributes["image"],
		ExitCode:      exitCode,
		Signal:        signal,
		HealthStatus:  healthStatus,
	}, nil
}

func parseOptionalInt(values ...string) (*int, error) {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		n, err := strconv.Atoi(value)
		if err != nil {
			return nil, err
		}
		return &n, nil
	}
	return nil, nil
}
