package limits

import (
	"errors"

	"github.com/google/uuid"
	"github.com/radynsade/faryengo/internal/categories"
)

//
// ID
//

var ErrIDInvalid = errors.New("invalid ID")

type ID uuid.UUID

func NewID(value uuid.UUID) (ID, error) {
	var (
		err error
		id  ID
	)

	id = ID(value)
	err = id.Validate()
	return id, err
}

func (id ID) Validate() error {
	var err error

	if uuid.UUID(id) == uuid.Nil {
		err = ErrIDInvalid
	}

	return err
}

//
// Budget
//

type Budget struct {
	ID         ID
	Categories []categories.ID
}
