package net

import (
	"crypto/tls"
	"net/http"
	"testing"
)

func TestServiceOptions_TLSConfig(t *testing.T) {
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	svc := NewService(ServiceOptions{
		TLSConfig: tlsConfig,
	})
	tr, ok := svc.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", svc.client.Transport)
	}
	if tr.TLSClientConfig != tlsConfig {
		t.Errorf("TLS config was not applied to the client transport")
	}
}

func TestServiceOptions_TLSConfig_WithClient(t *testing.T) {
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	client := &http.Client{}
	svc := NewService(ServiceOptions{
		Client: client,
		TLSConfig: tlsConfig,
	})
	tr, ok := svc.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", svc.client.Transport)
	}
	if tr.TLSClientConfig != tlsConfig {
		t.Errorf("TLS config was not applied to the existing client transport")
	}
}
