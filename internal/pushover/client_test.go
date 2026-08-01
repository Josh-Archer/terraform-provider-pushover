// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package pushover_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Josh-Archer/terraform-provider-pushover/internal/pushover"
)

// successResponse returns a minimal Pushover success JSON response body.
func successResponse(extra map[string]interface{}) string {
	base := map[string]interface{}{
		"status":  1,
		"request": "test-request-id",
	}
	for k, v := range extra {
		base[k] = v
	}
	b, _ := json.Marshal(base)
	return string(b)
}

// errorResponse returns a Pushover API error JSON response.
func errorResponse(errors ...string) string {
	b, _ := json.Marshal(map[string]interface{}{
		"status": 0,
		"errors": errors,
	})
	return string(b)
}

// ----- SendMessage -----

func TestSendMessage_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if r.FormValue("token") != "test_token" {
			t.Errorf("unexpected token: %s", r.FormValue("token"))
		}
		if r.FormValue("user") != "test_user" {
			t.Errorf("unexpected user: %s", r.FormValue("user"))
		}
		if r.FormValue("message") != "hello world" {
			t.Errorf("unexpected message: %s", r.FormValue("message"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successResponse(nil)))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("test_token", srv.URL, srv.Client())
	resp, err := client.SendMessage(context.Background(), &pushover.MessageRequest{
		User:    "test_user",
		Message: "hello world",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Request != "test-request-id" {
		t.Errorf("expected request id 'test-request-id', got %s", resp.Request)
	}
}

func TestSendMessage_WithAllFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		// Verify all optional fields are forwarded.
		checks := map[string]string{
			"title":     "My Title",
			"url":       "https://example.com",
			"url_title": "Click Here",
			"priority":  "1",
			"sound":     "pushover",
			"device":    "iphone",
			"html":      "1",
		}
		for field, want := range checks {
			if got := r.FormValue(field); got != want {
				t.Errorf("field %q: want %q, got %q", field, want, got)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successResponse(nil)))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())
	_, err := client.SendMessage(context.Background(), &pushover.MessageRequest{
		User:     "u",
		Message:  "test",
		Title:    "My Title",
		URL:      "https://example.com",
		URLTitle: "Click Here",
		Priority: 1,
		Sound:    "pushover",
		Device:   "iphone",
		HTML:     1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSendMessage_EmergencyFieldsForwarded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if r.FormValue("priority") != "2" {
			t.Errorf("expected priority 2, got %s", r.FormValue("priority"))
		}
		if r.FormValue("retry") != "60" {
			t.Errorf("expected retry 60, got %s", r.FormValue("retry"))
		}
		if r.FormValue("expire") != "3600" {
			t.Errorf("expected expire 3600, got %s", r.FormValue("expire"))
		}
		if r.FormValue("callback") != "https://example.com/cb" {
			t.Errorf("unexpected callback: %s", r.FormValue("callback"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successResponse(map[string]interface{}{
			"receipt": "rcpt_abc123",
		})))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())
	resp, err := client.SendMessage(context.Background(), &pushover.MessageRequest{
		User:     "u",
		Message:  "emergency",
		Priority: 2,
		Retry:    60,
		Expire:   3600,
		Callback: "https://example.com/cb",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Receipt != "rcpt_abc123" {
		t.Errorf("expected receipt rcpt_abc123, got %s", resp.Receipt)
	}
}

func TestSendMessage_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(errorResponse("application token is invalid", "user identifier is not a valid user, group, or unsubscribed user identifier")))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("bad_token", srv.URL, srv.Client())
	_, err := client.SendMessage(context.Background(), &pushover.MessageRequest{
		User:    "bad_user",
		Message: "test",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSendMessage_TokenOverride(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if r.FormValue("token") != "override_token" {
			t.Errorf("expected override_token, got %s", r.FormValue("token"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successResponse(nil)))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("provider_token", srv.URL, srv.Client())
	_, err := client.SendMessage(context.Background(), &pushover.MessageRequest{
		Token:   "override_token",
		User:    "u",
		Message: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ----- SendMessage with attachments -----

func TestSendMessage_LocalAttachment(t *testing.T) {
	// Minimal valid-looking JPEG header bytes (content is irrelevant for the client).
	imageBytes := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}
	tmp, err := os.CreateTemp("", "pushover-attach-*.jpg")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(imageBytes); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		ct := r.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "multipart/form-data") {
			t.Errorf("expected multipart/form-data Content-Type, got %q", ct)
		}
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		if r.FormValue("token") != "test_token" {
			t.Errorf("unexpected token: %s", r.FormValue("token"))
		}
		if r.FormValue("user") != "test_user" {
			t.Errorf("unexpected user: %s", r.FormValue("user"))
		}
		if r.FormValue("message") != "with image" {
			t.Errorf("unexpected message: %s", r.FormValue("message"))
		}
		file, header, err := r.FormFile("attachment")
		if err != nil {
			t.Fatalf("FormFile attachment: %v", err)
		}
		defer file.Close()
		got, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("ReadAll attachment: %v", err)
		}
		if !bytes.Equal(got, imageBytes) {
			t.Errorf("attachment bytes mismatch: got %v, want %v", got, imageBytes)
		}
		if header.Header.Get("Content-Type") != "image/jpeg" {
			t.Errorf("unexpected attachment Content-Type: %s", header.Header.Get("Content-Type"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successResponse(nil)))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("test_token", srv.URL, srv.Client())
	resp, err := client.SendMessage(context.Background(), &pushover.MessageRequest{
		User:       "test_user",
		Message:    "with image",
		Attachment: tmp.Name(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Request != "test-request-id" {
		t.Errorf("expected request id 'test-request-id', got %s", resp.Request)
	}
}

func TestSendMessage_RemoteAttachmentURL(t *testing.T) {
	imageBytes := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A} // PNG signature

	// File server that serves the image.
	fileSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(imageBytes)
	}))
	defer fileSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		file, header, err := r.FormFile("attachment")
		if err != nil {
			t.Fatalf("FormFile attachment: %v", err)
		}
		defer file.Close()
		got, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if !bytes.Equal(got, imageBytes) {
			t.Errorf("attachment bytes mismatch")
		}
		if header.Header.Get("Content-Type") != "image/png" {
			t.Errorf("unexpected Content-Type: %s", header.Header.Get("Content-Type"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successResponse(nil)))
	}))
	defer apiSrv.Close()

	client := pushover.NewClientWithBase("tok", apiSrv.URL, apiSrv.Client())
	_, err := client.SendMessage(context.Background(), &pushover.MessageRequest{
		User:       "u",
		Message:    "remote image",
		Attachment: fileSrv.URL + "/graph.png",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSendMessage_AttachmentTypeOverride(t *testing.T) {
	imageBytes := []byte{0x00, 0x01, 0x02, 0x03}
	tmp, err := os.CreateTemp("", "pushover-attach-*.bin")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(imageBytes); err != nil {
		t.Fatalf("Write: %v", err)
	}
	_ = tmp.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		_, header, err := r.FormFile("attachment")
		if err != nil {
			t.Fatalf("FormFile: %v", err)
		}
		if header.Header.Get("Content-Type") != "image/gif" {
			t.Errorf("expected image/gif, got %s", header.Header.Get("Content-Type"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successResponse(nil)))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())
	_, err = client.SendMessage(context.Background(), &pushover.MessageRequest{
		User:           "u",
		Message:        "typed",
		Attachment:     tmp.Name(),
		AttachmentType: "image/gif",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSendMessage_AttachmentTooLarge(t *testing.T) {
	// Create a file just over the 5 MiB limit without holding all of it in memory first.
	tmp, err := os.CreateTemp("", "pushover-large-*.jpg")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(tmp.Name())
	// Truncate to MaxAttachmentBytes+1.
	if err := tmp.Truncate(int64(pushover.MaxAttachmentBytes) + 1); err != nil {
		t.Fatalf("Truncate: %v", err)
	}
	_ = tmp.Close()

	// API server should never be called.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("API should not be called for oversize attachment")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())
	_, err = client.SendMessage(context.Background(), &pushover.MessageRequest{
		User:       "u",
		Message:    "too big",
		Attachment: tmp.Name(),
	})
	if err == nil {
		t.Fatal("expected error for oversize attachment")
	}
	if !strings.Contains(err.Error(), "exceeds Pushover limit") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSendMessage_AttachmentMissingFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("API should not be called for missing attachment")
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())
	_, err := client.SendMessage(context.Background(), &pushover.MessageRequest{
		User:       "u",
		Message:    "missing",
		Attachment: filepath.Join(os.TempDir(), "definitely-does-not-exist-pushover-xyz.jpg"),
	})
	if err == nil {
		t.Fatal("expected error for missing attachment file")
	}
}

// ----- GetSounds -----

func TestGetSounds_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1,"request":"r1","sounds":{"pushover":"Pushover (default)","bike":"Bike","bugle":"Bugle"}}`))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())
	sounds, err := client.GetSounds(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sounds) != 3 {
		t.Errorf("expected 3 sounds, got %d", len(sounds))
	}

	// Verify all keys are present.
	soundMap := make(map[string]string)
	for _, s := range sounds {
		soundMap[s.Key] = s.Name
	}
	if soundMap["pushover"] != "Pushover (default)" {
		t.Errorf("unexpected sound name for 'pushover': %s", soundMap["pushover"])
	}
	if soundMap["bike"] != "Bike" {
		t.Errorf("unexpected sound name for 'bike': %s", soundMap["bike"])
	}
}

func TestGetSounds_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(errorResponse("application token is invalid")))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("bad", srv.URL, srv.Client())
	_, err := client.GetSounds(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ----- ValidateUser -----

func TestValidateUser_RegularUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if r.FormValue("user") != "valid_user_key" {
			t.Errorf("unexpected user: %s", r.FormValue("user"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1,"request":"r1","group":0,"devices":["iphone","android"],"licenses":["iOS","Android"]}`))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())
	resp, err := client.ValidateUser(context.Background(), &pushover.ValidateRequest{
		User: "valid_user_key",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.IsGroupKey() {
		t.Error("expected non-group key")
	}
	if len(resp.Devices) != 2 {
		t.Errorf("expected 2 devices, got %d", len(resp.Devices))
	}
	if resp.Devices[0] != "iphone" && resp.Devices[1] != "iphone" {
		t.Error("expected 'iphone' in devices")
	}
}

func TestValidateUser_GroupKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1,"request":"r1","group":1,"devices":[],"licenses":[]}`))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())
	resp, err := client.ValidateUser(context.Background(), &pushover.ValidateRequest{
		User: "group_key",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.IsGroupKey() {
		t.Error("expected group key")
	}
}

func TestValidateUser_InvalidKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(errorResponse("user key is invalid")))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())
	_, err := client.ValidateUser(context.Background(), &pushover.ValidateRequest{
		User: "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid user key")
	}
}

func TestValidateUser_WithDeviceFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if r.FormValue("device") != "iphone" {
			t.Errorf("expected device 'iphone', got %s", r.FormValue("device"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1,"request":"r1","group":0,"devices":["iphone"],"licenses":["iOS"]}`))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())
	resp, err := client.ValidateUser(context.Background(), &pushover.ValidateRequest{
		User:   "valid_user",
		Device: "iphone",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Devices) != 1 || resp.Devices[0] != "iphone" {
		t.Errorf("unexpected devices: %v", resp.Devices)
	}
}

// ----- GetReceipt -----

func TestGetReceipt_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1,"request":"r1","acknowledged":1,"acknowledged_at":1700000000,"acknowledged_by":"uABC","acknowledged_by_device":"iphone","last_delivered_at":1700000010,"expired":0,"expires_at":1700003600,"called_back":0,"called_back_at":0}`))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())
	resp, err := client.GetReceipt(context.Background(), "receipt123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Acknowledged != 1 {
		t.Errorf("expected acknowledged=1, got %d", resp.Acknowledged)
	}
	if resp.AcknowledgedBy != "uABC" {
		t.Errorf("unexpected acknowledged_by: %s", resp.AcknowledgedBy)
	}
}

func TestGetReceipt_Expired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1,"request":"r1","acknowledged":0,"acknowledged_at":0,"acknowledged_by":"","acknowledged_by_device":"","last_delivered_at":1700000010,"expired":1,"expires_at":1700003600,"called_back":0,"called_back_at":0}`))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())
	resp, err := client.GetReceipt(context.Background(), "receipt456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Expired != 1 {
		t.Errorf("expected expired=1, got %d", resp.Expired)
	}
}

