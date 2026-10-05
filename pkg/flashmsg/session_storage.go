package flashmsg

import "context"

// Session identifies the storage key for a message session. GenerationKey is
// empty for anonymous sessions and identifies the current generation for
// authenticated sessions.
type Session struct {
	Key           string
	GenerationKey string
}

// FlashSessionStorage persists and atomically consumes session messages.
type FlashSessionStorage interface {
	Add(ctx context.Context, session Session, kind, message string) error
	// Take consumes one type, or all types when kind is empty.
	Take(ctx context.Context, session Session, kind string) (*Bag, error)
}
