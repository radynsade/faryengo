package utils

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/radynsade/faryengo/pkg/flashmsg"
)

//
// Flash sessions
//

// A signed-in request keeps its messages under its device Session, selected
// from the verified identity, never from request parameters. A guest gets a
// separate random flash session that only adding a message creates. Both use
// distinct key prefixes, so a guest cookie can never name a device Session's
// messages.

const (
	FlashTTL              = 15 * time.Minute
	sessionFlashKeyPrefix = "faryen:web:admin:flashes:session:"
	guestFlashKeyPrefix   = "faryen:web:admin:flashes:guest:"
)

var ErrFlashStorageNil = errors.New("flash storage is nil")

type Flashes struct {
	storage flashmsg.FlashSessionStorage
	cookies Cookies
}

func NewFlashes(storage flashmsg.FlashSessionStorage, cookies Cookies) (*Flashes, error) {
	var (
		flashes *Flashes
		err     error
	)

	if storage == nil {
		err = ErrFlashStorageNil
	} else {
		flashes = &Flashes{storage: storage, cookies: cookies}
	}

	return flashes, err
}

// Add

func (f *Flashes) Add(
	writer http.ResponseWriter,
	request *http.Request,
	kind string,
	message string,
) error {
	session, err := f.session(request, true)

	if err == nil {
		err = f.storage.Add(request.Context(), session, kind, message)
	}

	if err == nil && StateOf(request).Identity == nil {
		f.cookies.Set(writer, FlashCookie, StateOf(request).GuestID, time.Now().Add(FlashTTL))
	}

	if err != nil {
		err = fmt.Errorf("add an admin %s flash: %w", kind, err)
	}

	return err
}

// Take consumes one kind of message, or every kind when kind is empty.

func (f *Flashes) Take(request *http.Request, kind string) (*flashmsg.Bag, error) {
	bag := flashmsg.New()
	session, err := f.session(request, false)

	if err == nil && session.Key != "" {
		bag, err = f.storage.Take(request.Context(), session, kind)
	}

	if err != nil {
		bag = nil
		err = fmt.Errorf("take admin flashes: %w", err)
	}

	return bag, err
}

// Show an error in the response that reports it, leaving other pending
// messages for a later page.

func (f *Flashes) ShowError(
	writer http.ResponseWriter,
	request *http.Request,
	message string,
) ([]string, error) {
	var messages []string

	err := f.Add(writer, request, flashmsg.Error, message)

	if err == nil {
		var bag *flashmsg.Bag

		bag, err = f.Take(request, flashmsg.Error)

		if err == nil {
			messages = bag.Get(flashmsg.Error)
		}
	}

	return messages, err
}

// Without flash storage no response can report its outcome. A change that
// already succeeded stays committed, so the response never suggests that it
// was undone.

func FlashUnavailable(writer http.ResponseWriter, request *http.Request, err error) {
	slog.ErrorContext(request.Context(), "admin flash storage", "error", err)
	Fail(writer, request, http.StatusServiceUnavailable, "errors.notifications")
}

//
// Helpers
//

func (f *Flashes) session(request *http.Request, create bool) (flashmsg.Session, error) {
	var (
		session flashmsg.Session
		err     error
	)

	state := StateOf(request)

	if state.Identity != nil {
		session.Key = sessionFlashKeyPrefix + "{" + state.Identity.Session.ID.String() + "}"
	} else {
		if state.GuestID == "" {
			if id, parseErr := uuid.Parse(f.cookies.Get(request, FlashCookie)); parseErr == nil {
				state.GuestID = id.String()
			}
		}

		if state.GuestID == "" && create {
			var id uuid.UUID

			id, err = uuid.NewV7()

			if err == nil {
				state.GuestID = id.String()
			}
		}

		if state.GuestID != "" {
			session.Key = guestFlashKeyPrefix + "{" + state.GuestID + "}"
		}
	}

	return session, err
}
