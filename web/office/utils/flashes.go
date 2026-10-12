package utils

import (
	"context"
	"errors"
	"fmt"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/pkg/flashmsg"
)

//
// Flash sessions
//

// A signed-in request keeps its messages under its device Session, selected
// from the verified identity, never from request parameters. The office shows
// messages only to signed-in Users, so it has no guest flash sessions.

const sessionFlashKeyPrefix = "faryen:web:office:flashes:session:"

var ErrFlashStorageNil = errors.New("flash storage is nil")

type Flashes struct {
	storage flashmsg.FlashSessionStorage
}

func NewFlashes(storage flashmsg.FlashSessionStorage) (*Flashes, error) {
	var (
		flashes *Flashes
		err     error
	)

	if storage == nil {
		err = ErrFlashStorageNil
	} else {
		flashes = &Flashes{storage: storage}
	}

	return flashes, err
}

// The Session is passed explicitly, because a change of credentials replaces
// the request's Session with a new one before the confirmation is stored.

func (f *Flashes) Add(
	ctx context.Context,
	session *security.Session,
	kind string,
	message string,
) error {
	err := f.storage.Add(ctx, flashSession(session), kind, message)

	if err != nil {
		err = fmt.Errorf("add an office %s flash: %w", kind, err)
	}

	return err
}

// Take consumes every kind of message.

func (f *Flashes) Take(ctx context.Context, session *security.Session) (*flashmsg.Bag, error) {
	bag, err := f.storage.Take(ctx, flashSession(session), "")

	if err != nil {
		bag = nil
		err = fmt.Errorf("take office flashes: %w", err)
	}

	return bag, err
}

//
// Helpers
//

func flashSession(session *security.Session) flashmsg.Session {
	return flashmsg.Session{Key: sessionFlashKeyPrefix + "{" + session.ID.String() + "}"}
}
