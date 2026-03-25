package httputil

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
)

// ResolveAndValidateURL resolves a URL's hostname and validates it doesn't
// target internal/private networks (SSRF protection). Returns the first safe
// resolved IP, or an error if unsafe or unresolvable.
func ResolveAndValidateURL(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL")
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("URL scheme must be http or https")
	}

	hostname := parsed.Hostname()
	if hostname == "" {
		return "", fmt.Errorf("URL missing hostname")
	}

	port := parsed.Port()
	if port == "" {
		port = "80"
	}
	portNum, err := strconv.Atoi(port)
	if err != nil || portNum < 1 || portNum > 65535 {
		return "", fmt.Errorf("invalid port")
	}

	addrs, err := net.LookupHost(hostname)
	if err != nil {
		return "", fmt.Errorf("cannot resolve hostname: %w", err)
	}

	// Check ALL resolved IPs — if any are blocked, reject
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			return "", fmt.Errorf("invalid IP address resolved")
		}
		if IsIPBlocked(ip) {
			return "", fmt.Errorf("remote server URL resolves to a private or reserved IP address")
		}
	}

	if len(addrs) == 0 {
		return "", fmt.Errorf("no addresses resolved for hostname")
	}

	return addrs[0], nil
}

// IsIPBlocked checks if an IP address is private, reserved, or otherwise unsafe for SSRF.
func IsIPBlocked(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}

	// CGNAT range (100.64.0.0/10)
	_, cgnat, _ := net.ParseCIDR("100.64.0.0/10")
	if cgnat.Contains(ip) {
		return true
	}

	// Documentation ranges
	for _, cidr := range []string{"192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24"} {
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(ip) {
			return true
		}
	}

	return false
}
