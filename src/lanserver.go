package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"claudedash/internal/applog"
	"claudedash/internal/discovery"
	"claudedash/internal/httpd"
	"claudedash/internal/usage"
)

// lanServer is the HTTP server the Kindle talks to. It can move to another
// port while running, for when the port is changed in the settings page.
type lanServer struct {
	watch *httpd.ClientWatch
	log   *applog.Log
	// beacon tells Kindles on the LAN where to find this server.
	beacon *discovery.Broadcaster

	mu     sync.Mutex
	server *http.Server
}

func (l *lanServer) listening() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.server != nil
}

// bind starts serving on port. The previous listener is closed only once the
// new one is up, so a port that cannot be opened leaves the old one serving.
func (l *lanServer) bind(port int) error {
	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler:           l.watch,
		ReadHeaderTimeout: 5 * time.Second,
		// A stuck handler must not hold a connection open forever.
		WriteTimeout: 60 * time.Second,
	}
	go server.Serve(listener)

	l.mu.Lock()
	previous := l.server
	l.server = server
	l.mu.Unlock()
	if previous != nil {
		previous.Close()
	}
	if l.beacon != nil {
		l.beacon.SetPort(port)
		l.beacon.Start()
	}
	l.log.Add(applog.System, "listening on port %d", port)
	return nil
}

func (l *lanServer) close() {
	if l.beacon != nil {
		l.beacon.Stop()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.server != nil {
		l.server.Close()
	}
}

func (l *lanServer) LastClient() (string, time.Time, bool) {
	return l.watch.LastClient()
}

// poller refreshes usage on a timer whose period can change while it runs.
type poller struct {
	service  *usage.Service
	interval chan time.Duration
	initial  time.Duration
}

func newPoller(service *usage.Service, every time.Duration) *poller {
	return &poller{service: service, interval: make(chan time.Duration, 1), initial: every}
}

func (p *poller) setInterval(every time.Duration) {
	// Only the latest request matters.
	select {
	case <-p.interval:
	default:
	}
	p.interval <- every
}

func (p *poller) run() {
	p.service.Refresh(context.Background())
	ticker := time.NewTicker(p.initial)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.service.Refresh(context.Background())
		case every := <-p.interval:
			ticker.Reset(every)
		}
	}
}
