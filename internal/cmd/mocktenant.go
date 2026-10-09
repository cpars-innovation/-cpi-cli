package cmd

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"strings"
	"time"

	"github.com/cpars-innovation/cpicli/internal/cpitest"
	"github.com/cpars-innovation/cpicli/internal/output"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func NewMockTenantCommand() *cobra.Command {
	c := &cobra.Command{
		Use:   "mock-tenant",
		Short: "Run an in-memory SAP CPI tenant for local development and tests (never a real tenant)",
		Long: `Serve an in-memory mock of the SAP CPI APIs that cpictl uses: packages,
artifacts (download, upload, deploy, undeploy), parameters, runtime status,
endpoints, message processing logs with custom headers, steps and errors,
credentials, keystore and Partner Directory. Any credentials are accepted
(Basic Auth with CSRF, or OAuth client credentials at /oauth/token).

--seed demo loads a demo landscape for --tier dev|test|prod:
  Orders_In -> (ProcessDirect) Orders_Route -> (JMS) Billing_In -> (ProcessDirect)
  Billing_Post, Partner_Notify (timer, SFTP), and on dev Returns_In; a day of
  messages with the OrderNo custom header and some failures. The tiers differ
  like real ones: dev is ahead (newer Orders_Route, a new flow, Billing_Post in
  draft), prod has a parameter changed on the tenant, a certificate expiring in
  20 days and no Returns_API credential.
--seed empty starts without content.

The tenant behaves live: a deploy starts the designtime version, uploads are
recorded, a message sent to a flow's endpoint (POST /http/orders/in) creates
a message log. State is in memory; a restart resets it.

Plain http is accepted by cpictl for loopback hosts only. To reach the mock
from another container use --tls: a CA and server certificate are generated
for --tls-hosts, the CA is written to --ca-out; point the client at it with
SSL_CERT_FILE.`,
		Example: `  cpictl mock-tenant --tier dev --addr 127.0.0.1:8081
  cpictl mock-tenant --tier prod --addr 0.0.0.0:8443 --tls --tls-hosts mock-prod,localhost --ca-out /certs/mock-ca.pem`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		Annotations:  map[string]string{annotationOffline: "true", annotationNoEnvelope: "true", annotationNoStats: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			addr, _ := cmd.Flags().GetString("addr")
			seed, _ := cmd.Flags().GetString("seed")
			tier, _ := cmd.Flags().GetString("tier")
			useTLS, _ := cmd.Flags().GetBool("tls")
			if seed != "demo" && seed != "empty" {
				return output.Usagef("--seed %q: demo or empty", seed)
			}
			var tlsConfig *tls.Config
			scheme := "http"
			if useTLS {
				hosts, _ := cmd.Flags().GetStringSlice("tls-hosts")
				caOut, _ := cmd.Flags().GetString("ca-out")
				if caOut == "" {
					return output.Usagef("--tls needs --ca-out (where the generated CA certificate is written)")
				}
				cfg, caPEM, err := mockTLS(hosts)
				if err != nil {
					return err
				}
				if err := os.WriteFile(caOut, caPEM, 0o644); err != nil {
					return err
				}
				tlsConfig, scheme = cfg, "https"
			}
			l, err := net.Listen("tcp", addr)
			if err != nil {
				return err
			}
			m := cpitest.Serve(l, tlsConfig)
			defer m.Close()
			m.Live, m.OAuth = true, true
			host := l.Addr().String()
			if h, p, err := net.SplitHostPort(host); err == nil && (h == "::" || h == "0.0.0.0") {
				host = net.JoinHostPort("localhost", p)
			}
			m.EndpointBase, _ = cmd.Flags().GetString("public-url")
			if m.EndpointBase == "" {
				m.EndpointBase = scheme + "://" + host
			}
			if seed == "demo" {
				if err := cpitest.SeedDemo(m, tier, time.Now()); err != nil {
					return output.Usage(err)
				}
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Mock CPI tenant (%s, seed %s) on %s://%s\n\n", tier, seed, scheme, host)
			fmt.Fprintf(out, "CPICTL_TMN_HOST=%s://%s\nCPICTL_TMN_USERID=mock\nCPICTL_TMN_PASSWORD=mock\n", scheme, host)
			fmt.Fprintf(out, "# or OAuth: CPICTL_OAUTH_HOST=%s CPICTL_OAUTH_CLIENTID=mock CPICTL_OAUTH_CLIENTSECRET=mock\n", host)
			if useTLS {
				caOut, _ := cmd.Flags().GetString("ca-out")
				fmt.Fprintf(out, "SSL_CERT_FILE=%s\n", caOut)
			}
			log.Info().Str("tier", tier).Str("seed", seed).Msgf("Mock tenant listening on %s; stop with Ctrl+C", l.Addr())
			<-cmd.Context().Done()
			return nil
		},
	}
	c.Flags().String("addr", "127.0.0.1:8081", "Listen address")
	c.Flags().String("seed", "demo", "Content: demo or empty")
	c.Flags().String("tier", "dev", "Demo variant: "+strings.Join(cpitest.DemoTiers, ", "))
	c.Flags().String("public-url", "", "Base URL of the flows' runtime endpoints as clients reach the mock, e.g. https://mock-dev:8443 (default: the listen address)")
	c.Flags().Bool("tls", false, "Serve HTTPS with a generated certificate (for access from other containers)")
	c.Flags().StringSlice("tls-hosts", []string{"localhost", "127.0.0.1"}, "Host names and IPs of the generated certificate")
	c.Flags().String("ca-out", "", "With --tls: file the generated CA certificate is written to (PEM)")
	return c
}

// mockTLS generates a CA and a server certificate for hosts.
func mockTLS(hosts []string) (*tls.Config, []byte, error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "cpictl mock-tenant CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(1, 0, 0), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "cpictl mock-tenant"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(1, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	for _, h := range hosts {
		if ip := net.ParseIP(strings.TrimSpace(h)); ip != nil {
			leaf.IPAddresses = append(leaf.IPAddresses, ip)
		} else if h = strings.TrimSpace(h); h != "" {
			leaf.DNSNames = append(leaf.DNSNames, h)
		}
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, nil, err
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}
	cert := tls.Certificate{Certificate: [][]byte{leafDER, caDER}, PrivateKey: key}
	return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), nil
}
