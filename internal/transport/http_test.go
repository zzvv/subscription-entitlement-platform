package transport

import (
	"bytes"
	"context"
	"example.com/subscription-entitlement-platform/internal/application"
	"example.com/subscription-entitlement-platform/internal/repository"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProcessRejectsTrailingJSONCommand(t *testing.T) {
	handler := New(application.NewService(repository.NewStore()))
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/subscriptions/commands",
		bytes.NewBufferString(`{"id":"sub-a","tenant":"tenant-a","scope":"standard","action":"activate"}{"id":"sub-b","tenant":"tenant-b","scope":"standard","action":"activate"}`),
	)
	recorder := httptest.NewRecorder()

	handler.Process(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected trailing JSON command to be rejected with %d, got %d: %s", http.StatusBadRequest, recorder.Code, recorder.Body.String())
	}
}

func TestProcessMapsCanceledRequestToClientClosedRequest(t *testing.T) {
	handler := New(application.NewService(repository.NewStore()))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/subscriptions/commands",
		bytes.NewBufferString(`{"id":"sub-a","tenant":"tenant-a","scope":"standard","action":"activate"}`),
	).WithContext(ctx)
	recorder := httptest.NewRecorder()

	handler.Process(recorder, request)

	if recorder.Code != 499 {
		t.Fatalf("canceled request must be reported as client closed request (499), got %d: %s", recorder.Code, recorder.Body.String())
	}
}
