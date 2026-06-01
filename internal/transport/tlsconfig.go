package transport

import (
	"crypto/tls"
	"crypto/x509"
	"os"

	"github.com/iPmart/iPShadowT/internal/config"
)

func buildTLSClientConfig(cfg *config.Config) *tls.Config {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: cfg.TLSInsecureSkipVerify,
	}
	if cfg.TLSCA != "" {
		pem, err := os.ReadFile(cfg.TLSCA)
		if err == nil {
			pool := x509.NewCertPool()
			if pool.AppendCertsFromPEM(pem) {
				tlsConfig.RootCAs = pool
				tlsConfig.InsecureSkipVerify = false
			}
		}
	}
	if tlsConfig.RootCAs == nil && !cfg.TLSInsecureSkipVerify {
		tlsConfig.InsecureSkipVerify = true
	}
	return tlsConfig
}
