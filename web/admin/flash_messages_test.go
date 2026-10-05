package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"

	"github.com/radynsade/faryengo/internal/security"
	securityredis "github.com/radynsade/faryengo/internal/security/redis"
	"github.com/radynsade/faryengo/pkg/flashmsg"
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

	principal := security.Principal{UserID: security.UserID(uuid.New()), SessionID: uuid.New()}
	session := security.Session{ID: principal.SessionID, UserID: principal.UserID, CredentialVersion: uuid.New(), RefreshHash: strings.Repeat("a", 64), ExpiresAt: time.Now().Add(time.Hour)}

	if err := store.Create(t.Context(), session); err != nil {
		t.Fatal(err)
	}

	return &Handler{flashStorage: client, secureCookies: true}, mini, store, principal
}

func requestWithFlashState(ctx context.Context, principal *security.Principal) *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/admin/en", nil)
	return request.WithContext(context.WithValue(ctx, flashRequestKey{}, &flashRequest{principal: principal}))
}

func TestFlashSessionStorage(t *testing.T) {
	handler, mini, store, principal := flashFixture(t)
	request := requestWithFlashState(t.Context(), &principal)
	keys, _, err := handler.flashKeys(request, false)

	if err != nil {
		t.Fatal(err)
	}

	originalTTL := mini.TTL(keys[1])
	mini.HSet(keys[1], "another-app:flashes", `{"info":["Keep this"]}`)

	for _, message := range []struct{ kind, text string }{
		{flashmsg.Success, "First"}, {flashmsg.Error, "Invalid"}, {flashmsg.Success, "Second"},
	} {
		if err := handler.addFlash(t.Context(), httptest.NewRecorder(), request, message.kind, message.text); err != nil {
			t.Fatal(err)
		}
	}

	bag, err := handler.readFlashes(t.Context(), request, flashmsg.Error)

	if err != nil || !slices.Equal(bag.Get(flashmsg.Error), []string{"Invalid"}) || len(bag.Peek(flashmsg.Success)) != 0 {
		t.Fatalf("typed consumption = %v, %v", bag, err)
	}

	bag, err = handler.readFlashes(t.Context(), request, "")

	if err != nil || !slices.Equal(bag.Get(flashmsg.Success), []string{"First", "Second"}) {
		t.Fatalf("remaining successes = %v, %v", bag, err)
	}

	bag, err = handler.readFlashes(t.Context(), request, "")

	if err != nil || len(bag.All()) != 0 {
		t.Fatalf("messages replayed: %v, %v", bag, err)
	}

	if mini.TTL(keys[1]) != originalTTL || mini.HGet(keys[1], "another-app:flashes") != `{"info":["Keep this"]}` {
		t.Fatal("flashes changed session expiration or another application's data")
	}

	if _, err := store.FindByID(t.Context(), principal.UserID, principal.SessionID); err != nil {
		t.Fatalf("flash consumption damaged the authentication session: %v", err)
	}
}

func TestFlashSessionIsolation(t *testing.T) {
	handler, _, store, principal := flashFixture(t)
	first := requestWithFlashState(t.Context(), &principal)
	other := principal
	other.SessionID = uuid.New()
	session := security.Session{ID: other.SessionID, UserID: other.UserID, CredentialVersion: uuid.New(), RefreshHash: strings.Repeat("b", 64), ExpiresAt: time.Now().Add(time.Hour)}

	if err := store.Create(t.Context(), session); err != nil {
		t.Fatal(err)
	}

	if err := handler.addFlash(t.Context(), httptest.NewRecorder(), first, flashmsg.Success, "First session only"); err != nil {
		t.Fatal(err)
	}

	second := requestWithFlashState(t.Context(), &other)
	bag, err := handler.readFlashes(t.Context(), second, "")

	if err != nil || len(bag.All()) != 0 {
		t.Fatal("one device read another device's flashes")
	}

	bag, err = handler.readFlashes(t.Context(), first, "")

	if err != nil || !slices.Equal(bag.Get(flashmsg.Success), []string{"First session only"}) {
		t.Fatal("the other device consumed the first device's flashes")
	}
}

