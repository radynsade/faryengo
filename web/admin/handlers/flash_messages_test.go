package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/radynsade/faryengo/internal/users"
)

func TestGuestFlashSession(t *testing.T) {
	f := newFixture(t, nil, false)

	if response := f.do(request{path: "/admin/en/sign-in"}); len(response.Result().Cookies()) != 0 || len(f.redis.Keys()) != 0 {
		t.Fatal("reading a guest page created a flash session")
	}

	failed := f.do(request{method: http.MethodPost, path: "/admin/en/sign-in", body: "email=ada@example.com&password=wrong"})
	cookies := failed.Result().Cookies()

	if len(cookies) != 1 || cookies[0].Name != "faryen_flash" || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("flash cookies = %+v", cookies)
	}

	if !strings.Contains(failed.Body.String(), "The email, password, or session is invalid.") {
		t.Fatal("the failure was not shown in its response")
	}

	if again := f.do(request{path: "/admin/en/sign-in", cookies: cookies}); strings.Contains(again.Body.String(), "is invalid") {
		t.Fatal("a consumed message was shown again")
	}

	for _, key := range f.redis.Keys() {
		if !strings.HasPrefix(key, "faryen:web:admin:flashes:guest:") {
			t.Fatalf("guest message stored under %s", key)
		}
	}
}

func TestFlashesBelongToTheDeviceSession(t *testing.T) {
	f := newFixture(t, nil, true)
	cookies := f.signIn(t)
	target := f.addRole(t, "Reader", nil, false)

	f.do(request{method: http.MethodPost, path: "/admin/en/roles/" + roleID(target) + "/delete", body: "confirm=delete", cookies: cookies})

	other := f.signIn(t)

	if body := f.do(request{path: "/admin/en/roles", cookies: other}).Body.String(); strings.Contains(body, "deleted successfully") {
		t.Fatal("another device received the message")
	}

	if body := f.do(request{path: "/admin/en/roles", cookies: cookies}).Body.String(); !strings.Contains(body, "deleted successfully") {
		t.Fatal("the device that made the change did not receive the message")
	}
}

func TestFlashStorageUnavailable(t *testing.T) {
	f := newFixture(t, users.AllPermissions(), false)
	guest := []*http.Cookie{{Name: "faryen_flash", Value: "0190a7c4-5c1e-7b4e-9a40-6b3c1e2d4f5a"}}

	f.redis.SetError("storage unavailable")

	response := f.do(request{path: "/admin/en/sign-in", cookies: guest})

	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("GET sign-in with storage down = %d %s", response.Code, response.Header().Get("Content-Type"))
	}

	response = f.do(request{method: http.MethodPost, path: "/admin/en/sign-in", body: "email=ada@example.com&password=wrong"})

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "Notifications are temporarily unavailable.") {
		t.Fatalf("failed sign-in with storage down = %d: %s", response.Code, response.Body.String())
	}
}
