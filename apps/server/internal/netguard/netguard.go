// Package netguard rejects destinations that would reach link-local or
// cloud metadata addresses. Ordinary private addresses stay allowed so a
// self-hosted MinIO or an internal webhook still works.
package netguard

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"
)

var ErrBlocked = errors.New("that address is not allowed")

// UnsafeIP is true for link-local, multicast, unspecified, and well-known
// metadata addresses.
func UnsafeIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return true
	}
	if ip.Equal(net.ParseIP("fd00:ec2::254")) || ip.Equal(net.ParseIP("100.100.100.200")) {
		return true
	}
	return false
}

func metadataHost(host string) bool {
	switch strings.ToLower(strings.TrimSuffix(host, ".")) {
	case "metadata.google.internal", "metadata.google.com", "instance-data":
		return true
	default:
		return false
	}
}

// CheckURL rejects a URL whose host is a blocked address or currently resolves
// to one. A name that does not resolve yet is left for DialContext.
func CheckURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return errors.New("url must be an http or https URL without a username")
	}
	host := parsed.Hostname()
	if metadataHost(host) {
		return ErrBlocked
	}
	if ip := net.ParseIP(host); ip != nil {
		if UnsafeIP(ip) {
			return ErrBlocked
		}
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil
	}
	for _, ip := range ips {
		if UnsafeIP(ip) {
			return ErrBlocked
		}
	}
	return nil
}

// DialContext connects only to addresses that pass UnsafeIP.
func DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if metadataHost(host) {
		return nil, ErrBlocked
	}
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		resolved, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		for _, item := range resolved {
			ips = append(ips, item.IP)
		}
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var dialErr error = ErrBlocked
	for _, ip := range ips {
		if UnsafeIP(ip) {
			continue
		}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		dialErr = err
	}
	return nil, dialErr
}
