package security

import (
	"context"
	"errors"
)

//
// User repository
//

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("user already exists")
	ErrUserConflict      = errors.New("user changed since it was loaded")
)

type ErrUserCreateFailed interface {
	error
	User() *User
	Unwrap() error
}

type ErrUserUpdateFailed interface {
	error
	User() *User
	Unwrap() error
}

type ErrUserDeleteFailed interface {
	error
	UserID() UserID
	Unwrap() error
}

type UserRepository interface {
	Create(ctx context.Context, user *User) ErrUserCreateFailed
	Update(ctx context.Context, user *User) ErrUserUpdateFailed
	Delete(ctx context.Context, id UserID) ErrUserDeleteFailed
	FindByID(ctx context.Context, id UserID) (*User, error)
	FindByEmail(ctx context.Context, email Email) (*User, error)
}
