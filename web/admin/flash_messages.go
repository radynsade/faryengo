package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"

	"github.com/radynsade/faryengo/internal/security"
	"github.com/radynsade/faryengo/pkg/flashmsg"
)

// FlashSessionStorage is the session-storage operation used by the admin transport.
// Atomic scripts prevent concurrent requests from overwriting or replaying flashes.
type FlashSessionStorage interface {
	Eval(context.Context, string, []string, ...any) *redislib.Cmd
}

const anonymousFlashTTL = 15 * time.Minute

// Flashes share the authenticated session hash and its existing expiration.
// This script never recreates a missing or revoked authenticated session.
const adminFlashScript = `
local session = KEYS[2]
if ARGV[1] == 'authenticated' then
    local generation = redis.call('GET', KEYS[1])
    local fields = redis.call('HMGET', session, 'generation', 'expires')
    local now = redis.call('TIME')
    local milliseconds = tonumber(now[1]) * 1000 + math.floor(tonumber(now[2]) / 1000)
    if not generation or fields[1] ~= generation or not fields[2] or tonumber(fields[2]) <= milliseconds then
        return nil
    end
elseif ARGV[2] == 'add' then
    redis.call('HSET', session, 'application', 'admin')
    redis.call('EXPIRE', session, ARGV[4])
end
local field = 'admin:flashes'
local data = redis.call('HGET', session, field)
if ARGV[2] == 'take' then
    if ARGV[3] ~= '' and data then
        local messages = cjson.decode(data)
        local selected = {}
        if messages[ARGV[3]] then
            selected[ARGV[3]] = messages[ARGV[3]]
            messages[ARGV[3]] = nil
        end
        if next(messages) then
            redis.call('HSET', session, field, cjson.encode(messages))
        else
            redis.call('HDEL', session, field)
        end
        return cjson.encode(selected)
    end
    redis.call('HDEL', session, field)
    return data or '{}'
end
local messages = cjson.decode(data or '{}')
local added = cjson.decode(ARGV[3])
for kind, items in pairs(added) do
    if not messages[kind] then messages[kind] = {} end
    for _, message in ipairs(items) do
        table.insert(messages[kind], message)
    end
end
redis.call('HSET', session, field, cjson.encode(messages))
return 1
`

type flashRequestKey struct{}

type flashRequest struct {
	principal *security.Principal
	guestID   uuid.UUID
}

func flashState(request *http.Request) *flashRequest {
	state, _ := request.Context().Value(flashRequestKey{}).(*flashRequest)
	return state
}

func (h *Handler) flashKeys(request *http.Request, create bool) ([]string, string, error) {
	state := flashState(request)
	var keys []string
	var mode string
	var err error

	if state.principal != nil {
		principal := state.principal
		prefix := "faryen:security:{" + uuid.UUID(principal.UserID).String() + "}"
		keys = []string{prefix + ":generation", prefix + ":session:" + principal.SessionID.String()}
		mode = "authenticated"
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
			keys = []string{key, key}
			mode = "anonymous"
		}
	}

	return keys, mode, err
}

func (h *Handler) addFlash(ctx context.Context, writer http.ResponseWriter, request *http.Request, kind, message string) error {
	keys, mode, err := h.flashKeys(request, true)

	if err == nil {
		bag := flashmsg.New()
		bag.Add(kind, message)
		var data []byte
		data, err = json.Marshal(bag)

		if err == nil {
			_, err = h.flashStorage.Eval(ctx, adminFlashScript, keys, mode, "add", string(data), int(anonymousFlashTTL.Seconds())).Int()
		}

		if errors.Is(err, redislib.Nil) {
			err = security.ErrSessionRevoked
		}

		if err == nil {
			if mode == "anonymous" {
				http.SetCookie(writer, &http.Cookie{
					Name: h.cookieName("flash"), Value: flashState(request).guestID.String(),
					Path: "/", HttpOnly: true, Secure: h.secureCookies, SameSite: http.SameSiteStrictMode,
					MaxAge: int(anonymousFlashTTL.Seconds()), Expires: time.Now().Add(anonymousFlashTTL),
				})
			}
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
	keys, mode, err := h.flashKeys(request, false)

	if err == nil && len(keys) != 0 {
		var data string
		data, err = h.flashStorage.Eval(ctx, adminFlashScript, keys, mode, "take", kind).Text()

		if errors.Is(err, redislib.Nil) {
			err = security.ErrSessionRevoked
		} else if err == nil {
			err = json.Unmarshal([]byte(data), bag)
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
	http.Error(writer, "Notifications are temporarily unavailable. Please reload before submitting again.", http.StatusServiceUnavailable)
}
