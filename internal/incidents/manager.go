package incidents

import (
	"fmt"
	"time"

	"docknudge/internal/notifiers"
)

const suppressionWindow = time.Minute

type Manager struct {
	cooldown     time.Duration
	lastSent     map[string]time.Time
	suppressions map[string]map[string]time.Time
}

func New(cooldown time.Duration) *Manager {
	return &Manager{
		cooldown:     cooldown,
		lastSent:     map[string]time.Time{},
		suppressions: map[string]map[string]time.Time{},
	}
}

func (m *Manager) Allow(alert notifiers.Alert) bool {
	key := incidentKey(alert.RuleName, alert.ContainerID)
	if last, ok := m.lastSent[key]; ok && alert.OccurredAt.Sub(last) < m.cooldown {
		return false
	}
	if rules := m.suppressions[alert.ContainerID]; rules != nil {
		if until, ok := rules[alert.RuleName]; ok && alert.OccurredAt.Before(until) {
			return false
		}
	}
	return true
}

func (m *Manager) Record(alert notifiers.Alert) {
	key := incidentKey(alert.RuleName, alert.ContainerID)
	m.lastSent[key] = alert.OccurredAt

	switch alert.RuleName {
	case "oom":
		m.suppress(alert.ContainerID, "die", alert.OccurredAt.Add(suppressionWindow))
		m.suppress(alert.ContainerID, "unexpected_stop", alert.OccurredAt.Add(suppressionWindow))
	case "die":
		m.suppress(alert.ContainerID, "unexpected_stop", alert.OccurredAt.Add(suppressionWindow))
	}
}

func (m *Manager) suppress(containerID, rule string, until time.Time) {
	if m.suppressions[containerID] == nil {
		m.suppressions[containerID] = map[string]time.Time{}
	}
	m.suppressions[containerID][rule] = until
}

func incidentKey(ruleName, containerID string) string {
	return fmt.Sprintf("%s:%s", ruleName, containerID)
}
