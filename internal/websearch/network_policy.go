package websearch

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var forbiddenOutboundPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:2::/48"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2001:10::/28"),
	netip.MustParsePrefix("2001:20::/28"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("fec0::/10"),
}

type outboundPolicy struct {
	resolver HostResolver
	dialer   net.Dialer
}

func newOutboundPolicy(resolver HostResolver, timeout time.Duration) outboundPolicy {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return outboundPolicy{resolver: resolver, dialer: net.Dialer{Timeout: timeout}}
}

func (policy outboundPolicy) validateURL(ctx context.Context, rawURL string) error {
	if ctx == nil {
		return ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	parsed, err := parseOutboundURL(rawURL)
	if err != nil {
		return err
	}
	_, err = policy.resolve(ctx, parsed.Hostname())
	return err
}

func parseOutboundURL(rawURL string) (*url.URL, error) {
	if rawURL == "" || rawURL != strings.TrimSpace(rawURL) || len(rawURL) > maxURLBytes {
		return nil, fmt.Errorf("%w: URL must contain 1..%d exact bytes", ErrUnsafeURL, maxURLBytes)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%w: parse URL: %v", ErrUnsafeURL, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("%w: URL requires http or https", ErrUnsafeURL)
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("%w: URL credentials are forbidden", ErrUnsafeURL)
	}
	if parsed.Fragment != "" {
		return nil, fmt.Errorf("%w: URL fragments are forbidden", ErrUnsafeURL)
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" {
		return nil, fmt.Errorf("%w: URL host is empty", ErrUnsafeURL)
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return nil, fmt.Errorf("%w: localhost is forbidden", ErrUnsafeURL)
	}
	if literal, err := netip.ParseAddr(host); err == nil {
		if err := validateOutboundIP(literal); err != nil {
			return nil, err
		}
	}
	return parsed, nil
}

func (policy outboundPolicy) resolve(ctx context.Context, host string) ([]net.IPAddr, error) {
	if literal, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		if err := validateOutboundIP(literal); err != nil {
			return nil, err
		}
		return []net.IPAddr{{IP: net.IP(literal.AsSlice())}}, nil
	}
	addresses, err := policy.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve outbound host %q: %w", host, err)
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("%w: host %q resolved to no addresses", ErrUnsafeURL, host)
	}
	for _, address := range addresses {
		parsed, ok := netip.AddrFromSlice(address.IP)
		if !ok {
			return nil, fmt.Errorf("%w: host %q returned an invalid address", ErrUnsafeURL, host)
		}
		if err := validateOutboundIP(parsed); err != nil {
			return nil, fmt.Errorf("host %q: %w", host, err)
		}
	}
	return addresses, nil
}

func validateOutboundIP(address netip.Addr) error {
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsUnspecified() ||
		address.IsLoopback() || address.IsPrivate() ||
		address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsMulticast() {
		return fmt.Errorf("%w: address %q is not an allowed public destination", ErrUnsafeURL, address)
	}
	for _, prefix := range forbiddenOutboundPrefixes {
		if prefix.Contains(address) {
			return fmt.Errorf("%w: address %q is a non-public special-purpose destination", ErrUnsafeURL, address)
		}
	}
	return nil
}

func (policy outboundPolicy) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("split outbound address %q: %w", address, err)
	}
	addresses, err := policy.resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	var failures []error
	for _, resolved := range addresses {
		ip := resolved.IP.String()
		if resolved.Zone != "" {
			ip += "%" + resolved.Zone
		}
		connection, dialErr := policy.dialer.DialContext(ctx, network, net.JoinHostPort(ip, port))
		if dialErr == nil {
			return connection, nil
		}
		failures = append(failures, dialErr)
	}
	return nil, fmt.Errorf("dial outbound host %q: %w", host, errors.Join(failures...))
}

func newSafeHTTPClient(config Config) (*http.Client, error) {
	policy := newOutboundPolicy(config.Resolver, config.Timeout)
	client := &http.Client{}
	if config.HTTPClient != nil {
		*client = *config.HTTPClient
	}
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	if transport, ok := base.(*http.Transport); ok {
		clone := transport.Clone()
		if clone.TLSClientConfig != nil && clone.TLSClientConfig.InsecureSkipVerify {
			return nil, fmt.Errorf("%w: insecure TLS verification is forbidden", ErrInvalidConfig)
		}
		clone.Proxy = nil
		clone.DialContext = policy.dialContext
		clone.DialTLS = nil
		clone.DialTLSContext = nil
		base = clone
	}
	client.Transport = policyRoundTripper{policy: policy, next: base}
	client.Timeout = config.Timeout
	client.Jar = nil
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if err := policy.validateURL(request.Context(), request.URL.String()); err != nil {
			return err
		}
		if len(via) == 0 || via[len(via)-1].URL == nil {
			return fmt.Errorf("%w: redirect has no exact source authority", ErrSearchRedirect)
		}
		return fmt.Errorf(
			"%w: %s redirects to %s", ErrSearchRedirect,
			via[len(via)-1].URL.String(), request.URL.String(),
		)
	}
	return client, nil
}

type policyRoundTripper struct {
	policy outboundPolicy
	next   http.RoundTripper
}

func (transport policyRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, fmt.Errorf("%w: HTTP request or URL is nil", ErrUnsafeURL)
	}
	if err := transport.policy.validateURL(request.Context(), request.URL.String()); err != nil {
		return nil, err
	}
	return transport.next.RoundTrip(request)
}
