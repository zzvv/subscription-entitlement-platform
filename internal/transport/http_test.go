package transport

import (
	"bytes"
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
