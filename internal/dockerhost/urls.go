package dockerhost

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"net"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// CandidateHostURLs returns deduplicated host service base URLs to try, primary first.
func CandidateHostURLs(primary, fallbackEnv, defaultPort string) []string {
	port := servicePort(primary, defaultPort)
	seen := map[string]struct{}{}
	out := make([]string, 0, 8)
	add := func(raw string) {
		normalized := NormalizeBaseURL(raw)
		if normalized == "" {
			return
		}
		if _, ok := seen[normalized]; ok {
			return
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}

	add(primary)
	if fallbackEnv != "" {
		for _, item := range strings.Split(os.Getenv(fallbackEnv), ",") {
			add(item)
		}
	}
	add(fmt.Sprintf("http://host.docker.internal:%s", port))
	if gateway := DefaultGatewayIP(); gateway != nil {
		add(fmt.Sprintf("http://%s:%s", gateway.String(), port))
	}
	add(fmt.Sprintf("http://127.0.0.1:%s", port))
	add(fmt.Sprintf("http://localhost:%s", port))
	return out
}

// NormalizeBaseURL trims, adds a scheme, and strips trailing dots from the host.
func NormalizeBaseURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return strings.TrimRight(value, "/")
	}
	parsed.Host = normalizeHost(parsed.Host)
	if parsed.Host == "" {
		return ""
	}
	return strings.TrimRight(parsed.String(), "/")
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return host
	}
	if hostname, port, err := net.SplitHostPort(host); err == nil {
		return net.JoinHostPort(strings.TrimSuffix(hostname, "."), port)
	}
	return strings.TrimSuffix(host, ".")
}

func servicePort(primary, defaultPort string) string {
	normalized := NormalizeBaseURL(primary)
	if normalized == "" {
		return defaultPort
	}
	parsed, err := url.Parse(normalized)
	if err != nil || parsed.Host == "" {
		return defaultPort
	}
	if port := parsed.Port(); port != "" {
		return port
	}
	return defaultPort
}

// DefaultGatewayIP reads the default route gateway from /proc/net/route (Linux containers).
func DefaultGatewayIP() net.IP {
	file, err := os.Open("/proc/net/route")
	if err != nil {
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return nil
	}
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		if fields[1] != "00000000" {
			continue
		}
		return parseRouteHexIP(fields[2])
	}
	return nil
}

func parseRouteHexIP(value string) net.IP {
	value = strings.TrimSpace(value)
	if len(value) != 8 {
		return nil
	}
	raw, err := strconv.ParseUint(value, 16, 32)
	if err != nil {
		return nil
	}
	ip := make(net.IP, 4)
	binary.LittleEndian.PutUint32(ip, uint32(raw))
	return ip
}

// FirewallHint suggests UFW rules when Docker-to-host TCP probes time out on Linux.
func FirewallHint(err error, ports []int) string {
	if err == nil || runtime.GOOS != "linux" {
		return ""
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "timeout") &&
		!strings.Contains(msg, "i/o timeout") &&
		!strings.Contains(msg, "deadline exceeded") &&
		!strings.Contains(msg, "context canceled") {
		return ""
	}
	portList := make([]string, 0, len(ports))
	for _, port := range ports {
		if port > 0 {
			portList = append(portList, strconv.Itoa(port))
		}
	}
	if len(portList) == 0 {
		return "On Arch/Linux with UFW enabled, Docker may be blocked from reaching host services. Run: scripts/ufw-docker-host.sh"
	}
	return fmt.Sprintf(
		"On Arch/Linux with UFW enabled, Docker is often blocked from host ports %s until you run: scripts/ufw-docker-host.sh (or: sudo ufw allow from 172.16.0.0/12 to any port %s proto tcp)",
		strings.Join(portList, ","),
		strings.Join(portList, ","),
	)
}
