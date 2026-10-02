package main

import (
	"context"
	"net"
	"net/http"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestServeExits(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := listener.Close(); err != nil {
			t.Error(err)
		}
	})

	for _, tt := range []struct {
		name                       string
		cancelled, closed, wantErr bool
	}{
		{name: "cancelled context", cancelled: true},
		{name: "already closed", closed: true},
		{name: "listen failure", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			server := &http.Server{Addr: listener.Addr().String(), Handler: http.NewServeMux()}

			if tt.cancelled {
				cancel()
				server.Addr = "127.0.0.1:0"
			}

			if tt.closed {
				if err := server.Close(); err != nil {
					t.Fatal(err)
				}
			}

			if err := serve(ctx, server); (err != nil) != tt.wantErr {
				t.Fatalf("serve = %v, want error = %v", err, tt.wantErr)
			}
		})
	}
}
