package pushover

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type redirectTransport struct{}

func (redirectTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://example.invalid/%zz?token=FAKE_SENTINEL_TOKEN_53"}}, Body: http.NoBody, Request: r}, nil
}
func TestReviewMalformedRedirect(t *testing.T) {
	c := NewClientWithBase("FAKE_SENTINEL_TOKEN_53", "http://example.invalid/1", &http.Client{Transport: redirectTransport{}})
	_, err := c.GetSounds(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "FAKE_SENTINEL_TOKEN_53") {
		t.Fatal("malformed redirect leaks sentinel")
	}
}
func TestReviewNestedURLError(t *testing.T) {
	e := &url.Error{Op: "Get", URL: "https://example.invalid/?token=FAKE_SENTINEL_TOKEN_53", Err: &url.Error{Op: "parse", URL: "https://example.invalid/?token=FAKE_SENTINEL_TOKEN_53", Err: errors.New("bad url")}}
	if strings.Contains(sanitizeTransportError(e).Error(), "FAKE_SENTINEL_TOKEN_53") {
		t.Fatal("nested URL error leaks sentinel")
	}
}
func TestReviewCancellation(t *testing.T) {
	e := &url.Error{Op: "Get", URL: "https://example.invalid/?token=FAKE_SENTINEL_TOKEN_53", Err: context.Canceled}
	if !errors.Is(sanitizeTransportError(e), context.Canceled) {
		t.Fatal("lost cancellation classification")
	}
}
func TestReviewDeadline(t *testing.T) {
	e := &url.Error{Op: "Get", URL: "https://example.invalid/?token=FAKE_SENTINEL_TOKEN_53", Err: context.DeadlineExceeded}
	got := sanitizeTransportError(e)
	if !errors.Is(got, context.DeadlineExceeded) {
		t.Fatal("lost deadline classification")
	}
	var ue *url.Error
	if !errors.As(got, &ue) {
		t.Fatal("lost error type classification")
	}
	if got.Error() != "HTTP request deadline exceeded" {
		t.Fatalf("unexpected safe message: %s", got.Error())
	}
}
func TestReviewMultipart(t *testing.T) {
	c := NewClientWithBase("FAKE_SENTINEL_TOKEN_53", "http://example.invalid/1", &http.Client{Transport: redirectTransport{}})
	err := c.doPostMultipart(context.Background(), "/messages.json?token=FAKE_SENTINEL_TOKEN_53", "multipart/form-data", []byte("dummy"), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "FAKE_SENTINEL_TOKEN_53") {
		t.Fatal("multipart redirect leaks sentinel")
	}
}
func TestReviewOverride(t *testing.T) {
	c := NewClientWithBase("base", "http://example.invalid/1", &http.Client{Transport: failingTransport{}})
	_, err := c.GetReceiptWithToken(context.Background(), "dummy", "OVERRIDE +/SECRET")
	if err == nil {
		t.Fatal("expected error")
	}
	for _, s := range []string{"OVERRIDE +/SECRET", "OVERRIDE+%2B%2FSECRET"} {
		if strings.Contains(err.Error(), s) {
			t.Fatal("override leaks")
		}
	}
}
func TestReviewNil(t *testing.T) {
	if sanitizeTransportError(nil) != nil {
		t.Fatal("nil changed")
	}
}
