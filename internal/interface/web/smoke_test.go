package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandler(t *testing.T) {
	srv := httptest.NewServer(Handler())
	defer srv.Close()

	cases := []struct {
		path, want string
	}{
		{"/", "<!doctype html>"},
		{"/static/styles.css", "--bg:"},
		{"/static/app.js", "api.listPairs"},
		{"/favicon.svg", "<svg"},
	}
	for _, c := range cases {
		resp, err := http.Get(srv.URL + c.path)
		if err != nil {
			t.Fatalf("%s: %v", c.path, err)
		}
		b, _ := io.ReadAll(resp.Body)
		// nolint:errcheck
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("%s: status=%d", c.path, resp.StatusCode)
		}
		if !strings.Contains(string(b), c.want) {
			t.Errorf("%s: missing %q", c.path, c.want)
		}
	}
}
