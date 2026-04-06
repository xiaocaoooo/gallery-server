package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/xiaocaoooo/gallery-server/internal/config"
)

func TestExtractToken(t *testing.T) {
	t.Run("authorization bearer wins", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?token=query", nil)
		req.Header.Set("Authorization", "Bearer auth-token")
		req.Header.Set("X-Read-Token", "header-token")
		if token := ExtractToken(req, "X-Read-Token"); token != "auth-token" {
			t.Fatalf("expected bearer token, got %q", token)
		}
	})

	t.Run("preferred header before generic header and query", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?token=query-token", nil)
		req.Header.Set("X-API-Token", "generic-token")
		req.Header.Set("X-Read-Token", "preferred-token")
		if token := ExtractToken(req, "X-Read-Token"); token != "preferred-token" {
			t.Fatalf("expected preferred header token, got %q", token)
		}
	})

	t.Run("query fallback", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/?token=query-token", nil)
		if token := ExtractToken(req, "X-Read-Token"); token != "query-token" {
			t.Fatalf("expected query token, got %q", token)
		}
	})
}

func TestAllowsReadAndWrite(t *testing.T) {
	cfg := config.AuthConfig{ReadToken: "read-secret", WriteToken: "write-secret"}

	if !AllowsRead("read-secret", cfg) {
		t.Fatal("expected read token to authorize reads")
	}
	if !AllowsRead("write-secret", cfg) {
		t.Fatal("expected write token to authorize reads")
	}
	if AllowsRead("wrong", cfg) {
		t.Fatal("expected wrong token to fail read auth")
	}
	if !AllowsWrite("write-secret", cfg) {
		t.Fatal("expected write token to authorize writes")
	}
	if AllowsWrite("read-secret", cfg) {
		t.Fatal("expected read token to fail write auth")
	}

	openRead := config.AuthConfig{WriteToken: "write-secret"}
	if !AllowsRead("", openRead) {
		t.Fatal("expected empty read token config to allow reads")
	}

	openWrite := config.AuthConfig{ReadToken: "read-secret"}
	if !AllowsWrite("", openWrite) {
		t.Fatal("expected empty write token config to allow writes")
	}
}
