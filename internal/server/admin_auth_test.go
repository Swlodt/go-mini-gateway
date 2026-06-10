package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdminAuthMiddleware(t *testing.T) {
	tests := []struct {
		name       string
		adminToken string
		header     string
		wantStatus int
		wantNext   bool
	}{
		{
			name:       "missing configured token",
			adminToken: "",
			header:     "",
			wantStatus: http.StatusServiceUnavailable,
			wantNext:   false,
		},
		{
			name:       "missing request token",
			adminToken: "secret",
			header:     "",
			wantStatus: http.StatusUnauthorized,
			wantNext:   false,
		},
		{
			name:       "wrong request token",
			adminToken: "secret",
			header:     "wrong",
			wantStatus: http.StatusUnauthorized,
			wantNext:   false,
		},
		{
			name:       "correct request token",
			adminToken: "secret",
			header:     "secret",
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{
				adminToken: tt.adminToken,
			}

			nextCalled := false

			handler := s.adminAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			}))

			req := httptest.NewRequest(http.MethodGet, "/admin/routes", nil)
			if tt.header != "" {
				req.Header.Set(adminTokenHeader, tt.header)
			}

			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status got %d, want %d", rec.Code, tt.wantStatus)
			}

			if nextCalled != tt.wantNext {
				t.Fatalf("nextCalled got %v, want %v", nextCalled, tt.wantNext)
			}
		})
	}
}

func TestConstantTimeTokenEqual(t *testing.T) {
	if !constantTimeTokenEqual("secret", "secret") {
		t.Fatalf("same token should be equal")
	}

	if constantTimeTokenEqual("wrong", "secret") {
		t.Fatalf("different token should not be equal")
	}

	if constantTimeTokenEqual("", "secret") {
		t.Fatalf("empty actual token should not equal expected token")
	}
}
