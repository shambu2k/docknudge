package runtime

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"docknudge/internal/config"
	"docknudge/internal/events"
	"docknudge/internal/logging"
	"docknudge/internal/notifiers"
)

func TestServiceProcessesLiveEventsAndReconnects(t *testing.T) {
	logger, err := logging.New("debug")
	if err != nil {
		t.Fatalf("logging.New(): %v", err)
	}

	var deliveries atomic.Int32
	client := runtimeDoer(func(r *http.Request) (*http.Response, error) {
		deliveries.Add(1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
		}, nil
	})

	cfg := config.Default()
	cfg.Version = 1
	cfg.Channels = map[string]config.Channel{
		"slack_ops": {Type: "slack", WebhookURL: "https://example.invalid/slack"},
	}
	cfg.Routes.Default.SendTo = []string{"slack_ops"}

	dispatcher, err := notifiers.NewDispatcher(cfg, logger, client)
	if err != nil {
		t.Fatalf("NewDispatcher(): %v", err)
	}

	base := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	source := &fakeSource{
		streams: []fakeStream{
			{
				events: []events.Event{testEvent(base, "oom")},
				err:    errors.New("disconnect"),
			},
			{
				events: []events.Event{testEvent(base.Add(10*time.Second), "start")},
				after:  cancel,
			},
		},
	}

	service := NewService(cfg, "docknudge.yml", source, dispatcher, logger, "host-a")
	if err := service.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if deliveries.Load() != 1 {
		t.Fatalf("expected exactly one alert delivery, got %d", deliveries.Load())
	}
	if source.streamCalls() != 2 {
		t.Fatalf("expected 2 stream attempts, got %d", source.streamCalls())
	}
}

type fakeSource struct {
	streams []fakeStream

	mu         sync.Mutex
	streamCall int
}

type fakeStream struct {
	events []events.Event
	err    error
	after  func()
}

func (f *fakeSource) Ping(context.Context) error {
	return nil
}

func (f *fakeSource) Stream(ctx context.Context) (<-chan events.Event, <-chan error, error) {
	f.mu.Lock()
	idx := f.streamCall
	f.streamCall++
	f.mu.Unlock()

	if idx >= len(f.streams) {
		return nil, nil, errors.New("unexpected stream request")
	}
	script := f.streams[idx]
	eventCh := make(chan events.Event)
	errCh := make(chan error, 1)

	go func() {
		defer close(eventCh)
		defer close(errCh)
		for _, event := range script.events {
			select {
			case <-ctx.Done():
				return
			case eventCh <- event:
			}
		}
		if script.err != nil {
			errCh <- script.err
			return
		}
		if script.after != nil {
			script.after()
		}
	}()

	return eventCh, errCh, nil
}

func (f *fakeSource) Close() error {
	return nil
}

func (f *fakeSource) streamCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.streamCall
}

func testEvent(ts time.Time, action string) events.Event {
	return events.Event{
		Timestamp:     ts,
		Action:        action,
		ContainerID:   "container-1234567890ab",
		ContainerName: "api",
	}
}

type runtimeDoer func(*http.Request) (*http.Response, error)

func (f runtimeDoer) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}
