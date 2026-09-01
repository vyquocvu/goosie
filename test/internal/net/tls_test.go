package net_test

import (
	"crypto/tls"
	"net/http"
	"testing"
	"github.com/vyquocvu/goosie/internal/net"
)

func TestServiceOptions_TLSConfig(t *testing.T) {
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	svc := net.NewService(net.ServiceOptions{
		TLSConfig: tlsConfig,
	})
	tr, ok := svc.Client().Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", svc.Client().Transport)
	}
	if tr.TLSClientConfig != tlsConfig {
		t.Errorf("TLS config was not applied to the client transport")
	}
}

func TestServiceOptions_TLSConfig_WithClient(t *testing.T) {
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	client := &http.Client{}
	svc := net.NewService(net.ServiceOptions{
		Client:    client,
		TLSConfig: tlsConfig,
	})
	tr, ok := svc.Client().Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", svc.Client().Transport)
	}
	if tr.TLSClientConfig != tlsConfig {
		t.Errorf("TLS config was not applied to the existing client transport")
	}
}
