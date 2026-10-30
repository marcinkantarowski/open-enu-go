package internal

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/open-enu/kernel/i18n"
)

// A tenant names the URL and the worker posts to it from inside the
// platform's network: without the rules below, a webhook is a request to
// the database, the metadata service or the admin port of anything on the
// compose network, made by us, with the answer written to a log the tenant
// reads. Three layers, because each one alone has a way around it:
//
//   - the URL, at registration and again before every attempt: https, no
//     `localhost`, no IP literal outside the public internet, no port of a
//     well-known internal service;
//   - the dial: the host is resolved ONCE, every address it resolves to must
//     be public, and the connection goes to that address - a DNS answer that
//     changes between the check and the connect (rebinding) changes nothing;
//   - no redirect is followed: a 302 to http://169.254.169.254/ would
//     otherwise skip both checks above.
//
// WEBHOOK_ALLOW_PRIVATE=true lifts the address rules (never https or the
// redirect rule) for the dev stack, whose echo receiver has a private
// address on the compose network. It defaults to false and production must
// never set it.

const keyAddressRefused i18n.Key = "webhook.error.address_refused"

var (
	errAddressRefused  = errors.New("webhook: the endpoint's address is not a public one")
	errRedirectRefused = errors.New("webhook: the receiver answered with a redirect, which is never followed")
)

// internalPorts are services that answer on a private network and have no
// business receiving a webhook: an endpoint naming one is a probe.
var internalPorts = map[string]bool{
	"22": true, "23": true, "25": true, "53": true, "110": true, "143": true, "389": true, "2375": true, "2376": true,
	"2379": true, "3306": true, "5432": true, "5672": true, "6379": true, "6443": true, "9200": true, "11211": true, "27017": true,
}

// nonPublic are the ranges netip's predicates do not already name.
var nonPublic = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"), // carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("240.0.0.0/4"),  // reserved, and the broadcast address
	netip.MustParsePrefix("64:ff9b::/96"), // NAT64: an IPv4 address in disguise
	netip.MustParsePrefix("2002::/16"),    // 6to4: likewise
}

// publicAddr is an address on the public internet.
func publicAddr(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsValid() || a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() ||
		a.IsInterfaceLocalMulticast() || a.IsMulticast() || a.IsUnspecified() {
		return false
	}
	for _, p := range nonPublic {
		if p.Contains(a) {
			return false
		}
	}
	return true
}

// outbound is the policy and what the client is built from. A test sets
// resolve (a hostname that resolves where the test says) and tls (trust in
// its own receiver); the process sets only allowPrivate.
type outbound struct {
	allowPrivate bool
	resolve      func(ctx context.Context, host string) ([]netip.Addr, error)
	tls          *tls.Config
}

// checkURL is the rule a URL is held to before it is stored and before
// every attempt. It looks at the URL only; the dial checks what it resolves to.
func (o outbound) checkURL(raw string) i18n.Key {
	u, err := url.Parse(raw)
	// HTTPS only. The payload is signed, not encrypted, so plain HTTP
	// publishes every event to anyone on the path.
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return keyHTTPSRequired
	}
	if internalPorts[u.Port()] {
		return keyAddressRefused
	}
	if o.allowPrivate {
		return ""
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return keyAddressRefused
	}
	if a, err := netip.ParseAddr(host); err == nil && !publicAddr(a) {
		return keyAddressRefused
	}
	return ""
}

// client is the deliverer's: a transport that dials only what the policy
// allows, through no proxy, and follows no redirect.
func (o outbound) client() *http.Client {
	resolve := o.resolve
	if resolve == nil {
		resolve = func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		}
	}
	dialer := &net.Dialer{Timeout: requestTimeout}
	transport := &http.Transport{
		Proxy:               nil, // a proxy would dial for us, past the check below
		TLSClientConfig:     o.tls,
		TLSHandshakeTimeout: requestTimeout,
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		ForceAttemptHTTP2:   true,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			var addrs []netip.Addr
			if a, err := netip.ParseAddr(host); err == nil {
				addrs = []netip.Addr{a}
			} else if addrs, err = resolve(ctx, host); err != nil {
				return nil, err
			}
			if len(addrs) == 0 {
				return nil, errAddressRefused
			}
			// Every answer must be public, not just the first: a name that
			// resolves to one public and one private address is a probe.
			for _, a := range addrs {
				if !o.allowPrivate && !publicAddr(a) {
					return nil, errAddressRefused
				}
			}
			// The address checked is the address dialled.
			return dialer.DialContext(ctx, network, net.JoinHostPort(addrs[0].Unmap().String(), port))
		},
	}
	return &http.Client{
		Timeout:   requestTimeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			return fmt.Errorf("%w (to host %s)", errRedirectRefused, req.URL.Hostname())
		},
	}
}
