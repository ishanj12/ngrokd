package dns

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/ngrok/ngrokd/pkg/routing"
	mdns "github.com/miekg/dns"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := l.LocalAddr().(*net.UDPAddr).Port
	l.Close()
	return port
}

func startTestResolver(t *testing.T, upstream []string) (*Resolver, *routing.Table, string) {
	t.Helper()
	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	tbl := routing.NewTable()
	r := NewResolver(addr, upstream, tbl, logr.Discard())
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("failed to start resolver: %v", err)
	}
	t.Cleanup(func() { r.Stop() })
	return r, tbl, addr
}

func startFakeUpstream(t *testing.T) string {
	t.Helper()
	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	r := NewResolver(addr, nil, nil, logr.Discard())
	if err := r.Start(context.Background()); err != nil {
		t.Fatalf("failed to start fake upstream: %v", err)
	}
	t.Cleanup(func() { r.Stop() })
	return addr
}

func queryA(t *testing.T, addr, name string) *mdns.Msg {
	t.Helper()
	c := new(mdns.Client)
	c.Timeout = 2 * time.Second
	m := new(mdns.Msg)
	m.SetQuestion(mdns.Fqdn(name), mdns.TypeA)
	resp, _, err := c.Exchange(m, addr)
	if err != nil {
		t.Fatalf("DNS query for %s failed: %v", name, err)
	}
	return resp
}

func queryAAAA(t *testing.T, addr, name string) *mdns.Msg {
	t.Helper()
	c := new(mdns.Client)
	c.Timeout = 2 * time.Second
	m := new(mdns.Msg)
	m.SetQuestion(mdns.Fqdn(name), mdns.TypeAAAA)
	resp, _, err := c.Exchange(m, addr)
	if err != nil {
		t.Fatalf("DNS AAAA query for %s failed: %v", name, err)
	}
	return resp
}

func queryTCP(t *testing.T, addr, name string) *mdns.Msg {
	t.Helper()
	c := new(mdns.Client)
	c.Net = "tcp"
	c.Timeout = 2 * time.Second
	m := new(mdns.Msg)
	m.SetQuestion(mdns.Fqdn(name), mdns.TypeA)
	resp, _, err := c.Exchange(m, addr)
	if err != nil {
		t.Fatalf("DNS TCP query for %s failed: %v", name, err)
	}
	return resp
}

// --- Exact record tests ---

func TestResolver_ExactMatch(t *testing.T) {
	_, tbl, addr := startTestResolver(t, nil)
	tbl.AddExact("myapp.example.com", net.ParseIP("10.0.0.1"))

	resp := queryA(t, addr, "myapp.example.com")
	if len(resp.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(resp.Answer))
	}
	a, ok := resp.Answer[0].(*mdns.A)
	if !ok {
		t.Fatal("expected A record")
	}
	if !a.A.Equal(net.ParseIP("10.0.0.1")) {
		t.Fatalf("expected 10.0.0.1, got %s", a.A)
	}
}

func TestResolver_ExactMatchCaseInsensitive(t *testing.T) {
	_, tbl, addr := startTestResolver(t, nil)
	tbl.AddExact("MyApp.Example.COM", net.ParseIP("10.0.0.2"))

	resp := queryA(t, addr, "myapp.example.com")
	if len(resp.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(resp.Answer))
	}
	a := resp.Answer[0].(*mdns.A)
	if !a.A.Equal(net.ParseIP("10.0.0.2")) {
		t.Fatalf("expected 10.0.0.2, got %s", a.A)
	}
}

func TestResolver_ExactMatchTTL(t *testing.T) {
	_, tbl, addr := startTestResolver(t, nil)
	tbl.AddExact("ttl.test.com", net.ParseIP("10.0.0.5"))

	resp := queryA(t, addr, "ttl.test.com")
	if len(resp.Answer) != 1 {
		t.Fatal("expected 1 answer")
	}
	if resp.Answer[0].Header().Ttl != 5 {
		t.Fatalf("expected TTL 5, got %d", resp.Answer[0].Header().Ttl)
	}
}

func TestResolver_RemoveExact(t *testing.T) {
	upstreamAddr := startFakeUpstream(t)

	_, tbl, addr := startTestResolver(t, []string{upstreamAddr})
	tbl.AddExact("temp.example.com", net.ParseIP("10.0.0.3"))

	resp := queryA(t, addr, "temp.example.com")
	if len(resp.Answer) != 1 {
		t.Fatalf("expected 1 answer before removal, got %d", len(resp.Answer))
	}

	tbl.RemoveExact("temp.example.com")

	// After removal, the name is unmanaged and forwarded upstream.
	// The fake upstream returns an empty answer (no records).
	resp = queryA(t, addr, "temp.example.com")
	for _, ans := range resp.Answer {
		if a, ok := ans.(*mdns.A); ok && a.A.Equal(net.ParseIP("10.0.0.3")) {
			t.Fatal("still got managed IP after removal")
		}
	}
}

