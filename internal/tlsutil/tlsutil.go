// Package tlsutil builds the server's TLS configuration: a user-supplied
// certificate that is hot-reloaded when it changes on disk, or an
// automatically generated self-signed one.
package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// certSource loads a key pair from disk and transparently reloads it when the
// files change (so certbot/ACME renewals work without a restart).
type certSource struct {
	certFile, keyFile string
	mu                sync.Mutex
	cert              *tls.Certificate
	mod               time.Time
	checked           time.Time
}

func (c *certSource) get(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cert != nil && time.Since(c.checked) < time.Minute {
		return c.cert, nil
	}
	c.checked = time.Now()
	st, err := os.Stat(c.certFile)
	if err != nil {
		if c.cert != nil {
			return c.cert, nil
		}
		return nil, err
	}
	if c.cert != nil && st.ModTime().Equal(c.mod) {
		return c.cert, nil
	}
	pair, err := tls.LoadX509KeyPair(c.certFile, c.keyFile)
	if err != nil {
		if c.cert != nil {
			return c.cert, nil // keep serving the old one
		}
		return nil, err
	}
	c.cert, c.mod = &pair, st.ModTime()
	return c.cert, nil
}

func localNames() ([]string, []net.IP) {
	names := []string{"localhost"}
	if h, err := os.Hostname(); err == nil && h != "" {
		names = append(names, h)
	}
	ips := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				ips = append(ips, ipn.IP)
			}
		}
	}
	return names, ips
}

func generateSelfSigned(certFile, keyFile string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		return err
	}
	names, ips := localNames()
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "DigitalSanDook", Organization: []string{"DigitalSanDook self-signed"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(800 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     names,
		IPAddresses:  ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	kb, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	// write the key first: a cert without its key is useless, the reverse is harmless
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o600); err != nil {
		return err
	}
	return os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
}

// ServerConfig returns the server TLS config. With no user certificate a
// self-signed one is created in dataDir (and replaced at startup when it is
// within 30 days of expiry).
func ServerConfig(dataDir, certFile, keyFile string) (*tls.Config, error) {
	if (certFile == "") != (keyFile == "") {
		return nil, errors.New("-cert and -key must be given together")
	}
	if certFile == "" {
		certFile = filepath.Join(dataDir, "cert.pem")
		keyFile = filepath.Join(dataDir, "key.pem")
		regen := true
		if pair, err := tls.LoadX509KeyPair(certFile, keyFile); err == nil {
			if leaf, err := x509.ParseCertificate(pair.Certificate[0]); err == nil && time.Until(leaf.NotAfter) > 30*24*time.Hour {
				regen = false
			}
		}
		if regen {
			if err := generateSelfSigned(certFile, keyFile); err != nil {
				return nil, err
			}
		}
	}
	src := &certSource{certFile: certFile, keyFile: keyFile}
	if _, err := src.get(nil); err != nil {
		return nil, err
	}
	// CurvePreferences is left at Go's default, which includes the hybrid
	// post-quantum X25519MLKEM768 key exchange.
	return &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: src.get,
		NextProtos:     []string{"h2", "http/1.1"},
	}, nil
}
