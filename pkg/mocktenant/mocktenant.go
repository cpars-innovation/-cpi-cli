// Package mocktenant serves an in-memory SAP CPI tenant for tests and local
// development of tools built on cpicli: the mock behind `cpictl
// mock-tenant`, with the demo landscape, your own landscapes (directory with
// landscape.yaml and content), flow execution and the /_mock admin API.
// It never talks to a real tenant. See docs/mock-tenant.md.
package mocktenant

import (
	"crypto/tls"
	"net"
	"time"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
)

type (
	// Tenant is a running mock tenant.
	Tenant = cpitest.Tenant
	// Landscape is a landscape loaded from landscape.yaml and its content.
	Landscape = cpitest.Landscape
	// TierSpec is what a tier of a landscape changes.
	TierSpec = cpitest.TierSpec
	// SystemSpec is the behaviour of a receiver system.
	SystemSpec = cpitest.SystemSpec
	// MessageLog is a message processing log of the mock.
	MessageLog = cpitest.MessageLog
	// Artifact is the mock's state of one artifact.
	Artifact = cpitest.Artifact
	// Package is an integration package of the mock.
	Package = cpitest.Package
)

// DemoTiers are the tiers of the demo landscape.
var DemoTiers = cpitest.DemoTiers

// Serve starts a live mock on a listener the caller opened (optionally TLS).
// It accepts Basic Auth with CSRF and OAuth client credentials (any client).
func Serve(l net.Listener, config *tls.Config) *Tenant {
	m := cpitest.Serve(l, config)
	live(m)
	return m
}

// Start starts a live mock on a free loopback port (http). Close stops it.
func Start() *Tenant {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err) // as httptest.NewServer
	}
	return Serve(l, nil)
}

func live(m *Tenant) { m.Live, m.OAuth, m.FilterMessageLogs = true, true, true }

// SeedDemo loads one tier (dev, test, prod) of the demo landscape.
func SeedDemo(m *Tenant, tier string, now time.Time) error { return cpitest.SeedDemo(m, tier, now) }

// LoadLandscapeDir reads a landscape directory.
func LoadLandscapeDir(dir string) (*Landscape, error) { return cpitest.LoadLandscapeDir(dir) }

// SeedLandscape loads one tier of a landscape.
func SeedLandscape(m *Tenant, l *Landscape, tier string, now time.Time) error {
	return cpitest.SeedLandscape(m, l, tier, now)
}

// SeedDir loads one tier of a landscape directory.
func SeedDir(m *Tenant, dir, tier string, now time.Time) error {
	return cpitest.SeedDir(m, dir, tier, now)
}
