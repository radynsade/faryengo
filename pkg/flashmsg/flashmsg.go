// Package flashmsg provides a serializable, typed flash-message bag.
// Applications own persistence and decide when to consume the messages.
package flashmsg

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

const (
	Success = "success"
	Error   = "error"
	Info    = "info"
	Warning = "warning"
)

// Bag groups messages by application-defined type. Its zero value is ready to use.
// A bag belongs to one request and must not be shared between goroutines.
type Bag struct {
	messages map[string][]string
}

type contextKey struct{}

func New() *Bag {
	return &Bag{}
}

// Add appends a message without replacing earlier messages of the same type.
func (b *Bag) Add(kind, message string) {
	if b.messages == nil {
		b.messages = make(map[string][]string)
	}

	b.messages[kind] = append(b.messages[kind], message)
}

// Peek reads messages of a type without consuming them.
func (b *Bag) Peek(kind string) []string {
	return slices.Clone(b.messages[kind])
}

// Get reads and consumes messages of a type.
func (b *Bag) Get(kind string) []string {
	messages := b.Peek(kind)
	delete(b.messages, kind)
	return messages
}

// Types returns message types in deterministic order for rendering.
func (b *Bag) Types() []string {
	return slices.Sorted(maps.Keys(b.messages))
}

// PeekAll reads all messages without consuming them. The result is a deep copy.
func (b *Bag) PeekAll() map[string][]string {
	messages := make(map[string][]string, len(b.messages))

	for _, kind := range b.Types() {
		messages[kind] = b.Peek(kind)
	}

	return messages
}

// All reads and consumes all messages.
func (b *Bag) All() map[string][]string {
	messages := b.PeekAll()
	b.messages = nil
	return messages
}

func (b *Bag) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(b.PeekAll())

	if err != nil {
		err = fmt.Errorf("encode flash messages: %w", err)
	}

	return data, err
}

// UnmarshalJSON replaces the bag only after successfully decoding the messages.
func (b *Bag) UnmarshalJSON(data []byte) error {
	var messages map[string][]string
	err := json.Unmarshal(data, &messages)

	if err == nil {
		b.messages = messages
	} else {
		err = fmt.Errorf("decode flash messages: %w", err)
	}

	return err
}

// WithBag makes a request's bag available to rendering code without HTTP coupling.
func WithBag(ctx context.Context, bag *Bag) context.Context {
	return context.WithValue(ctx, contextKey{}, bag)
}

// FromContext returns the request's bag, or an empty bag when none was attached.
func FromContext(ctx context.Context) *Bag {
	bag, ok := ctx.Value(contextKey{}).(*Bag)

	if !ok || bag == nil {
		bag = New()
	}

	return bag
}
