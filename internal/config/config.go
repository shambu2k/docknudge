package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	DefaultPath             = "docknudge.yml"
	defaultDockerHost       = "unix:///var/run/docker.sock"
	defaultCooldown         = 10 * time.Minute
	defaultLogLevel         = "info"
	defaultRestartThreshold = 3
	defaultRestartWindow    = 10 * time.Minute
	defaultStopLookback     = 30 * time.Second
)

var envPattern = regexp.MustCompile(`\$\{([A-Z0-9_]+)\}`)

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf("duration must be a scalar")
	}
	value := strings.TrimSpace(node.Value)
	if value == "" {
		d.Duration = 0
		return nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", value, err)
	}
	d.Duration = parsed
	return nil
}

func (d Duration) MarshalYAML() (any, error) {
	return d.String(), nil
}

func (d Duration) String() string {
	if d.Duration == 0 {
		return "0s"
	}
	return d.Duration.String()
}

type Config struct {
	Version    int    `yaml:"version"`
	DockerHost string `yaml:"docker_host"`
	// StartupBackfill is deprecated and ignored at runtime. It remains
	// parseable so older configs do not break immediately.
	StartupBackfill Duration           `yaml:"startup_backfill"`
	Cooldown        Duration           `yaml:"cooldown"`
	LogLevel        string             `yaml:"log_level"`
	Channels        map[string]Channel `yaml:"channels"`
	Routes          Routes             `yaml:"routes"`
	Rules           Rules              `yaml:"rules"`
}

type Channel struct {
	Type       string `yaml:"type"`
	WebhookURL string `yaml:"webhook_url"`
}

type Routes struct {
	Default Route `yaml:"default"`
}

type Route struct {
	SendTo []string `yaml:"send_to"`
}

type Rules struct {
	OOM            ToggleRule         `yaml:"oom"`
	Die            DieRule            `yaml:"die"`
	Unhealthy      ToggleRule         `yaml:"unhealthy"`
	RestartBurst   RestartBurstRule   `yaml:"restart_burst"`
	UnexpectedStop UnexpectedStopRule `yaml:"unexpected_stop"`
}

type ToggleRule struct {
	Enabled bool `yaml:"enabled"`
}

type DieRule struct {
	Enabled         bool  `yaml:"enabled"`
	IgnoreExitCodes []int `yaml:"ignore_exit_codes"`
}

type RestartBurstRule struct {
	Enabled   bool     `yaml:"enabled"`
	Threshold int      `yaml:"threshold"`
	Window    Duration `yaml:"window"`
}

type UnexpectedStopRule struct {
	Enabled  bool     `yaml:"enabled"`
	Lookback Duration `yaml:"lookback"`
}

func Default() Config {
	return Config{
		DockerHost: defaultDockerHost,
		Cooldown:   Duration{Duration: defaultCooldown},
		LogLevel:   defaultLogLevel,
		Channels:   map[string]Channel{},
		Rules: Rules{
			OOM:       ToggleRule{Enabled: true},
			Die:       DieRule{Enabled: true, IgnoreExitCodes: []int{0, 143}},
			Unhealthy: ToggleRule{Enabled: true},
			RestartBurst: RestartBurstRule{
				Enabled:   true,
				Threshold: defaultRestartThreshold,
				Window:    Duration{Duration: defaultRestartWindow},
			},
			UnexpectedStop: UnexpectedStopRule{
				Enabled:  true,
				Lookback: Duration{Duration: defaultStopLookback},
			},
		},
	}
}

func ResolvePath(path string) string {
	if strings.TrimSpace(path) == "" {
		return DefaultPath
	}
	return path
}

func Load(path string) (Config, error) {
	resolved := ResolvePath(path)
	raw, err := os.ReadFile(resolved)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", resolved, err)
	}

	expanded, missing := expandEnv(raw)
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	var root yaml.Node
	if err := yaml.Unmarshal(expanded, &root); err != nil {
		return Config{}, fmt.Errorf("parse yaml: %w", err)
	}
	if err := checkDuplicateKeys(&root); err != nil {
		return Config{}, err
	}

	cfg := Default()
	dec := yaml.NewDecoder(bytes.NewReader(expanded))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	var problems []string

	if c.Version != 1 {
		problems = append(problems, fmt.Sprintf("unsupported config version %d", c.Version))
	}
	if strings.TrimSpace(c.DockerHost) == "" {
		problems = append(problems, "docker_host must be set")
	}
	if c.Cooldown.Duration <= 0 {
		problems = append(problems, "cooldown must be greater than 0")
	}

	level := strings.ToLower(strings.TrimSpace(c.LogLevel))
	if !slices.Contains([]string{"debug", "info", "warn", "error"}, level) {
		problems = append(problems, fmt.Sprintf("unsupported log_level %q", c.LogLevel))
	}

	if len(c.Channels) == 0 {
		problems = append(problems, "at least one channel must be configured")
	}
	for name, channel := range c.Channels {
		switch channel.Type {
		case "slack", "gchat":
		default:
			problems = append(problems, fmt.Sprintf("channel %q has unsupported type %q", name, channel.Type))
		}
		if strings.TrimSpace(channel.WebhookURL) == "" {
			problems = append(problems, fmt.Sprintf("channel %q webhook_url must be set", name))
		}
	}

	if len(c.Routes.Default.SendTo) == 0 {
		problems = append(problems, "routes.default.send_to must include at least one channel")
	}
	for _, channelName := range c.Routes.Default.SendTo {
		if _, ok := c.Channels[channelName]; !ok {
			problems = append(problems, fmt.Sprintf("routes.default references unknown channel %q", channelName))
		}
	}

	if c.RestartBurst().Threshold < 1 {
		problems = append(problems, "rules.restart_burst.threshold must be at least 1")
	}
	if c.RestartBurst().Window.Duration <= 0 {
		problems = append(problems, "rules.restart_burst.window must be greater than 0")
	}
	if c.UnexpectedStopRule().Lookback.Duration <= 0 {
		problems = append(problems, "rules.unexpected_stop.lookback must be greater than 0")
	}

	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func (c Config) RestartBurst() RestartBurstRule {
	return c.Rules.RestartBurst
}

func (c Config) UnexpectedStopRule() UnexpectedStopRule {
	return c.Rules.UnexpectedStop
}

func expandEnv(raw []byte) ([]byte, []string) {
	missingSet := map[string]struct{}{}
	expanded := envPattern.ReplaceAllFunc(raw, func(match []byte) []byte {
		submatches := envPattern.FindSubmatch(match)
		name := string(submatches[1])
		value, ok := os.LookupEnv(name)
		if !ok {
			missingSet[name] = struct{}{}
			return match
		}
		return []byte(value)
	})

	missing := make([]string, 0, len(missingSet))
	for key := range missingSet {
		missing = append(missing, key)
	}
	slices.Sort(missing)
	return expanded, missing
}

func checkDuplicateKeys(node *yaml.Node) error {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if err := checkDuplicateKeys(child); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		seen := map[string]struct{}{}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]
			if _, ok := seen[key.Value]; ok {
				return fmt.Errorf("duplicate key %q on line %d", key.Value, key.Line)
			}
			seen[key.Value] = struct{}{}
			if err := checkDuplicateKeys(value); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if err := checkDuplicateKeys(child); err != nil {
				return err
			}
		}
	}
	return nil
}
