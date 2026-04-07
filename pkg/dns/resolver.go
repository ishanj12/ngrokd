package dns

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/go-logr/logr"
	"github.com/ishanjain/ngrok-forward-proxy/pkg/routing"
	mdns "github.com/miekg/dns"
)

// Resolver is a lightweight DNS server that answers A queries for managed
// hostnames (exact and wildcard) and forwards everything else upstream.
type Resolver struct {
	listenAddr string
	upstream   []string
	logger     logr.Logger
	table      *routing.Table

	udpServer *mdns.Server
	tcpServer *mdns.Server
}

// NewResolver creates a new DNS resolver.
// listenAddr is the address to listen on (e.g. "127.0.0.1:53").
// upstream is a list of upstream DNS servers (e.g. ["8.8.8.8:53"]).
// table is the shared routing table for hostname→IP lookups.
func NewResolver(listenAddr string, upstream []string, table *routing.Table, logger logr.Logger) *Resolver {
	if listenAddr == "" {
		listenAddr = "127.0.0.1:53"
	}
	if len(upstream) == 0 {
		upstream = []string{"8.8.8.8:53", "1.1.1.1:53"}
	}
	if table == nil {
		table = routing.NewTable()
	}
	return &Resolver{
		listenAddr: listenAddr,
		upstream:   upstream,
		logger:     logger,
		table:      table,
	}
}

// Table returns the resolver's routing table.
func (r *Resolver) Table() *routing.Table {
	return r.table
}

// Start starts the DNS server on both UDP and TCP.
func (r *Resolver) Start(ctx context.Context) error {
	mux := mdns.NewServeMux()
	mux.HandleFunc(".", r.handleQuery)

	udpReady := make(chan struct{})
	tcpReady := make(chan struct{})

	r.udpServer = &mdns.Server{
		Addr:              r.listenAddr,
		Net:               "udp",
		Handler:           mux,
		NotifyStartedFunc: func() { close(udpReady) },
	}
	r.tcpServer = &mdns.Server{
		Addr:              r.listenAddr,
		Net:               "tcp",
		Handler:           mux,
		NotifyStartedFunc: func() { close(tcpReady) },
	}

	errCh := make(chan error, 2)
	go func() { errCh <- r.udpServer.ListenAndServe() }()
	go func() { errCh <- r.tcpServer.ListenAndServe() }()

	select {
	case err := <-errCh:
		return fmt.Errorf("dns server failed to start: %w", err)
	case <-udpReady:
	}
	select {
	case err := <-errCh:
		return fmt.Errorf("dns server failed to start: %w", err)
	case <-tcpReady:
	}

	r.logger.Info("DNS server started", "address", r.listenAddr, "upstream", r.upstream)
	return nil
}

// Stop gracefully shuts down the DNS server.
func (r *Resolver) Stop() {
	if r.udpServer != nil {
		r.udpServer.Shutdown()
	}
	if r.tcpServer != nil {
		r.tcpServer.Shutdown()
	}
	r.logger.Info("DNS server stopped")
}

func (r *Resolver) handleQuery(w mdns.ResponseWriter, req *mdns.Msg) {
	if len(req.Question) == 0 {
		mdns.HandleFailed(w, req)
		return
	}

	q := req.Question[0]

	if r.table.IsManaged(q.Name) {
		r.handleManaged(w, req, q)
		return
	}

	r.handleForward(w, req)
}

func (r *Resolver) handleManaged(w mdns.ResponseWriter, req *mdns.Msg, q mdns.Question) {
	msg := new(mdns.Msg)
	msg.SetReply(req)
	msg.Authoritative = true

	switch q.Qtype {
	case mdns.TypeA:
		ip := r.table.Lookup(q.Name)
		if ip != nil {
			msg.Answer = append(msg.Answer, &mdns.A{
				Hdr: mdns.RR_Header{
					Name:   q.Name,
					Rrtype: mdns.TypeA,
					Class:  mdns.ClassINET,
					Ttl:    5,
				},
				A: ip,
			})
		}
	case mdns.TypeAAAA:
		// NODATA: empty response, prevents IPv6 bypass
	default:
		// For other types on managed names, also return NODATA
	}

	w.WriteMsg(msg)
}

func (r *Resolver) handleForward(w mdns.ResponseWriter, req *mdns.Msg) {
	c := new(mdns.Client)
	c.Timeout = 5 * time.Second

	if _, ok := w.RemoteAddr().(*net.TCPAddr); ok {
		c.Net = "tcp"
	}

	for _, upstream := range r.upstream {
		resp, _, err := c.Exchange(req, upstream)
		if err != nil {
			r.logger.V(1).Info("upstream DNS query failed", "upstream", upstream, "error", err)
			continue
		}
		w.WriteMsg(resp)
		return
	}

	msg := new(mdns.Msg)
	msg.SetRcode(req, mdns.RcodeServerFailure)
	w.WriteMsg(msg)
}
