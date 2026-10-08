package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

const maxBody = 8 << 20

var ErrSource = errors.New("source could not be verified")

// Configured hosts are a fixed adapter allowlist, not a URL supplied by a page.
// Redirects are forbidden, including redirects to login and challenge pages.
func NewClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 10 * time.Second
	transport.MaxConnsPerHost = 2
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		// A configured proxy is an infrastructure endpoint; it resolves upstreams.
		proxy := false
		for _, scheme := range []string{"https", "http"} {
			p, _ := http.ProxyFromEnvironment(&http.Request{URL: &url.URL{Scheme: scheme, Host: "stepik.org"}})
			if p != nil && host == p.Hostname() {
				proxy = true
			}
		}
		if proxy {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, address)
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if !publicIP(ip.IP) {
				return nil, ErrSource
			}
		}
		for _, ip := range ips {
			conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), "443"))
			if err == nil {
				return conn, nil
			}
		}
		return nil, ErrSource
	}
	return &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func publicIP(ip net.IP) bool {
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified()
}

func fetch(ctx context.Context, client *http.Client, raw string) ([]byte, string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || !allowedSourceHost(u.Host) || u.User != nil {
		return nil, "", ErrSource
	}
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
		if err != nil {
			return nil, "", ErrSource
		}
		req.Header.Set("User-Agent", "DevCourseFinder-CatalogUpdater/1.0 (twice-daily official course checks)")
		req.Header.Set("Accept", "application/json, text/html;q=0.9")
		response, err := client.Do(req)
		if err != nil {
			return nil, "", ErrSource
		}
		data, readErr := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
		response.Body.Close()
		if readErr != nil || len(data) > maxBody {
			return nil, "", ErrSource
		}
		if response.StatusCode == http.StatusOK {
			digest := sha256.Sum256(data)
			return data, hex.EncodeToString(digest[:]), nil
		}
		if attempt == 0 && (response.StatusCode == 429 || response.StatusCode >= 500) {
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, "", ErrSource
			case <-timer.C:
				continue
			}
		}
		return nil, "", ErrSource
	}
	return nil, "", ErrSource
}

func allowedSourceHost(host string) bool {
	for _, allowed := range providerHosts {
		if host == allowed {
			return true
		}
	}
	return false
}
