package net_test

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"net/http"
	"testing"
	"time"
	"github.com/vyquocvu/goosie/internal/net"
)

func syntheticCertificate(subject, issuer string) *x509.Certificate {
	return &x509.Certificate{
		Subject:   pkix.Name{CommonName: subject},
		Issuer:    pkix.Name{CommonName: issuer},
		NotBefore: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:  time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestSecuritySummaryFromResponseExtractsTLSCertificate(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://example.test", nil)
	if err != nil {
		t.Fatal(err)
	}
	cert := syntheticCertificate("Example Subject", "Example Issuer")
	resp := &http.Response{
		Request: req,
		TLS:     &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}},
	}

	summary := net.SecuritySummaryFromResponse(resp, nil)

	if summary.URL != "https://example.test" {
		t.Errorf("URL = %q", summary.URL)
	}
	if summary.Scheme != "https" {
		t.Errorf("Scheme = %q, want https", summary.Scheme)
	}
	if !summary.Secure {
		t.Error("Secure = false, want true")
	}
	if summary.Subject != "CN=Example Subject" {
		t.Errorf("Subject = %q", summary.Subject)
	}
	if summary.Issuer != "CN=Example Issuer" {
		t.Errorf("Issuer = %q", summary.Issuer)
	}
	if !summary.NotBefore.Equal(cert.NotBefore) || !summary.NotAfter.Equal(cert.NotAfter) {
		t.Error("validity dates were not copied")
	}
}

func TestSecuritySummaryFromResponseCapturesError(t *testing.T) {
	errBoom := errors.New("boom")

	summary := net.SecuritySummaryFromResponse(nil, errBoom)

	if summary.Error != "boom" {
		t.Fatalf("Error = %q, want boom", summary.Error)
	}
	if summary.Secure {
		t.Fatal("nil response with error should not be secure")
	}
}