func TestResolver_MultipleExactRecords(t *testing.T) {
	_, tbl, addr := startTestResolver(t, nil)
	tbl.AddExact("a.test.com", net.ParseIP("10.0.0.10"))
	tbl.AddExact("b.test.com", net.ParseIP("10.0.0.11"))
	tbl.AddExact("c.test.com", net.ParseIP("10.0.0.12"))

	for _, tc := range []struct {
		host string
		ip   string
	}{
		{"a.test.com", "10.0.0.10"},
		{"b.test.com", "10.0.0.11"},
		{"c.test.com", "10.0.0.12"},
	} {
		resp := queryA(t, addr, tc.host)
		if len(resp.Answer) != 1 {
			t.Fatalf("%s: expected 1 answer, got %d", tc.host, len(resp.Answer))
		}
		a := resp.Answer[0].(*mdns.A)
		if !a.A.Equal(net.ParseIP(tc.ip)) {
			t.Fatalf("%s: expected %s, got %s", tc.host, tc.ip, a.A)
		}
	}
}

// --- Wildcard tests ---

func TestResolver_WildcardMatch(t *testing.T) {
	_, tbl, addr := startTestResolver(t, nil)
	tbl.AddWildcard("example.com", net.ParseIP("10.0.1.1"))

	resp := queryA(t, addr, "anything.example.com")
	if len(resp.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(resp.Answer))
	}
	a := resp.Answer[0].(*mdns.A)
	if !a.A.Equal(net.ParseIP("10.0.1.1")) {
		t.Fatalf("expected 10.0.1.1, got %s", a.A)
	}
}

func TestResolver_WildcardNoMatchBaseDomain(t *testing.T) {
	upstreamAddr := startFakeUpstream(t)

	_, tbl, addr := startTestResolver(t, []string{upstreamAddr})
	tbl.AddWildcard("example.com", net.ParseIP("10.0.1.1"))

	resp := queryA(t, addr, "example.com")
	for _, ans := range resp.Answer {
		if a, ok := ans.(*mdns.A); ok && a.A.Equal(net.ParseIP("10.0.1.1")) {
			t.Fatal("wildcard should not match base domain")
		}
	}
}

func TestResolver_WildcardLongestSuffixWins(t *testing.T) {
	_, tbl, addr := startTestResolver(t, nil)
	tbl.AddWildcard("example.com", net.ParseIP("10.0.1.1"))
	tbl.AddWildcard("sub.example.com", net.ParseIP("10.0.1.2"))

	resp := queryA(t, addr, "foo.sub.example.com")
	if len(resp.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(resp.Answer))
	}
	a := resp.Answer[0].(*mdns.A)
	if !a.A.Equal(net.ParseIP("10.0.1.2")) {
		t.Fatalf("expected 10.0.1.2 (longer suffix), got %s", a.A)
	}

	resp = queryA(t, addr, "foo.example.com")
	if len(resp.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(resp.Answer))
	}
	a = resp.Answer[0].(*mdns.A)
	if !a.A.Equal(net.ParseIP("10.0.1.1")) {
		t.Fatalf("expected 10.0.1.1, got %s", a.A)
	}
}

func TestResolver_WildcardRemove(t *testing.T) {
	upstreamAddr := startFakeUpstream(t)

	_, tbl, addr := startTestResolver(t, []string{upstreamAddr})
	tbl.AddWildcard("remove.test", net.ParseIP("10.0.1.5"))

	resp := queryA(t, addr, "x.remove.test")
	if len(resp.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(resp.Answer))
	}

	tbl.RemoveWildcard("remove.test")

	resp = queryA(t, addr, "x.remove.test")
	for _, ans := range resp.Answer {
		if a, ok := ans.(*mdns.A); ok && a.A.Equal(net.ParseIP("10.0.1.5")) {
			t.Fatal("got managed IP after wildcard removal")
		}
	}
}

func TestResolver_WildcardUpdate(t *testing.T) {
	_, tbl, addr := startTestResolver(t, nil)
	tbl.AddWildcard("update.test", net.ParseIP("10.0.2.1"))

	resp := queryA(t, addr, "a.update.test")
	a := resp.Answer[0].(*mdns.A)
	if !a.A.Equal(net.ParseIP("10.0.2.1")) {
		t.Fatalf("expected 10.0.2.1, got %s", a.A)
	}

	tbl.AddWildcard("update.test", net.ParseIP("10.0.2.2"))

	resp = queryA(t, addr, "a.update.test")
	a = resp.Answer[0].(*mdns.A)
	if !a.A.Equal(net.ParseIP("10.0.2.2")) {
		t.Fatalf("expected 10.0.2.2 after update, got %s", a.A)
	}
}

// --- Exact takes priority over wildcard ---

