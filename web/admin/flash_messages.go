package admin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/pkg/flashmsg"
	flashredis "github.com/radynsade/faryengo/pkg/flashmsg/redis"
	admini18n "github.com/radynsade/faryengo/web/admin/i18n"
)

const anonymousFlashTTL = 15 * time.Minute

type flashRequestKey struct{}

type flashRequest struct {
	principal *security.Principal
	guestID   uuid.UUID
}

func flashState(request *http.Request) *flashRequest {
	state, _ := request.Context().Value(flashRequestKey{}).(*flashRequest)
	return state
}

func (h *Handler) flashSession(request *http.Request, create bool) (flashmsg.Session, error) {
	state := flashState(request)
	var session flashmsg.Session
	var err error

	if state.principal != nil {
		principal := state.principal
		prefix := "faryen:security:{" + uuid.UUID(principal.UserID).String() + "}"
		session = flashmsg.Session{GenerationKey: prefix + ":generation", Key: prefix + ":session:" + principal.SessionID.String()}
	} else {
		if state.guestID == uuid.Nil {
			if id, parseErr := uuid.Parse(h.cookie(request, "flash")); parseErr == nil {
				state.guestID = id
			}
		}

		if state.guestID == uuid.Nil && create {
			state.guestID, err = uuid.NewRandom()
		}

		if state.guestID != uuid.Nil {
			key := "faryen:web:admin:session:{" + state.guestID.String() + "}"
			session = flashmsg.Session{Key: key}
		}
	}

	return session, err
}

func (h *Handler) addFlash(ctx context.Context, writer http.ResponseWriter, request *http.Request, kind, message string) error {
	session, err := h.flashSession(request, true)

	if err == nil {
		err = h.flashStorage.Add(ctx, session, kind, message)

		if errors.Is(err, flashredis.ErrSessionRevoked) {
			err = security.ErrSessionRevoked
		}

		if err == nil && session.GenerationKey == "" {
			http.SetCookie(writer, &http.Cookie{
				Name: h.cookieName("flash"), Value: flashState(request).guestID.String(),
				Path: "/", HttpOnly: true, Secure: h.secureCookies, SameSite: http.SameSiteStrictMode,
				MaxAge: int(anonymousFlashTTL.Seconds()), Expires: time.Now().Add(anonymousFlashTTL),
			})
		}
	}

	if err != nil {
		err = fmt.Errorf("add admin %s flash: %w", kind, err)
	}

	return err
}

// readFlashes atomically consumes all flashes, or only kind when it is nonempty.
func (h *Handler) readFlashes(ctx context.Context, request *http.Request, kind string) (*flashmsg.Bag, error) {
	bag := flashmsg.New()
	session, err := h.flashSession(request, false)

	if err == nil && session.Key != "" {
		bag, err = h.flashStorage.Take(ctx, session, kind)

		if errors.Is(err, flashredis.ErrSessionRevoked) {
			err = security.ErrSessionRevoked
		}
	}

	if err != nil {
		bag = nil
		err = fmt.Errorf("read admin flashes: %w", err)
	}

	return bag, err
}

func (h *Handler) flashUnavailable(writer http.ResponseWriter, request *http.Request, err error) {
	slog.ErrorContext(request.Context(), "admin flash session storage", "error", err)
	http.Error(writer, admini18n.T(request.Context(), "errors.notifications"), http.StatusServiceUnavailable)
}
