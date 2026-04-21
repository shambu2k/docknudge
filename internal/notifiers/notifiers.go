package notifiers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"docknudge/internal/config"
)

type Alert struct {
	RuleName         string
	Severity         string
	Host             string
	ContainerID      string
	ContainerName    string
	ContainerIDShort string
	Image            string
	EventType        string
	OccurredAt       time.Time
	Summary          string
}

type Sender interface {
	Send(context.Context, Alert) error
}

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type Dispatcher struct {
	channels map[string]Sender
	defaults []string
	logger   *slog.Logger
}

func NewDispatcher(cfg config.Config, logger *slog.Logger, client HTTPDoer) (*Dispatcher, error) {
	if logger == nil {
		logger = slog.Default()
	}
	channels := make(map[string]Sender, len(cfg.Channels))
	for name, channel := range cfg.Channels {
		channels[name] = NewWebhookSender(name, channel.Type, channel.WebhookURL, client)
	}
	return &Dispatcher{
		channels: channels,
		defaults: slices.Clone(cfg.Routes.Default.SendTo),
		logger:   logger,
	}, nil
}

func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

func (d *Dispatcher) ChannelNames() []string {
	names := make([]string, 0, len(d.channels))
	for name := range d.channels {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func (d *Dispatcher) SendChannel(ctx context.Context, channelName string, alert Alert) error {
	sender, ok := d.channels[channelName]
	if !ok {
		return fmt.Errorf("unknown channel %q", channelName)
	}
	if err := sender.Send(ctx, alert); err != nil {
		d.logger.Error("webhook delivery failed", "channel", channelName, "error", err)
		return err
	}
	return nil
}

func (d *Dispatcher) DispatchDefault(ctx context.Context, alert Alert) error {
	var errs []error
	for _, channelName := range d.defaults {
		if err := d.SendChannel(ctx, channelName, alert); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

type WebhookSender struct {
	channelName string
	channelType string
	webhookURL  string
	client      HTTPDoer
}

func NewWebhookSender(channelName, channelType, webhookURL string, client HTTPDoer) *WebhookSender {
	return &WebhookSender{
		channelName: channelName,
		channelType: channelType,
		webhookURL:  webhookURL,
		client:      client,
	}
}

func (w *WebhookSender) Send(ctx context.Context, alert Alert) error {
	payload := map[string]string{"text": alert.Summary}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	var lastErr error
	backoff := 200 * time.Millisecond
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.webhookURL, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("build webhook request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := w.client.Do(req)
		if err != nil {
			lastErr = err
			if isTransientError(err) && attempt < 2 {
				if err := sleepContext(ctx, backoff); err != nil {
					return err
				}
				backoff *= 2
				continue
			}
			return fmt.Errorf("%s channel %q request failed: %w", w.channelType, w.channelName, err)
		}

		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}

		lastErr = fmt.Errorf("%s channel %q returned status %d: %s", w.channelType, w.channelName, resp.StatusCode, strings.TrimSpace(string(respBody)))
		if shouldRetryStatus(resp.StatusCode) && attempt < 2 {
			if err := sleepContext(ctx, backoff); err != nil {
				return err
			}
			backoff *= 2
			continue
		}
		return lastErr
	}

	return lastErr
}

func isTransientError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return errors.Is(err, io.ErrUnexpectedEOF)
}

func shouldRetryStatus(statusCode int) bool {
	if statusCode == http.StatusTooManyRequests {
		return true
	}
	return statusCode >= 500 && statusCode <= 599
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
