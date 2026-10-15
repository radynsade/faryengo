package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestClientAddress(t *testing.T) {
	trusted := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("::1/128"),
	}

	tests := []struct {
		name      string
		trusted   []netip.Prefix
		remote    string
		forwarded []string
		want      string
	}{
		{"no trusted proxies", nil, "10.0.0.1:4000", []string{"203.0.113.9"}, "10.0.0.1:4000"},
		{"untrusted peer is never replaced", trusted, "198.51.100.7:4000", []string{"203.0.113.9"}, "198.51.100.7:4000"},
		{"trusted peer without the header", trusted, "10.0.0.1:4000", nil, "10.0.0.1:4000"},
		{"one proxy", trusted, "10.0.0.1:4000", []string{"203.0.113.9"}, "203.0.113.9:0"},
		{"IPv6 proxy and client", trusted, "[::1]:4000", []string{"2001:db8::5"}, "[2001:db8::5]:0"},
		{"chained trusted proxies", trusted, "10.0.0.1:4000", []string{"203.0.113.9, 10.0.0.2"}, "203.0.113.9:0"},
		{"forged entries left of the client are ignored", trusted, "10.0.0.1:4000", []string{"192.0.2.66, 203.0.113.9"}, "203.0.113.9:0"},
		{"repeated headers are read as one list", trusted, "10.0.0.1:4000", []string{"192.0.2.66", "203.0.113.9"}, "203.0.113.9:0"},
		{"malformed entry stops at the last trusted hop", trusted, "10.0.0.1:4000", []string{"203.0.113.9, junk, 10.0.0.2"}, "10.0.0.2:0"},
		{"only trusted entries", trusted, "10.0.0.1:4000", []string{"10.0.0.3, 10.0.0.2"}, "10.0.0.3:0"},
		{"IPv4-mapped peer", trusted, "[::ffff:10.0.0.1]:4000", []string{"203.0.113.9"}, "203.0.113.9:0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got string

			handler := ClientAddress(test.trusted, http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
				got = request.RemoteAddr
			}))

			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.RemoteAddr = test.remote

			for _, value := range test.forwarded {
				request.Header.Add("X-Forwarded-For", value)
			}

			handler.ServeHTTP(httptest.NewRecorder(), request)

			if got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestRemoteHost(t *testing.T) {
	tests := []struct {
		name   string
		remote string
		want   string
	}{
		{"IPv4 with a port", "203.0.113.9:4000", "203.0.113.9"},
		{"IPv6 with a port", "[2001:db8::5]:0", "2001:db8::5"},
		{"no port", "203.0.113.9", "203.0.113.9"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.RemoteAddr = test.remote

			if got := RemoteHost(request); got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}
