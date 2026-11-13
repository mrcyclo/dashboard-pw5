// Package discovery lets the Kindle find the PC on the LAN with no address to
// type: the PC broadcasts a small beacon every few seconds and the Kindle listens.
package discovery

import (
	"encoding/json"
	"net"
	"regexp"
	"sync"
	"time"
)

// Port is the UDP port beacons are sent to.
const Port = 8378

const (
	appName  = "claudedash"
	protocol = 1
	maxSize  = 1024
	// A PC not heard for this long is no longer offered or used.
	fresh = 10 * time.Second
)

type Beacon struct {
	App   string `json:"app"`
	Proto int    `json:"proto"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Port  int    `json:"port"`
}

// NewBeacon is this PC's beacon.
func NewBeacon(id, name string, port int) Beacon {
	return Beacon{App: appName, Proto: protocol, ID: id, Name: name, Port: port}
}

var hexID = regexp.MustCompile(`^[0-9a-f]{16}$`)

func Encode(b Beacon) []byte {
	data, _ := json.Marshal(b)
	return data
}

// Decode accepts only beacons of this app and protocol version.
func Decode(data []byte) (Beacon, bool) {
	var b Beacon
	if len(data) == 0 || len(data) > maxSize {
		return b, false
	}
	if err := json.Unmarshal(data, &b); err != nil {
		return Beacon{}, false
	}
	if b.App != appName || b.Proto != protocol || !hexID.MatchString(b.ID) || b.Port < 1 || b.Port > 65535 {
		return Beacon{}, false
	}
	return b, true
}

// Broadcaster sends the beacon to every target every interval until stopped.
type Broadcaster struct {
	mu      sync.Mutex
	beacon  Beacon
	every   time.Duration
	targets func() []net.IP
	target  int
	stop    chan struct{}
	listen  func() (net.PacketConn, error)
	// Log, when set, reports a socket that could not be opened.
	Log func(format string, args ...any)
}

func NewBroadcaster(b Beacon, every time.Duration, targets func() []net.IP) *Broadcaster {
	return &Broadcaster{
		beacon:  b,
		every:   every,
		targets: targets,
		target:  Port,
		listen:  func() (net.PacketConn, error) { return net.ListenPacket("udp4", ":0") },
	}
}

func (b *Broadcaster) SetPort(port int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.beacon.Port = port
}

func (b *Broadcaster) Start() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stop != nil {
		return
	}
	b.stop = make(chan struct{})
	go b.run(b.stop)
}

func (b *Broadcaster) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stop == nil {
		return
	}
	close(b.stop)
	b.stop = nil
}

func (b *Broadcaster) run(stop chan struct{}) {
	ticker := time.NewTicker(b.every)
	defer ticker.Stop()
	var conn net.PacketConn
	failing := false
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()
	for {
		// The PC may start before its network is up, so a failed socket is tried again.
		if conn == nil {
			c, err := b.listen()
			if err != nil {
				// Logged once per outage, not every tick.
				if b.Log != nil && !failing {
					b.Log("beacon: %v", err)
				}
				failing = true
				select {
				case <-stop:
					return
				case <-ticker.C:
				}
				continue
			}
			conn = c
			failing = false
		}
		b.mu.Lock()
		data, port := Encode(b.beacon), b.target
		b.mu.Unlock()
		for _, ip := range b.targets() {
			conn.WriteTo(data, &net.UDPAddr{IP: ip, Port: port})
		}
		select {
		case <-stop:
			return
		case <-ticker.C:
		}
	}
}

// BroadcastTargets lists the IPv4 broadcast address of every LAN interface that is up.
func BroadcastTargets() []net.IP {
	var out []net.IP
	interfaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			network, ok := addr.(*net.IPNet)
			if !ok || network.IP.To4() == nil || len(network.Mask) < 4 {
				continue
			}
			ip := network.IP.To4()
			broadcast := make(net.IP, 4)
			for i := range ip {
				broadcast[i] = ip[i] | ^network.Mask[len(network.Mask)-4+i]
			}
			out = append(out, broadcast)
		}
	}
	return out
}

// Seen is a beacon heard from an address.
type Seen struct {
	Beacon
	Addr net.IP
	At   time.Time
}

// Listen forwards every valid beacon read from conn until it is closed.
func Listen(conn net.PacketConn, out chan<- Seen) {
	buf := make([]byte, maxSize+1)
	for {
		n, from, err := conn.ReadFrom(buf)
		if err != nil {
			return
		}
		b, ok := Decode(buf[:n])
		addr, isUDP := from.(*net.UDPAddr)
		if !ok || !isUDP {
			continue
		}
		out <- Seen{Beacon: b, Addr: addr.IP, At: time.Now()}
	}
}

// Chooser decides which PC to use: the one used before, else the first heard.
type Chooser struct {
	remembered string
	order      []string
	heard      map[string]Seen
}

func NewChooser(remembered string) *Chooser {
	return &Chooser{remembered: remembered, heard: map[string]Seen{}}
}

func (c *Chooser) Hear(s Seen) {
	if _, ok := c.heard[s.ID]; !ok {
		c.order = append(c.order, s.ID)
	}
	c.heard[s.ID] = s
	if c.remembered == "" {
		c.remembered = s.ID
	}
}

// Current is the remembered PC, while it is still being heard.
func (c *Chooser) Current(now time.Time) (Seen, bool) {
	s, ok := c.heard[c.remembered]
	if !ok || now.Sub(s.At) > fresh {
		return Seen{}, false
	}
	return s, true
}

// Others are the other PCs heard recently, in the order first heard.
func (c *Chooser) Others(now time.Time) []Seen {
	var out []Seen
	for _, id := range c.order {
		s := c.heard[id]
		if id != c.remembered && now.Sub(s.At) <= fresh {
			out = append(out, s)
		}
	}
	return out
}

func (c *Chooser) Choose(id string) {
	c.remembered = id
}
