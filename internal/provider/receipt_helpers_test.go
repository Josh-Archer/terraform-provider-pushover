// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"errors"
	"testing"
)

func TestIsReceiptNotFound(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("pushover API error: receipt not found"), true},
		{errors.New("pushover API error: invalid receipt"), true},
		{errors.New("pushover API error: no such receipt"), true},
		{errors.New("network timeout"), false},
		{errors.New("pushover API error: already cancelled"), false},
	}
	for _, tc := range cases {
		if got := isReceiptNotFound(tc.err); got != tc.want {
			t.Errorf("isReceiptNotFound(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestIsReceiptAlreadyResolved(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("pushover API error: receipt not found"), true},
		{errors.New("pushover API error: already cancelled"), true},
		{errors.New("pushover API error: already canceled"), true},
		{errors.New("pushover API error: already acknowledged"), true},
		{errors.New("pushover API error: expired"), true},
		{errors.New("network timeout"), false},
	}
	for _, tc := range cases {
		if got := isReceiptAlreadyResolved(tc.err); got != tc.want {
			t.Errorf("isReceiptAlreadyResolved(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}
