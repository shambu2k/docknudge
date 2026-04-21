package rules

import (
	"fmt"
	"slices"
	"time"

	"docknudge/internal/config"
	"docknudge/internal/events"
	"docknudge/internal/notifiers"
)

const inactivityTTL = time.Hour

type Engine struct {
	cfg        config.Config
	host       string
	containers map[string]*containerState
}

type containerState struct {
	lastSeen              time.Time
	restarts              []time.Time
	restartBurstTriggered bool
	unhealthy             bool
	evidence              []evidence
}

type evidence struct {
	at       time.Time
	action   string
	exitCode *int
	signal   *int
}

func New(cfg config.Config, host string) *Engine {
	return &Engine{
		cfg:        cfg,
		host:       host,
		containers: map[string]*containerState{},
	}
}

func (e *Engine) Process(event events.Event) []notifiers.Alert {
	e.cleanup(event.Timestamp)
	state := e.stateFor(event.ContainerID)
	state.lastSeen = event.Timestamp

	alerts := make([]notifiers.Alert, 0, 1)

	switch event.Action {
	case "oom":
		if e.cfg.Rules.OOM.Enabled {
			alerts = append(alerts, e.newAlert("oom", "critical", event, "container OOM killed"))
		}
	case "die":
		if e.cfg.Rules.Die.Enabled && !e.shouldIgnoreDie(state, event) {
			summary := "container exited"
			if event.ExitCode != nil {
				summary = fmt.Sprintf("exited with code %d", *event.ExitCode)
			}
			alerts = append(alerts, e.newAlert("die", "critical", event, summary))
		}
	case "health_status":
		if event.HealthStatus == "healthy" {
			state.unhealthy = false
		}
		if e.cfg.Rules.Unhealthy.Enabled && event.HealthStatus == "unhealthy" && !state.unhealthy {
			state.unhealthy = true
			alerts = append(alerts, e.newAlert("unhealthy", "warning", event, "health check became unhealthy"))
		}
	case "restart":
		if e.cfg.Rules.RestartBurst.Enabled {
			state.restarts = append(state.restarts, event.Timestamp)
			state.restarts = trimTimes(state.restarts, event.Timestamp.Add(-e.cfg.Rules.RestartBurst.Window.Duration))
			if len(state.restarts) >= e.cfg.Rules.RestartBurst.Threshold && !state.restartBurstTriggered {
				state.restartBurstTriggered = true
				summary := fmt.Sprintf("restarted %d times in %s", len(state.restarts), e.cfg.Rules.RestartBurst.Window.String())
				alerts = append(alerts, e.newAlert("restart_burst", "critical", event, summary))
			}
			if len(state.restarts) < e.cfg.Rules.RestartBurst.Threshold {
				state.restartBurstTriggered = false
			}
		}
	case "stop":
		if e.cfg.Rules.UnexpectedStop.Enabled && !e.isExpectedStop(state, event.Timestamp) {
			alerts = append(alerts, e.newAlert("unexpected_stop", "critical", event, "container stopped unexpectedly"))
		}
	}

	e.recordEvidence(state, event)
	return alerts
}

func (e *Engine) cleanup(now time.Time) {
	for containerID, state := range e.containers {
		if now.Sub(state.lastSeen) > inactivityTTL {
			delete(e.containers, containerID)
			continue
		}
		state.restarts = trimTimes(state.restarts, now.Add(-e.cfg.Rules.RestartBurst.Window.Duration))
		if len(state.restarts) < e.cfg.Rules.RestartBurst.Threshold {
			state.restartBurstTriggered = false
		}
		state.evidence = trimEvidence(state.evidence, now.Add(-e.cfg.Rules.UnexpectedStop.Lookback.Duration))
	}
}

func (e *Engine) stateFor(containerID string) *containerState {
	state := e.containers[containerID]
	if state == nil {
		state = &containerState{}
		e.containers[containerID] = state
	}
	return state
}

func (e *Engine) shouldIgnoreDie(state *containerState, event events.Event) bool {
	if event.ExitCode != nil && slices.Contains(e.cfg.Rules.Die.IgnoreExitCodes, *event.ExitCode) {
		return true
	}
	lookbackStart := event.Timestamp.Add(-e.cfg.Rules.UnexpectedStop.Lookback.Duration)
	for _, item := range state.evidence {
		if item.at.Before(lookbackStart) {
			continue
		}
		if item.action == "kill" && item.signal != nil && (*item.signal == 9 || *item.signal == 15) {
			return true
		}
	}
	return false
}

func (e *Engine) isExpectedStop(state *containerState, now time.Time) bool {
	lookbackStart := now.Add(-e.cfg.Rules.UnexpectedStop.Lookback.Duration)
	for _, item := range state.evidence {
		if item.at.Before(lookbackStart) {
			continue
		}
		if item.action == "kill" && item.signal != nil && (*item.signal == 9 || *item.signal == 15) {
			return true
		}
		if item.action == "die" && item.exitCode != nil && (*item.exitCode == 0 || *item.exitCode == 137 || *item.exitCode == 143) {
			return true
		}
	}
	return false
}

func (e *Engine) recordEvidence(state *containerState, event events.Event) {
	switch event.Action {
	case "kill", "die":
		state.evidence = append(state.evidence, evidence{
			at:       event.Timestamp,
			action:   event.Action,
			exitCode: event.ExitCode,
			signal:   event.Signal,
		})
		state.evidence = trimEvidence(state.evidence, event.Timestamp.Add(-e.cfg.Rules.UnexpectedStop.Lookback.Duration))
	}
}

func (e *Engine) newAlert(ruleName, severity string, event events.Event, message string) notifiers.Alert {
	containerName := event.ContainerName
	if containerName == "" {
		containerName = event.ShortContainerID()
	}
	return notifiers.Alert{
		RuleName:         ruleName,
		Severity:         severity,
		Host:             e.host,
		ContainerID:      event.ContainerID,
		ContainerName:    containerName,
		ContainerIDShort: event.ShortContainerID(),
		Image:            event.Image,
		EventType:        event.Action,
		OccurredAt:       event.Timestamp,
		Summary:          fmt.Sprintf("[%s] %s / %s - %s", severity, e.host, containerName, message),
	}
}

func trimTimes(times []time.Time, min time.Time) []time.Time {
	idx := 0
	for idx < len(times) && times[idx].Before(min) {
		idx++
	}
	if idx == 0 {
		return times
	}
	return append([]time.Time(nil), times[idx:]...)
}

func trimEvidence(items []evidence, min time.Time) []evidence {
	idx := 0
	for idx < len(items) && items[idx].at.Before(min) {
		idx++
	}
	if idx == 0 {
		return items
	}
	return append([]evidence(nil), items[idx:]...)
}
