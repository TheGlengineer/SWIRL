package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHostCheck(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := hostCheck(5000, inner)
	cases := []struct {
		host, origin string
		want         int
	}{
		{"127.0.0.1:5000", "", 200},
		{"localhost:5000", "", 200},
		{"LOCALHOST:5000", "", 200},
		{"[::1]:5000", "", 200},
		{"127.0.0.1:5000", "http://127.0.0.1:5000", 200},
		{"127.0.0.1:5000", "null", 200},
		{"evil.example:5000", "", 403}, // DNS rebinding: a name that resolves here
		{"127.0.0.1:5001", "", 403},    // another port
		{"127.0.0.1", "", 403},         // no port
		{"127.0.0.1:5000", "http://evil.example", 403},
		{"127.0.0.1:5000", "http://127.0.0.1:5001", 403},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.Host = c.host
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("host %q origin %q: got %d want %d", c.host, c.origin, w.Code, c.want)
		}
	}
}
