package httpd

import (
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	// Longer than the dashboard's slowest regular polling, which backs off to 30 seconds.
	silenceThreshold = 45 * time.Second

	maxTrackedClients = 64
	maxLoggedPath     = 80
)

type lastSeen struct {
	at      time.Time
	request string
}

type ClientWatch struct {
	next http.Handler
	now  func() time.Time
	logf func(format string, args ...any)

	mu      sync.Mutex
	clients map[string]lastSeen
}

// WatchClients logs when a LAN client first appears and when it returns after
// going quiet. The dashboard polls every few seconds, so a gap pins down when
// and after which request a device froze, even if the device kept no log.
func WatchClients(next http.Handler, now func() time.Time, logf func(format string, args ...any)) *ClientWatch {
	return &ClientWatch{next: next, now: now, logf: logf, clients: map[string]lastSeen{}}
}

func (c *ClientWatch) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.note(r)
	c.next.ServeHTTP(w, r)
}

func (c *ClientWatch) note(r *http.Request) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return
	}
	if ip := net.ParseIP(host); ip == nil || ip.IsLoopback() {
		return
	}
	now := c.now()
	request := r.Method + " " + printable(r.URL.Path)

	c.mu.Lock()
	previous, known := c.clients[host]
	if !known && len(c.clients) >= maxTrackedClients {
		c.clients = map[string]lastSeen{}
	}
	c.clients[host] = lastSeen{at: now, request: request}
	c.mu.Unlock()

	if !known {
		c.logf("client %s connected", host)
		return
	}
	gap := now.Sub(previous.at)
	if gap < silenceThreshold {
		return
	}
	c.logf(
		"client %s back after %s of silence, last seen %s with %s",
		host,
		gap.Round(time.Second),
		previous.at.Format("15:04:05"),
		previous.request,
	)
}

// LastClient reports the LAN device heard from most recently.
func (c *ClientWatch) LastClient() (ip string, at time.Time, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for host, seen := range c.clients {
		if !ok || seen.at.After(at) {
			ip, at, ok = host, seen.at, true
		}
	}
	return ip, at, ok
}

// printable keeps a path that came from the network from forging log lines or flooding the log.
func printable(path string) string {
	if len(path) > maxLoggedPath {
		path = path[:maxLoggedPath]
	}
	clean := make([]rune, 0, len(path))
	for _, r := range path {
		if r < 0x20 || r == 0x7f {
			r = '?'
		}
		clean = append(clean, r)
	}
	return string(clean)
}
