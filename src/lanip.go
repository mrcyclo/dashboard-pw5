package main

import (
	"net"
	"strings"
)

type adapter struct {
	name string
	ip   string
	// physical means the adapter has a hardware address; VPN tunnels do not.
	physical bool
}

var virtualNames = []string{"vmware", "vethernet", "virtual", "hyper-v", "loopback", "docker", "wsl"}

// pickLAN chooses the address the Kindle should use to reach this PC. Asking
// the OS for its outbound address is not enough: with a VPN or proxy tunnel
// active that is the tunnel's address, which the Kindle cannot reach.
func pickLAN(adapters []adapter, kindleIP string) string {
	usable := make([]adapter, 0, len(adapters))
	for _, a := range adapters {
		ip := net.ParseIP(a.ip).To4()
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		usable = append(usable, a)
	}

	// An adapter on the same /24 as a Kindle that has already called in is certain.
	if kindle := net.ParseIP(kindleIP).To4(); kindle != nil {
		for _, a := range usable {
			ip := net.ParseIP(a.ip).To4()
			if ip[0] == kindle[0] && ip[1] == kindle[1] && ip[2] == kindle[2] {
				return a.ip
			}
		}
	}
	for _, a := range usable {
		if a.physical && !isVirtual(a.name) {
			return a.ip
		}
	}
	for _, a := range usable {
		if a.physical {
			return a.ip
		}
	}
	if len(usable) > 0 {
		return usable[0].ip
	}
	return "127.0.0.1"
}

func isVirtual(name string) bool {
	lower := strings.ToLower(name)
	for _, marker := range virtualNames {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func listAdapters() []adapter {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []adapter
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil {
				continue
			}
			out = append(out, adapter{name: iface.Name, ip: ipNet.IP.String(), physical: len(iface.HardwareAddr) > 0})
		}
	}
	return out
}
