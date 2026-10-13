package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	salt, hash := hashPassword("CorrectHorseBattery9!")
	if !verifyPassword("CorrectHorseBattery9!", salt, hash) {
		t.Fatal("valid password rejected")
	}
	if verifyPassword("incorrect-password", salt, hash) {
		t.Fatal("invalid password accepted")
	}
}
func TestSessionSignatureAndExpiry(t *testing.T) {
	s := &Server{secret: []byte("test-secret-that-is-long-enough")}
	token := s.signSession("user-123")
	id, err := s.verifySession(token)
	if err != nil || id != "user-123" {
		t.Fatalf("valid token failed: id=%q err=%v", id, err)
	}
	if _, err = s.verifySession(token + "x"); err == nil {
		t.Fatal("tampered token accepted")
	}
}
func TestHandleValidation(t *testing.T) {
	for _, value := range []string{"abc", "Averna_12", "123"} {
		if !validHandle(value) {
			t.Errorf("expected valid handle %q", value)
		}
	}
	for _, value := range []string{"ab", "with space", "ééé", "bad!"} {
		if validHandle(value) {
			t.Errorf("expected invalid handle %q", value)
		}
	}
}
func TestCookieSessionFlags(t *testing.T) {
	s := &Server{secret: []byte("test-secret")}
	w := httptest.NewRecorder()
	s.setSession(w, "u1", httptest.NewRequest("GET", "http://example.test/", nil))
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("unexpected session cookie: %#v", cookies)
	}
}

func TestEloDeltaRewardsWinAndFavorsUnderdogs(t *testing.T) {
	if got := eloDelta(1200, 1200); got != 16 {
		t.Fatalf("equal ratings: got %d, want 16", got)
	}
	if got := eloDelta(1000, 1600); got <= 16 {
		t.Fatalf("underdog win should gain more than 16, got %d", got)
	}
	if got := eloDelta(2000, 1000); got >= 16 {
		t.Fatalf("favorite win should gain less than 16, got %d", got)
	}
}
