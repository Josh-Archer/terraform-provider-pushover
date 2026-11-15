package pushover

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("synthetic network failure")
}
func TestDiagnosticTokenRedaction(t *testing.T) {
	const secret = "FAKE_SENTINEL_TOKEN_53"
	c := NewClientWithBase(secret, "http://example.invalid/1", &http.Client{Transport: failingTransport{}})
	calls := map[string]func() error{
		"receipt": func() error { _, e := c.GetReceipt(context.Background(), "dummy"); return e },
		"sounds":  func() error { _, e := c.GetSounds(context.Background()); return e },
		"limits":  func() error { _, e := c.GetLimits(context.Background()); return e },
		"group":   func() error { _, e := c.GetGroup(context.Background(), "dummy"); return e },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			e := call()
			if e == nil {
				t.Fatal("expected transport error")
			}
			if strings.Contains(e.Error(), secret) {
				t.Fatal("sentinel token leaked in diagnostic")
			}
		})
	}
}
