package cpitest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"time"
)

// DemoTiers are the tiers of the demo landscape.
var DemoTiers = []string{"dev", "test", "prod"}

// SeedDemo fills the tenant with one tier (dev, test or prod) of the
// built-in demo landscape (landscapes/demo): packages, flows with real
// models, parameters, deployments, credentials, keystore entries, Partner
// Directory parameters and a day of executed messages with failures and the
// OrderNo custom header. The tiers differ the way real ones do: dev is ahead
// (a newer version, a new flow, a draft), prod has a change made on the
// tenant and an expiring certificate, and misses a credential the new flow
// needs.
func SeedDemo(m *Tenant, tier string, now time.Time) error {
	l, err := BuiltinLandscape("demo")
	if err != nil {
		return err
	}
	return SeedLandscape(m, l, tier, now)
}

// demoCert creates a self-signed certificate that expires at notAfter.
func demoCert(alias string, notAfter time.Time) (KeystoreEntry, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return KeystoreEntry{}, err
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: alias + ".example.com", Organization: []string{"Demo"}},
		NotBefore: notAfter.AddDate(-1, 0, 0), NotAfter: notAfter}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	return KeystoreEntry{Alias: alias, NotAfter: notAfter, DER: der}, err
}
