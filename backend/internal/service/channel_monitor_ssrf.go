package service

import (
	"context"
	"net"
	"strings"
)

var monitorBlockedHostnames = map[string]struct{}{
	"localhost":                  {},
	"localhost.localdomain":      {},
	"metadata":                   {},
	"metadata.google.internal":   {},
	"metadata.goog":              {},
	"instance-data":              {},
	"instance-data.ec2.internal": {},
}

var monitorBlockedCIDRs = mustParseCIDRs([]string{
	"127.0.0.0/8",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"169.254.0.0/16",
	"100.64.0.0/10",
	"0.0.0.0/8",
	"::1/128",
	"fc00::/7",
	"fe80::/10",
	"::/128",
})

var monitorDialer = &net.Dialer{
	Timeout:   monitorDialTimeout,
	KeepAlive: monitorDialKeepAlive,
}

func mustParseCIDRs(cidrs []string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, raw := range cidrs {
		_, n, err := net.ParseCIDR(raw)
		if err != nil {
			panic("channel monitor invalid cidr: " + raw)
		}
		out = append(out, n)
	}
	return out
}

func isPrivateOrBlockedHostname(hostname string) bool {
	if hostname == "" {
		return true
	}
	hostname = strings.ToLower(hostname)
	if _, ok := monitorBlockedHostnames[hostname]; ok {
		return true
	}
	if ip := net.ParseIP(hostname); ip != nil {
		return isPrivateIP(ip)
	}
	return false
}

func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsPrivate() {
		return true
	}
	for _, n := range monitorBlockedCIDRs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return nil, &net.AddrError{Err: "blocked by SSRF policy", Addr: address}
		}
		return monitorDialer.DialContext(ctx, network, address)
	}
	if isPrivateOrBlockedHostname(host) {
		return nil, &net.AddrError{Err: "blocked by SSRF policy", Addr: address}
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, addr := range addrs {
		if isPrivateIP(addr.IP) {
			lastErr = &net.AddrError{Err: "blocked by SSRF policy", Addr: addr.IP.String()}
			continue
		}
		conn, err := monitorDialer.DialContext(ctx, network, net.JoinHostPort(addr.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = &net.AddrError{Err: "no usable addresses", Addr: host}
	}
	return nil, lastErr
}
