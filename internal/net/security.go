package net

import (
	"net/http"
	"net/url"
	"time"
)

type SecuritySummary struct {
	URL       string
	Scheme    string
	Secure    bool
	Subject   string
	Issuer    string
	NotBefore time.Time
	NotAfter  time.Time
	Error     string
}

func securitySummaryFromURL(u *url.URL) SecuritySummary {
	if u == nil {
		return SecuritySummary{}
	}
	return SecuritySummary{
		URL:    u.String(),
		Scheme: u.Scheme,
		Secure: u.Scheme == "https",
	}
}

func SecuritySummaryFromResponse(resp *http.Response, fetchErr error) SecuritySummary {
	var summary SecuritySummary
	if resp != nil && resp.Request != nil && resp.Request.URL != nil {
		summary.URL = resp.Request.URL.String()
		summary.Scheme = resp.Request.URL.Scheme
	}
	if fetchErr != nil {
		summary.Error = fetchErr.Error()
		return summary
	}
	summary.Secure = summary.Scheme == "https"
	if resp == nil || resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0 {
		return summary
	}
	cert := resp.TLS.PeerCertificates[0]
	summary.Subject = cert.Subject.String()
	summary.Issuer = cert.Issuer.String()
	summary.NotBefore = cert.NotBefore
	summary.NotAfter = cert.NotAfter
	return summary
}