func TestFlashSessionRevocation(t *testing.T) {
	for _, tt := range []struct {
		name   string
		revoke func(*testing.T, *miniredis.Miniredis, *securityredis.SessionStore, security.Principal, []string)
	}{
		{name: "sign-out", revoke: func(t *testing.T, _ *miniredis.Miniredis, store *securityredis.SessionStore, principal security.Principal, _ []string) {
			if err := store.Revoke(t.Context(), principal.UserID, principal.SessionID); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "all devices", revoke: func(t *testing.T, _ *miniredis.Miniredis, store *securityredis.SessionStore, principal security.Principal, _ []string) {
			if err := store.RevokeAll(t.Context(), principal.UserID); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "expired key", revoke: func(_ *testing.T, mini *miniredis.Miniredis, _ *securityredis.SessionStore, _ security.Principal, _ []string) {
			mini.FastForward(time.Hour)
		}},
		{name: "absolute expiry", revoke: func(_ *testing.T, mini *miniredis.Miniredis, _ *securityredis.SessionStore, _ security.Principal, keys []string) {
			mini.HSet(keys[1], "expires", "1")
		}},
		{name: "missing generation", revoke: func(_ *testing.T, mini *miniredis.Miniredis, _ *securityredis.SessionStore, _ security.Principal, keys []string) {
			mini.Del(keys[0])
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			handler, mini, store, principal := flashFixture(t)
			request := requestWithFlashState(t.Context(), &principal)
			keys, _, err := handler.flashKeys(request, false)

			if err != nil {
				t.Fatal(err)
			}

			if err := handler.addFlash(t.Context(), httptest.NewRecorder(), request, flashmsg.Success, "Queued"); err != nil {
				t.Fatal(err)
			}

			tt.revoke(t, mini, store, principal, keys)
			existed := mini.Exists(keys[1])
			ttl := mini.TTL(keys[1])

			if err := handler.addFlash(t.Context(), httptest.NewRecorder(), request, flashmsg.Error, "Do not resurrect"); !errors.Is(err, security.ErrSessionRevoked) {
				t.Fatalf("add to revoked session = %v", err)
			}

			if _, err := handler.readFlashes(t.Context(), request, ""); !errors.Is(err, security.ErrSessionRevoked) {
				t.Fatalf("read revoked session = %v", err)
			}

			if mini.Exists(keys[1]) != existed || mini.TTL(keys[1]) != ttl {
				t.Fatal("flash access resurrected or prolonged a revoked session")
			}
		})
	}
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

			keys, _, err := handler.flashKeys(request, false)

			if err != nil || mini.TTL(keys[1]) != anonymousFlashTTL {
				t.Fatalf("anonymous session lifetime = %v, %v", mini.TTL(keys[1]), err)
			}

			next := requestWithFlashState(t.Context(), nil)
			next.AddCookie(cookies[0])
			bag, err = handler.readFlashes(t.Context(), next, "")

			if err != nil || !slices.Equal(bag.Get(flashmsg.Error), []string{"Sign in again"}) {
				t.Fatalf("anonymous redirect lost its flash: %v, %v", bag, err)
			}

			mini.FastForward(anonymousFlashTTL)
			bag, err = handler.readFlashes(t.Context(), next, "")

			if err != nil || len(bag.All()) != 0 || mini.Exists(keys[1]) {
				t.Fatal("reading an expired guest session recreated it")
			}
		})
	}
}

func TestConcurrentFlashStorage(t *testing.T) {
	handler, _, _, principal := flashFixture(t)
	group, ctx := errgroup.WithContext(t.Context())

	for writer := range 10 {
		group.Go(func() error {
			request := requestWithFlashState(ctx, &principal)
			var err error

			for message := range 10 {
				err = handler.addFlash(ctx, httptest.NewRecorder(), request, flashmsg.Success, fmt.Sprintf("%d:%d", writer, message))

				if err != nil {
					break
				}
			}

			return err
		})
	}

	if err := group.Wait(); err != nil {
		t.Fatal(err)
	}

	var consumed atomic.Int64
	group, ctx = errgroup.WithContext(t.Context())

	for range 10 {
		group.Go(func() error {
			request := requestWithFlashState(ctx, &principal)
			bag, err := handler.readFlashes(ctx, request, "")

			if err == nil {
				messages := bag.Get(flashmsg.Success)
				consumed.Add(int64(len(messages)))

				if len(messages) != 0 && len(messages) != 100 {
					err = fmt.Errorf("non-atomic consumption returned %d messages", len(messages))
				}

				slices.Sort(messages)

				for i := 1; i < len(messages); i++ {
					if messages[i] == messages[i-1] {
						err = fmt.Errorf("duplicate flash %q", messages[i])
						break
					}
				}
			}

			return err
		})
	}

	if err := group.Wait(); err != nil {
		t.Fatal(err)
	}

	if consumed.Load() != 100 {
		t.Fatalf("concurrent requests consumed %d flashes, want 100", consumed.Load())
	}
}

func TestFlashStorageCancellation(t *testing.T) {
	handler, _, _, principal := flashFixture(t)
	request := requestWithFlashState(t.Context(), &principal)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := handler.addFlash(ctx, httptest.NewRecorder(), request, flashmsg.Success, "Cancelled"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled add = %v", err)
	}

	if _, err := handler.readFlashes(ctx, request, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read = %v", err)
	}

	bag, err := handler.readFlashes(t.Context(), request, "")

	if err != nil || len(bag.All()) != 0 {
		t.Fatal("cancelled write left a flash")
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

func TestFlashSurvivesSessionRefresh(t *testing.T) {
	mux, _, _ := httpFixture(t)
	cookies := login(t, mux)
	created := httpRequest(mux, http.MethodPost, "/admin/en/roles/create", "name[en]=Editors", cookies)

	if created.Code != http.StatusSeeOther {
		t.Fatalf("create = %d", created.Code)
	}

	refreshed := httpRequest(mux, http.MethodPost, "/admin/en/refresh", "", cookies)

	if refreshed.Code != http.StatusSeeOther {
		t.Fatalf("refresh = %d", refreshed.Code)
	}

	for render := range 2 {
		response := httpRequest(mux, http.MethodGet, created.Header().Get("Location"), "", refreshed.Result().Cookies())
		shown := strings.Contains(response.Body.String(), "created successfully.")

		if response.Code != http.StatusOK || shown != (render == 0) {
			t.Fatalf("flash after refresh, render %d = %d: %s", render, response.Code, response.Body.String())
		}
	}
}

type failedFlashStorage struct{ err error }

func (s failedFlashStorage) Eval(ctx context.Context, _ string, _ []string, _ ...any) *redislib.Cmd {
	command := redislib.NewCmd(ctx)
	command.SetErr(s.err)
	return command
}
