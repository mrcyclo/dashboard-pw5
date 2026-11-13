package discovery

import (
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var sample = Beacon{App: "claudedash", Proto: 1, ID: "0123456789abcdef", Name: "desk-pc", Port: 8377}

func TestRoundTrip(t *testing.T) {
	got, ok := Decode(Encode(sample))
	if !ok || got != sample {
		t.Fatalf("%+v %v", got, ok)
	}
}

func TestDecodeRejects(t *testing.T) {
	bad := []string{
		`{"app":"other","proto":1,"id":"0123456789abcdef","name":"x","port":8377}`,
		`{"app":"claudedash","proto":2,"id":"0123456789abcdef","name":"x","port":8377}`,
		`{"app":"claudedash","proto":1,"id":"xyz","name":"x","port":8377}`,
		`{"app":"claudedash","proto":1,"id":"0123456789abcdef","name":"x","port":0}`,
		`{"app":"claudedash","proto":1,"id":"0123456789abcdef","name":"x","port":70000}`,
		`garbage`,
		``,
		`{"app":"claudedash","proto":1,"id":"0123456789abcdef","name":"` + strings.Repeat("x", 2000) + `","port":8377}`,
	}
	for _, b := range bad {
		if _, ok := Decode([]byte(b)); ok {
			t.Errorf("accepted %.60s", b)
		}
	}
}

func seen(id, ip string, at time.Time) Seen {
	b := sample
	b.ID = id
	return Seen{Beacon: b, Addr: net.ParseIP(ip), At: at}
}

var now = time.Unix(1000, 0)

func TestChooserPrefersTheRememberedPC(t *testing.T) {
	c := NewChooser("aaaaaaaaaaaaaaaa")
	c.Hear(seen("bbbbbbbbbbbbbbbb", "10.0.0.2", now))
	if _, ok := c.Current(now); ok {
		t.Fatal("another PC must not be picked while one is remembered")
	}
	c.Hear(seen("aaaaaaaaaaaaaaaa", "10.0.0.1", now))
	if cur, ok := c.Current(now); !ok || cur.Addr.String() != "10.0.0.1" {
		t.Fatalf("%+v %v", cur, ok)
	}
	c.Hear(seen("aaaaaaaaaaaaaaaa", "10.0.0.9", now.Add(time.Second)))
	if cur, _ := c.Current(now.Add(time.Second)); cur.Addr.String() != "10.0.0.9" {
		t.Fatal("a new address of the same PC is followed")
	}
}

func TestChooserTakesTheFirstWhenNoneRemembered(t *testing.T) {
	c := NewChooser("")
	c.Hear(seen("bbbbbbbbbbbbbbbb", "10.0.0.2", now))
	c.Hear(seen("cccccccccccccccc", "10.0.0.3", now))
	cur, ok := c.Current(now)
	if !ok || cur.ID != "bbbbbbbbbbbbbbbb" || c.Remembered() != "bbbbbbbbbbbbbbbb" {
		t.Fatalf("%+v %v %s", cur, ok, c.Remembered())
	}
}

func TestOthersAndChoose(t *testing.T) {
	c := NewChooser("aaaaaaaaaaaaaaaa")
	c.Hear(seen("aaaaaaaaaaaaaaaa", "10.0.0.1", now.Add(-20*time.Second)))
	c.Hear(seen("bbbbbbbbbbbbbbbb", "10.0.0.2", now))
	c.Hear(seen("cccccccccccccccc", "10.0.0.3", now.Add(-20*time.Second)))
	others := c.Others(now)
	if len(others) != 1 || others[0].ID != "bbbbbbbbbbbbbbbb" {
		t.Fatalf("only PCs heard in the last 10 s: %+v", others)
	}
	if _, ok := c.Current(now); ok {
		t.Fatal("a PC silent for 20 s is not current")
	}
	c.Choose("bbbbbbbbbbbbbbbb")
	if cur, ok := c.Current(now); !ok || cur.ID != "bbbbbbbbbbbbbbbb" || c.Remembered() != "bbbbbbbbbbbbbbbb" {
		t.Fatal("choose switches")
	}
}

func TestBroadcasterSendsToEveryTarget(t *testing.T) {
	a, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	port := a.LocalAddr().(*net.UDPAddr).Port

	b := NewBroadcaster(sample, 50*time.Millisecond, func() []net.IP { return []net.IP{net.ParseIP("127.0.0.1")} })
	b.target = port
	b.Start()
	defer b.Stop()

	a.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1024)
	n, _, err := a.ReadFrom(buf)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := Decode(buf[:n]); !ok || got.ID != sample.ID {
		t.Fatalf("%s", buf[:n])
	}
	b.SetPort(9000)
	a.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		n, _, err := a.ReadFrom(buf)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := Decode(buf[:n]); got.Port == 9000 {
			break
		}
	}
	b.Stop()
}

func TestListenForwardsOnlyBeacons(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	out := make(chan Seen, 4)
	go Listen(conn, out)
	sender, _ := net.Dial("udp4", conn.LocalAddr().String())
	sender.Write([]byte("noise"))
	sender.Write(Encode(sample))
	select {
	case s := <-out:
		if s.ID != sample.ID || s.Addr.String() != "127.0.0.1" {
			t.Fatalf("%+v", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no beacon")
	}
	conn.Close()
}

func TestBroadcastTargetsAreIPv4(t *testing.T) {
	for _, ip := range BroadcastTargets() {
		if ip.To4() == nil {
			t.Errorf("%v", ip)
		}
	}
}

func TestBroadcasterRetriesWhenItCannotOpenASocket(t *testing.T) {
	a, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	b := NewBroadcaster(sample, 20*time.Millisecond, func() []net.IP { return []net.IP{net.ParseIP("127.0.0.1")} })
	b.target = a.LocalAddr().(*net.UDPAddr).Port
	failures := 0
	b.listen = func() (net.PacketConn, error) {
		if failures < 2 {
			failures++
			return nil, errors.New("network is down")
		}
		return net.ListenPacket("udp4", ":0")
	}
	var logged atomic.Int32
	b.Log = func(string, ...any) { logged.Add(1) }
	b.Start()
	defer b.Stop()

	a.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1024)
	if _, _, err := a.ReadFrom(buf); err != nil {
		t.Fatalf("the broadcaster must keep trying: %v", err)
	}
	if logged.Load() == 0 {
		t.Fatal("the failure is logged")
	}
}
