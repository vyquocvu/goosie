package net

import (
	"context"
	"net/url"
)

type CookiePolicy func(*url.URL) bool

type cookiePolicyKey struct{}

func WithCookiePolicy(ctx context.Context, policy CookiePolicy) context.Context {
	if policy == nil {
		return ctx
	}
	return context.WithValue(ctx, cookiePolicyKey{}, policy)
}

func cookiesAllowedForURL(ctx context.Context, u *url.URL) bool {
	if u == nil {
		return false
	}
	policy, ok := ctx.Value(cookiePolicyKey{}).(CookiePolicy)
	return !ok || policy(u)
}

func hasCookiePolicy(ctx context.Context) bool {
	_, ok := ctx.Value(cookiePolicyKey{}).(CookiePolicy)
	return ok
}
