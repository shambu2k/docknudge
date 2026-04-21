package notifiers_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"docknudge/internal/notifiers"
)

func TestWebhookSenderPostsTextPayload(t *testing.T) {
	var called atomic.Int32
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		called.Add(1)
		defer r.Body.Close()
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload["text"] != "[critical] host / api - container OOM killed" {
			t.Fatalf("unexpected payload: %#v", payload)
		}
		return response(http.StatusOK, ""), nil
	})

	sender := notifiers.NewWebhookSender("slack_ops", "slack", "https://example.invalid/slack", client)
	err := sender.Send(context.Background(), notifiers.Alert{
		Summary: "[critical] host / api - container OOM killed",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if called.Load() != 1 {
		t.Fatalf("expected 1 request, got %d", called.Load())
	}
}

func TestWebhookSenderRetriesTransientStatus(t *testing.T) {
	var called atomic.Int32
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		count := called.Add(1)
		if count == 1 {
			return response(http.StatusInternalServerError, "retry"), nil
		}
		return response(http.StatusOK, ""), nil
	})

	sender := notifiers.NewWebhookSender("gchat_ops", "gchat", "https://example.invalid/gchat", client)
	if err := sender.Send(context.Background(), notifiers.Alert{Summary: "test"}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if called.Load() != 2 {
		t.Fatalf("expected 2 attempts, got %d", called.Load())
	}
}

func TestWebhookSenderDoesNotRetryPermanentStatus(t *testing.T) {
	var called atomic.Int32
	client := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		called.Add(1)
		return response(http.StatusBadRequest, "bad request"), nil
	})

	sender := notifiers.NewWebhookSender("slack_ops", "slack", "https://example.invalid/slack", client)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := sender.Send(ctx, notifiers.Alert{Summary: "test"})
	if err == nil {
		t.Fatal("expected Send() to fail on permanent status")
	}
	if called.Load() != 1 {
		t.Fatalf("expected 1 attempt, got %d", called.Load())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func response(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}
