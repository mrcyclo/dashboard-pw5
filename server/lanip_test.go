package main

import "testing"

var adapters = []adapter{
	{name: "Luna Proxy", ip: "172.19.0.1", physical: false},
	{name: "VMware Network Adapter VMnet8", ip: "192.168.166.1", physical: true},
	{name: "vEthernet (Default Switch)", ip: "172.24.32.1", physical: true},
	{name: "Ethernet", ip: "10.0.0.225", physical: true},
}

func TestPickLANPrefersTheKindlesNetwork(t *testing.T) {
	if got := pickLAN(adapters, "10.0.0.114"); got != "10.0.0.225" {
		t.Fatalf("got %q", got)
	}
	// Whatever the adapter is called, sharing a network with the Kindle settles it.
	odd := []adapter{
		{name: "Ethernet", ip: "10.0.0.225", physical: true},
		{name: "VMware Network Adapter VMnet8", ip: "192.168.166.1", physical: true},
	}
	if got := pickLAN(odd, "192.168.166.50"); got != "192.168.166.1" {
		t.Fatalf("got %q", got)
	}
}

func TestPickLANWithoutAKindleSkipsVirtualAdapters(t *testing.T) {
	if got := pickLAN(adapters, ""); got != "10.0.0.225" {
		t.Fatalf("got %q", got)
	}
}

func TestPickLANIgnoresAKindleOnNoKnownNetwork(t *testing.T) {
	if got := pickLAN(adapters, "203.0.113.9"); got != "10.0.0.225" {
		t.Fatalf("got %q", got)
	}
}

func TestPickLANFallsBack(t *testing.T) {
	onlyVirtual := []adapter{
		{name: "Luna Proxy", ip: "172.19.0.1", physical: false},
		{name: "VMware Network Adapter VMnet1", ip: "192.168.10.1", physical: true},
	}
	if got := pickLAN(onlyVirtual, ""); got != "192.168.10.1" {
		t.Fatalf("a virtual adapter with a hardware address beats a tunnel, got %q", got)
	}
	if got := pickLAN([]adapter{{name: "Luna Proxy", ip: "172.19.0.1"}}, ""); got != "172.19.0.1" {
		t.Fatalf("got %q", got)
	}
	if got := pickLAN(nil, ""); got != "127.0.0.1" {
		t.Fatalf("got %q", got)
	}
}

func TestPickLANSkipsLinkLocal(t *testing.T) {
	list := []adapter{
		{name: "Wi-Fi", ip: "169.254.12.7", physical: true},
		{name: "Ethernet 2", ip: "192.168.1.20", physical: true},
	}
	if got := pickLAN(list, ""); got != "192.168.1.20" {
		t.Fatalf("got %q", got)
	}
}
