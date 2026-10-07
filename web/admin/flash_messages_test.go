package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"

	"github.com/radynsade/faryengo/internal/security"
	securityredis "github.com/radynsade/faryengo/internal/security/redis"
	"github.com/radynsade/faryengo/pkg/flashmsg"
	flashredis "github.com/radynsade/faryengo/pkg/flashmsg/redis"
)

func flashFixture(t *testing.T) (*Handler, *miniredis.Miniredis, *securityredis.SessionStore, security.Principal) {
	t.Helper()
	mini := miniredis.RunT(t)
	client := redislib.NewClient(&redislib.Options{Addr: mini.Addr(), MaxRetries: -1, DialTimeout: 100 * time.Millisecond})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	store, err := securityredis.NewSessionStore(client)

	if err != nil {
		t.Fatal(err)
	}

	principal := security.Principal{UserID: security.UserID(newTestUUID(t)), SessionID: newTestUUID(t)}
	session := security.Session{ID: principal.SessionID, UserID: principal.UserID, AuthenticationSnapshotVersion: newTestUUID(t), ExpiresAt: time.Now().Add(time.Hour)}

	if err := store.CreateSession(t.Context(), session, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}

	flashes, err := flashredis.NewStore(client, "admin", anonymousFlashTTL)

	if err != nil {
		t.Fatal(err)
	}

	return &Handler{flashStorage: flashes, secureCookies: true}, mini, store, principal
}

func requestWithFlashState(ctx context.Context, principal *security.Principal) *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/admin/en", nil)
	return request.WithContext(context.WithValue(ctx, flashRequestKey{}, &flashRequest{principal: principal}))
}

func TestAnonymousFlashSession(t *testing.T) {
	for _, secure := range []bool{true, false} {
		t.Run(fmt.Sprintf("secure=%t", secure), func(t *testing.T) {
			handler, mini, _, _ := flashFixture(t)
			handler.secureCookies = secure
			request := requestWithFlashState(t.Context(), nil)
			originalKeys := mini.Keys()
			bag, err := handler.readFlashes(t.Context(), request, "")

			if err != nil || len(bag.All()) != 0 || !slices.Equal(mini.Keys(), originalKeys) {
				t.Fatal("reading flashes created an anonymous session")
			}

			response := httptest.NewRecorder()

			if err := handler.addFlash(t.Context(), response, request, flashmsg.Error, "Sign in again"); err != nil {
				t.Fatal(err)
			}

			cookies := response.Result().Cookies()

			if len(cookies) != 1 || cookies[0].Secure != secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Path != "/" || cookies[0].MaxAge != int(anonymousFlashTTL.Seconds()) {
				t.Fatalf("anonymous flash cookie = %v", cookies)
			}

			id, err := uuid.Parse(cookies[0].Value)

			if err != nil || id.Version() != 7 {
				t.Fatalf("anonymous flash ID must be UUIDv7: %v, %v", id, err)
			}

			session, err := handler.flashSession(request, false)

			if err != nil || mini.TTL(session.Key) != anonymousFlashTTL {
				t.Fatalf("anonymous session lifetime = %v, %v", mini.TTL(session.Key), err)
			}

			next := requestWithFlashState(t.Context(), nil)
			next.AddCookie(cookies[0])
			bag, err = handler.readFlashes(t.Context(), next, "")

			if err != nil || !slices.Equal(bag.Get(flashmsg.Error), []string{"Sign in again"}) {
				t.Fatalf("anonymous redirect lost its flash: %v, %v", bag, err)
			}

			mini.FastForward(anonymousFlashTTL)
			bag, err = handler.readFlashes(t.Context(), next, "")

			if err != nil || len(bag.All()) != 0 || mini.Exists(session.Key) {
				t.Fatal("reading an expired guest session recreated it")
			}
		})
	}
}

func TestFlashStorageFailureResponse(t *testing.T) {
	for _, tt := range []struct {
		name, method, path, body string
		writes                   int
	}{
		{name: "rendering", method: http.MethodGet, path: "/admin/en/roles"},
		{name: "validation error", method: http.MethodPost, path: "/admin/en/roles/create", body: "permissions=view_role"},
		{name: "confirmation after write", method: http.MethodPost, path: "/admin/en/roles/create", body: "name[en]=Saved", writes: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			failure := errors.New("private storage error")
			mux, repository, _ := httpFixtureWithFlashStorage(t, true, failedFlashStorage{err: failure})
			response := httpRequest(mux, tt.method, tt.path, tt.body, login(t, mux))

			if response.Code != http.StatusServiceUnavailable || response.Header().Get("Location") != "" || response.Header().Get("Cache-Control") != "no-store" || repository.roleWrites != tt.writes {
				t.Fatalf("storage failure = %d, writes = %d: %s", response.Code, repository.roleWrites, response.Body.String())
			}

			if !strings.Contains(response.Body.String(), "reload before submitting again") || strings.Contains(response.Body.String(), failure.Error()) {
				t.Fatal("storage failure leaked details or invited a duplicate write")
			}
		})
	}
}

func TestFlashSurvivesSessionNavigation(t *testing.T) {
	mux, _, _ := httpFixture(t)
	cookies := login(t, mux)
	created := httpRequest(mux, http.MethodPost, "/admin/en/roles/create", "name[en]=Editors", cookies)

	if created.Code != http.StatusSeeOther {
		t.Fatalf("create = %d", created.Code)
	}

	for render := range 2 {
		response := httpRequest(mux, http.MethodGet, created.Header().Get("Location"), "", cookies)
		shown := strings.Contains(response.Body.String(), "created successfully.")

		if response.Code != http.StatusOK || shown != (render == 0) {
			t.Fatalf("flash after redirect, render %d = %d: %s", render, response.Code, response.Body.String())
		}
	}
}

type failedFlashStorage struct{ err error }

func (s failedFlashStorage) Add(context.Context, flashmsg.Session, string, string) error {
	return s.err
}

func (s failedFlashStorage) Take(context.Context, flashmsg.Session, string) (*flashmsg.Bag, error) {
	return nil, s.err
}

func TestFlashRevokedSessionTransport(t *testing.T) {
	handler, _, store, principal := flashFixture(t)
	request := requestWithFlashState(t.Context(), &principal)

	if err := store.Revoke(t.Context(), principal.UserID, principal.SessionID); err != nil {
		t.Fatal(err)
	}

	if err := handler.addFlash(t.Context(), httptest.NewRecorder(), request, flashmsg.Error, "Revoked"); !errors.Is(err, security.ErrSessionRevoked) {
		t.Fatalf("revoked flash add = %v", err)
	}

	if _, err := handler.readFlashes(t.Context(), request, ""); !errors.Is(err, security.ErrSessionRevoked) {
		t.Fatalf("revoked flash read = %v", err)
	}
}