func TestResolver_ExactOverWildcard(t *testing.T) {
	_, tbl, addr := startTestResolver(t, nil)
	tbl.AddWildcard("example.com", net.ParseIP("10.0.1.1"))
	tbl.AddExact("specific.example.com", net.ParseIP("10.0.0.99"))

	resp := queryA(t, addr, "specific.example.com")
	if len(resp.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(resp.Answer))
	}
	a := resp.Answer[0].(*mdns.A)
	if !a.A.Equal(net.ParseIP("10.0.0.99")) {
		t.Fatalf("exact should take priority, got %s", a.A)
	}
}

// --- AAAA queries for managed hostnames → NODATA ---

func TestResolver_AAAAManagedReturnsNoData(t *testing.T) {
	_, tbl, addr := startTestResolver(t, nil)
	tbl.AddExact("v4only.test.com", net.ParseIP("10.0.0.1"))

	resp := queryAAAA(t, addr, "v4only.test.com")
	if len(resp.Answer) != 0 {
		t.Fatalf("expected NODATA for AAAA on managed host, got %d answers", len(resp.Answer))
	}
	if resp.Rcode != mdns.RcodeSuccess {
		t.Fatalf("expected NOERROR rcode for NODATA, got %d", resp.Rcode)
	}
}

func TestResolver_AAAAWildcardReturnsNoData(t *testing.T) {
	_, tbl, addr := startTestResolver(t, nil)
	tbl.AddWildcard("wild.test", net.ParseIP("10.0.1.1"))

	resp := queryAAAA(t, addr, "foo.wild.test")
	if len(resp.Answer) != 0 {
		t.Fatalf("expected NODATA for AAAA on wildcard host, got %d answers", len(resp.Answer))
	}
}

// --- Upstream forwarding ---

func TestResolver_ForwardsUnmanaged(t *testing.T) {
	_, tbl, addr := startTestResolver(t, []string{"8.8.8.8:53"})
	tbl.AddExact("managed.test", net.ParseIP("10.0.0.1"))

	resp := queryA(t, addr, "google.com")
	if resp.Rcode != mdns.RcodeSuccess {
		t.Fatalf("expected success for upstream query, got rcode %d", resp.Rcode)
	}
	if len(resp.Answer) == 0 {
		t.Fatal("expected at least 1 answer from upstream for google.com")
	}
}

// --- TCP support ---

func TestResolver_TCPQuery(t *testing.T) {
	_, tbl, addr := startTestResolver(t, nil)
	tbl.AddExact("tcp.test.com", net.ParseIP("10.0.0.7"))

	resp := queryTCP(t, addr, "tcp.test.com")
	if len(resp.Answer) != 1 {
		t.Fatalf("expected 1 answer via TCP, got %d", len(resp.Answer))
	}
	a := resp.Answer[0].(*mdns.A)
	if !a.A.Equal(net.ParseIP("10.0.0.7")) {
		t.Fatalf("expected 10.0.0.7, got %s", a.A)
	}
}

// --- ClearAll ---

func TestResolver_ClearAllRecords(t *testing.T) {
	upstreamAddr := startFakeUpstream(t)

	_, tbl, addr := startTestResolver(t, []string{upstreamAddr})
	tbl.AddExact("a.test", net.ParseIP("10.0.0.1"))
	tbl.AddWildcard("b.test", net.ParseIP("10.0.0.2"))

	resp := queryA(t, addr, "a.test")
	if len(resp.Answer) != 1 {
		t.Fatal("expected exact match before clear")
	}
	resp = queryA(t, addr, "x.b.test")
	if len(resp.Answer) != 1 {
		t.Fatal("expected wildcard match before clear")
	}

	tbl.ClearAll()

	resp = queryA(t, addr, "a.test")
	for _, ans := range resp.Answer {
		if a, ok := ans.(*mdns.A); ok && a.A.Equal(net.ParseIP("10.0.0.1")) {
			t.Fatal("still got managed IP after clear")
		}
	}
}

// --- Concurrency ---

func TestResolver_ConcurrentAccess(t *testing.T) {
	_, tbl, addr := startTestResolver(t, nil)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			h := fmt.Sprintf("host%d.test.com", i)
			tbl.AddExact(h, net.ParseIP(fmt.Sprintf("10.0.%d.%d", i/256, i%256)))
		}
	}()

	for i := 0; i < 50; i++ {
		h := fmt.Sprintf("host%d.test.com", i)
		tbl.AddExact(h, net.ParseIP(fmt.Sprintf("10.0.%d.%d", i/256, i%256)))
	}
	for i := 0; i < 50; i++ {
		queryA(t, addr, fmt.Sprintf("host%d.test.com", i))
	}

	<-done
}

// --- Table accessor ---

func TestResolver_TableAccessor(t *testing.T) {
	tbl := routing.NewTable()
	r := NewResolver("127.0.0.1:0", nil, tbl, logr.Discard())
	if r.Table() != tbl {
		t.Fatal("Table() should return the same table passed to NewResolver")
	}
}


