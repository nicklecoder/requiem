package reach

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// requiem: embedding/fail-fast-unreachable
func TestUnreachable_TellsAnOutageFromAnAnswer(t *testing.T) {
	c := Client(30 * time.Second)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := "http://" + l.Addr().String()
	l.Close()
	if _, err := c.Get(closed); !Unreachable(err) {
		t.Fatalf("a refused connection is unreachable, got %v", err)
	}
	if _, err := c.Get("http://requiem-no-such-host.invalid"); !Unreachable(err) {
		t.Fatalf("a name that does not resolve is unreachable, got %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "model not found", http.StatusNotFound)
	}))
	defer srv.Close()
	resp, err := c.Get(srv.URL)
	if err != nil || Unreachable(err) {
		t.Fatalf("a server that answers, even with an error, is reachable: %v", err)
	}
	resp.Body.Close()
}

// requiem: embedding/fail-fast-unreachable
// An address that drops packets, as a LAN server does from elsewhere, gives
// up after the connect bound, not the request timeout.
func TestClient_GivesUpConnectingQuickly(t *testing.T) {
	c := Client(30 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://192.0.2.1:9/", nil) // TEST-NET-1: never routed
	start := time.Now()
	_, err := c.Do(req)
	if err == nil {
		t.Skip("192.0.2.1 answered on this network")
	}
	if d := time.Since(start); d > ConnectTimeout+2*time.Second {
		t.Fatalf("connecting to a dropped address took %s, want about %s", d, ConnectTimeout)
	}
	if !Unreachable(err) {
		t.Fatalf("a connect timeout is unreachable, got %v", err)
	}
}
