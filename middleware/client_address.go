package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientAddress replaces a request's RemoteAddr with the address of the client
// behind the trusted proxies, so later handlers see who actually connected.
// X-Forwarded-For is read only when the connection itself comes from a
// trusted proxy, and only from the right: each proxy appends the address it
// received the request from, so the first untrusted address from the right is
// the one no client can forge. A malformed entry ends the search at the last
// trusted hop. Every proxy that forwards to the server must be trusted, and
// must append to X-Forwarded-For rather than pass it through. Without trusted
// proxies the request is left untouched.

func ClientAddress(trusted []netip.Prefix, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		peer, ok := remoteAddr(request.RemoteAddr)

		if ok && isTrusted(trusted, peer) {
			client := forwardedClient(trusted, peer, request.Header.Values("X-Forwarded-For"))

			if client != peer {
				request = request.Clone(request.Context())
				request.RemoteAddr = netip.AddrPortFrom(client, 0).String()
			}
		}

		next.ServeHTTP(writer, request)
	})
}

// RemoteHost returns the host part of a request's RemoteAddr, or the whole
// value when it has no port.

func RemoteHost(request *http.Request) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)

	if err != nil {
		host = request.RemoteAddr
	}

	return host
}

//
// Helpers
//

func remoteAddr(value string) (netip.Addr, bool) {
	addrPort, err := netip.ParseAddrPort(value)

	return addrPort.Addr().Unmap(), err == nil
}

func isTrusted(trusted []netip.Prefix, addr netip.Addr) bool {
	found := false

	for index := 0; !found && index < len(trusted); index++ {
		found = trusted[index].Contains(addr)
	}

	return found
}

func forwardedClient(trusted []netip.Prefix, peer netip.Addr, headers []string) netip.Addr {
	var entries []string

	for _, header := range headers {
		entries = append(entries, strings.Split(header, ",")...)
	}

	client, done := peer, false

	for index := len(entries) - 1; !done && index >= 0; index-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(entries[index]))

		if err != nil {
			done = true
		} else {
			client = addr.Unmap()
			done = !isTrusted(trusted, client)
		}
	}

	return client
}
