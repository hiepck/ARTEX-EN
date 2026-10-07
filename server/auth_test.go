package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// GetSetting's returns for "key missing" and "read failed" differ only by the
// error: in both cases value is the empty string. Once a password handler treats
// that error as "no password set yet", the init entry is left wide open during a
// database hiccup — authInit would let an unauthenticated request overwrite the
// existing admin password, and authStatus would send the frontend straight to
// /setup to do exactly that.
//
// These two tests close the handler's connection pool to force a read failure and
// assert both paths fail closed.
func TestAuthStatusFailsClosedWhenDataSourceUnavailable(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v)", err)
	}
	defer m.Close()
	// Close the pool so subsequent GetSetting calls return an error instead of sql.ErrNoRows.
	if err := m.pg.Close(); err != nil {
		t.Fatal(err)
	}

	s := &Server{m: m}
	w := httptest.NewRecorder()
	s.authStatus(w, httptest.NewRequest("GET", "/api/auth/status", nil))

	if w.Code != 503 {
		t.Fatalf("status=%d want 503 (treating a read failure as uninitialized would send users to /setup to overwrite the password); body=%s", w.Code, w.Body.String())
	}
	var payload struct {
		Initialized *bool `json:"initialized"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err == nil && payload.Initialized != nil {
		t.Fatalf("must not answer initialized on a read failure, got %v", *payload.Initialized)
	}
}

func TestAuthInitFailsClosedWhenDataSourceUnavailable(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v)", err)
	}
	defer m.Close()
	if err := m.pg.Close(); err != nil {
		t.Fatal(err)
	}

	s := &Server{m: m}
	w := httptest.NewRecorder()
	body := strings.NewReader(`{"password":"correct horse battery"}`)
	s.authInit(w, httptest.NewRequest("POST", "/api/auth/init", body))

	if w.Code != 503 {
		t.Fatalf("status=%d want 503 (letting the request through on a read failure would allow an unauthenticated overwrite of the existing password); body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "token") {
		t.Fatalf("must not issue a token on a read failure: %s", w.Body.String())
	}
}

func TestValidatePassword(t *testing.T) {
	for _, tc := range []struct {
		name, pw string
		wantErr  bool
	}{
		{"empty", "", true},
		{"seven characters", "1234567", true},
		{"eight characters", "12345678", false},
		{"eight CJK characters count by runes, not bytes", "密码密码密码密码", false},
		{"three CJK characters are 9 bytes but only 3 runes", "密码强", true},
		{"72 bytes", strings.Repeat("a", 72), false},
		{"73 bytes exceeds the bcrypt limit", strings.Repeat("a", 73), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := validatePassword(tc.pw); (got != "") != tc.wantErr {
				t.Fatalf("validatePassword(%q)=%q, wantErr=%v", tc.pw, got, tc.wantErr)
			}
		})
	}
}
