// Package vpn provides a WireGuard-based VPN tunnel for selectively routing
// HTTP requests through a WireGuard interface. This is used to bypass age
// verification walls on sites like MetArt, Playboy, Femjoy, etc.
//
// The package uses github.com/botanica-consulting/wiredialer which creates a
// userspace WireGuard tunnel — no OS-level VPN configuration is required.
package vpn

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/botanica-consulting/wiredialer"
	"github.com/carlj/godownload/internal/config"
)

// defaultBlockedDomains is the fallback list of domains that require VPN
// routing when the config does not specify any.
var defaultBlockedDomains = []string{
	"metart.com",
	"met-art.com",
	"sexart.com",
	"thelifeerotic.com",
	"eternaldesire.com",
	"playboy.com",
	"femjoy.com",
	"mplstudios.com",
}

// Service manages a singleton WireGuard dialer and provides VPN-aware HTTP
// clients. It is safe for concurrent use.
type Service struct {
	cfg            config.WireGuardConfig
	blockedDomains []string

	dialerOnce sync.Once
	dialer     *wiredialer.WireDialer
	dialerErr  error

	// directClient is a connection-pooled client for non-VPN requests.
	directClient *http.Client
}

// New creates a VPN Service from the given config.
// If cfg.ConfPath is empty, the service operates in pass-through mode (all
// requests use the direct client).
func New(cfg config.WireGuardConfig, directClient *http.Client) *Service {
	blocked := cfg.BlockedDomains
	if len(blocked) == 0 {
		blocked = defaultBlockedDomains
	}

	return &Service{
		cfg:            cfg,
		blockedDomains: blocked,
		directClient:   directClient,
	}
}

// initDialer lazily initialises the WireGuard dialer on first use.
func (s *Service) initDialer() (*wiredialer.WireDialer, error) {
	s.dialerOnce.Do(func() {
		confPath := s.cfg.ConfPath
		if confPath == "" {
			confPath = "wireguard.conf" // check default location
		}

		if _, err := os.Stat(confPath); os.IsNotExist(err) {
			slog.Info("vpn: WireGuard config not found, requests will use direct connection",
				"path", confPath)
			s.dialerErr = fmt.Errorf("wireguard config not found at %s", confPath)
			return
		}

		slog.Info("vpn: loading WireGuard configuration", "path", confPath)

		dialer, err := wiredialer.NewDialerFromFile(confPath)
		if err != nil {
			s.dialerErr = fmt.Errorf("vpn: creating WireGuard dialer: %w", err)
			slog.Error("vpn: failed to create WireGuard dialer", "error", err)
			return
		}

		s.dialer = dialer
		slog.Info("vpn: WireGuard tunnel established successfully")
	})

	return s.dialer, s.dialerErr
}

// ShouldRoute returns true if the given URL should be routed through the VPN.
func (s *Service) ShouldRoute(targetURL string) bool {
	lower := strings.ToLower(targetURL)
	for _, domain := range s.blockedDomains {
		if strings.Contains(lower, domain) {
			return true
		}
	}
	return false
}

// GetHTTPClient returns an HTTP client appropriate for the target URL.
// If the URL matches a blocked domain and VPN is configured, a VPN-routed
// client is returned. Otherwise the shared direct client is used.
func (s *Service) GetHTTPClient(targetURL string) *http.Client {
	// Bypass mode — always use direct client.
	if s.cfg.Bypass {
		slog.Debug("vpn: bypass enabled, using direct connection", "url", targetURL)
		return s.directClient
	}

	// Check BYPASS_VPN env var for runtime overrides.
	if os.Getenv("BYPASS_VPN") == "true" {
		slog.Debug("vpn: BYPASS_VPN=true, using direct connection", "url", targetURL)
		return s.directClient
	}

	if !s.ShouldRoute(targetURL) {
		return s.directClient
	}

	slog.Info("vpn: URL requires WireGuard tunnel", "url", targetURL)

	dialer, err := s.initDialer()
	if err != nil {
		slog.Warn("vpn: WireGuard not available, falling back to direct connection",
			"error", err, "url", targetURL)
		return s.directClient
	}

	slog.Info("vpn: using WireGuard tunnel", "url", targetURL)

	// Return a new client per VPN request. VPN connections should NOT be
	// pooled with direct connections — the dialer routes through a
	// completely different network path.
	return &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			// Force IPv4 to prevent IPv6 leaks that bypass the tunnel.
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.DialContext(ctx, "tcp4", addr)
			},
		},
		// Disable automatic redirect following so redirects also go through
		// our VPN transport.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