// ----- CancelReceipt -----

func TestCancelReceipt_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successResponse(nil)))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())
	resp, err := client.CancelReceipt(context.Background(), "receipt789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != 1 {
		t.Errorf("expected status 1, got %d", resp.Status)
	}
}

// ----- Group -----

func TestGetGroup_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1,"request":"r1","name":"My Group","users":[{"user":"u1","device":"iphone","memo":"lead","disabled":false},{"user":"u2","device":"","memo":"","disabled":true}]}`))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())
	resp, err := client.GetGroup(context.Background(), "group_key_abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Name != "My Group" {
		t.Errorf("expected 'My Group', got %s", resp.Name)
	}
	if len(resp.Users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(resp.Users))
	}
	if resp.Users[0].User != "u1" {
		t.Errorf("expected u1, got %s", resp.Users[0].User)
	}
	if !resp.Users[1].Disabled {
		t.Error("expected u2 to be disabled")
	}
}

func TestAddGroupUser_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if r.FormValue("user") != "new_user" {
			t.Errorf("unexpected user: %s", r.FormValue("user"))
		}
		if r.FormValue("memo") != "test memo" {
			t.Errorf("unexpected memo: %s", r.FormValue("memo"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successResponse(nil)))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())
	_, err := client.AddGroupUser(context.Background(), "gkey", "new_user", "", "test memo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRemoveGroupUser_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successResponse(nil)))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())
	_, err := client.RemoveGroupUser(context.Background(), "gkey", "user1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnableDisableGroupUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successResponse(nil)))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())

	if _, err := client.DisableGroupUser(context.Background(), "gkey", "u1", ""); err != nil {
		t.Fatalf("DisableGroupUser: %v", err)
	}
	if _, err := client.EnableGroupUser(context.Background(), "gkey", "u1", ""); err != nil {
		t.Fatalf("EnableGroupUser: %v", err)
	}
}

func TestRenameGroup_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if r.FormValue("name") != "New Name" {
			t.Errorf("expected name 'New Name', got %s", r.FormValue("name"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successResponse(nil)))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())
	_, err := client.RenameGroup(context.Background(), "gkey", "New Name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ----- Retry / backoff -----

func TestSendMessage_RetriesOn503ThenSucceeds(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`temporary`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successResponse(nil)))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())
	_, err := client.SendMessage(context.Background(), &pushover.MessageRequest{
		User:    "u",
		Message: "retry me",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestSendMessage_RetriesOn429(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`rate limited`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successResponse(nil)))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())
	_, err := client.SendMessage(context.Background(), &pushover.MessageRequest{
		User:    "u",
		Message: "rate limit",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestSendMessage_ExhaustsRetriesOnPersistent5xx(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())
	// 2 retries → 3 total attempts
	client.SetRetryPolicy(2, time.Millisecond, time.Second)

	_, err := client.SendMessage(context.Background(), &pushover.MessageRequest{
		User:    "u",
		Message: "fail",
	})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to mention HTTP 500, got: %v", err)
	}
}

func TestSendMessage_DoesNotRetryClientErrors(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(errorResponse("application token is invalid")))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("bad", srv.URL, srv.Client())
	_, err := client.SendMessage(context.Background(), &pushover.MessageRequest{
		User:    "u",
		Message: "no retry",
	})
	if err == nil {
		t.Fatal("expected API error")
	}
	if attempts != 1 {
		t.Fatalf("expected a single attempt for 422, got %d", attempts)
	}
}

func TestSendMessage_HonorsRetryAfterHeader(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successResponse(nil)))
	}))
	defer srv.Close()

	var sleeps []time.Duration
	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())
	client.SetSleepForTest(func(ctx context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return nil
	})

	_, err := client.SendMessage(context.Background(), &pushover.MessageRequest{
		User:    "u",
		Message: "retry-after",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sleeps) != 1 {
		t.Fatalf("expected 1 sleep, got %d (%v)", len(sleeps), sleeps)
	}
	if sleeps[0] != 2*time.Second {
		t.Errorf("expected Retry-After delay of 2s, got %v", sleeps[0])
	}
}

func TestSendMessage_ExponentialBackoffWithoutRetryAfter(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successResponse(nil)))
	}))
	defer srv.Close()

	var sleeps []time.Duration
	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())
	client.SetRetryPolicy(3, 100*time.Millisecond, 10*time.Second)
	client.SetSleepForTest(func(ctx context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return nil
	})

	_, err := client.SendMessage(context.Background(), &pushover.MessageRequest{
		User:    "u",
		Message: "backoff",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Delays after attempts 0,1,2 → 100ms, 200ms, 400ms
	want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond}
	if len(sleeps) != len(want) {
		t.Fatalf("expected %d sleeps, got %d (%v)", len(want), len(sleeps), sleeps)
	}
	for i := range want {
		if sleeps[i] != want[i] {
			t.Errorf("sleep[%d]: want %v, got %v", i, want[i], sleeps[i])
		}
	}
}

func TestGetSounds_RetriesOn5xx(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1,"request":"r1","sounds":{"pushover":"Pushover (default)"}}`))
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())
	sounds, err := client.GetSounds(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sounds) != 1 {
		t.Fatalf("expected 1 sound, got %d", len(sounds))
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}
