package proxy

import (
	"fmt"
	"html"
	"net/http"
	"net/netip"
	"time"

	"github.com/sakusi4/t5s/internal/acl"
)

const blockedPage = `<!doctype html>
<meta charset="utf-8">
<title>403 Forbidden</title>
<h1>Access denied</h1>
<p>Your IP address <code>%s</code> is not on the allowlist for this share.</p>
<p>Ask the person who sent you this link to allow it.</p>
`

func guard(allow acl.List, clientIPHeader string, log *accessLog) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			addr := clientAddr(r, clientIPHeader)
			allowed := allow.Allows(addr)
			log.record(addr, allowed, time.Now())
			if !allowed {
				writeBlocked(w, addr)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientAddr(r *http.Request, clientIPHeader string) netip.Addr {
	if clientIPHeader != "" {
		if addr, err := netip.ParseAddr(r.Header.Get(clientIPHeader)); err == nil {
			return addr.Unmap().WithZone("")
		}
	}
	remote, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}
	}
	return remote.Addr().Unmap().WithZone("")
}

func writeBlocked(w http.ResponseWriter, addr netip.Addr) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusForbidden)
	fmt.Fprintf(w, blockedPage, html.EscapeString(addr.String()))
}
