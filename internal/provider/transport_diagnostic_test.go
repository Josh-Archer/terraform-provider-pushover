// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0
package provider

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/Josh-Archer/terraform-provider-pushover/internal/pushover"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

type diagnosticFailureTransport struct{}

func (diagnosticFailureTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("offline transport failure")
}
func TestSoundsTransportDiagnosticDoesNotExposeToken(t *testing.T) {
	const token = "FAKE_PROVIDER_DIAGNOSTIC_SENTINEL"
	d := &SoundsDataSource{client: pushover.NewClientWithBase(token, "https://example.invalid/1", &http.Client{Transport: diagnosticFailureTransport{}})}
	var resp datasource.ReadResponse
	d.Read(context.Background(), datasource.ReadRequest{}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected diagnostic")
	}
	for _, d := range resp.Diagnostics {
		if strings.Contains(d.Summary(), token) || strings.Contains(d.Detail(), token) {
			t.Fatal("token leaked into provider diagnostic")
		}
	}
}
