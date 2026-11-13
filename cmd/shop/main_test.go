package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestNewApplicationServesHealthCheck(t *testing.T) {
	server, store, err := newApplication(filepath.Join(t.TempDir(), "shop.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	result := httptest.NewRecorder()
	server.Handler().ServeHTTP(result, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if result.Code != http.StatusOK || result.Body.String() != "ok" {
		t.Fatalf("health check = %d, %q", result.Code, result.Body.String())
	}
}

func TestNewApplicationRejectsInvalidDatabasePath(t *testing.T) {
	if _, _, err := newApplication(t.TempDir(), "test-secret"); err == nil {
		t.Fatal("newApplication should return an error for a directory database path")
	}
}

func TestRunConfiguresAndStartsApplication(t *testing.T) {
	stop := errors.New("stop test server")
	database := filepath.Join(t.TempDir(), "run.db")
	err := run([]string{"-addr", ":18080", "-db", database}, func(string) string {
		return "test-secret"
	}, func(address string, handler http.Handler) error {
		if address != ":18080" {
			t.Fatalf("address = %q", address)
		}
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		if result.Code != http.StatusOK {
			t.Fatalf("health = %d", result.Code)
		}
		return stop
	})
	if !errors.Is(err, stop) {
		t.Fatalf("run error = %v", err)
	}
}

func TestRunRejectsInvalidFlag(t *testing.T) {
	if err := run([]string{"-unknown"}, func(string) string { return "" }, func(string, http.Handler) error { return nil }); err == nil {
		t.Fatal("run should reject an unknown flag")
	}
}
