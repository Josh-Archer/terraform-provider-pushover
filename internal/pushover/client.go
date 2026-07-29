// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

// Package pushover provides a client for the Pushover REST API.
package pushover

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.pushover.net/1"

// MaxAttachmentBytes is the Pushover API attachment size limit (5 MiB).
const MaxAttachmentBytes = 5_242_880

// Client is the Pushover API client.
type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client

	// Optional overrides used by tests; zero values / false mean defaults.
	retryConfigured bool
	maxRetries      int
	baseDelay       time.Duration
	maxDelay        time.Duration
	// sleep waits for d or until ctx is cancelled. nil uses time.NewTimer.
	sleep func(ctx context.Context, d time.Duration) error
}

// NewClient creates a new Pushover API client.
func NewClient(token string) *Client {
	return &Client{
		token:      token,
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{},
	}
}

// NewClientWithBase creates a Pushover client that targets a custom base URL.
// This is exported for use in tests only.
func NewClientWithBase(token, base string, httpClient *http.Client) *Client {
	return &Client{
		token:      token,
		baseURL:    base,
		httpClient: httpClient,
	}
}

// SetRetryPolicy configures bounded retry behaviour. Intended for tests.
// maxRetries is the number of retries after the first attempt (total attempts = maxRetries+1).
// Pass 0 for maxRetries to disable retries.
func (c *Client) SetRetryPolicy(maxRetries int, baseDelay, maxDelay time.Duration) {
	c.retryConfigured = true
	if maxRetries < 0 {
		maxRetries = 0
	}
	c.maxRetries = maxRetries
	if baseDelay > 0 {
		c.baseDelay = baseDelay
	}
	if maxDelay > 0 {
		c.maxDelay = maxDelay
	}
}

// SetSleepForTest overrides the backoff wait function. Intended for tests only.
func (c *Client) SetSleepForTest(fn func(ctx context.Context, d time.Duration) error) {
	c.sleep = fn
}

// APIResponse is the base Pushover API response.
type APIResponse struct {
	Status  int      `json:"status"`
	Request string   `json:"request"`
	Errors  []string `json:"errors,omitempty"`
}

// MessageRequest holds all fields for sending a Pushover message.
type MessageRequest struct {
	Token     string `json:"token"`
	User      string `json:"user"`
	Message   string `json:"message"`
	Title     string `json:"title,omitempty"`
	URL       string `json:"url,omitempty"`
	URLTitle  string `json:"url_title,omitempty"`
	Priority  int    `json:"priority"`
	Sound     string `json:"sound,omitempty"`
	Device    string `json:"device,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
	HTML      int    `json:"html,omitempty"`
	Monospace int    `json:"monospace,omitempty"`
	TTL       int    `json:"ttl,omitempty"`
	// Emergency priority (priority=2) fields
	Retry    int    `json:"retry,omitempty"`
	Expire   int    `json:"expire,omitempty"`
	Callback string `json:"callback,omitempty"`

	// Attachment is a local filesystem path or http(s) URL of an image to attach.
	// The file is uploaded via multipart/form-data. Max size: MaxAttachmentBytes.
	Attachment string `json:"-"`
	// AttachmentType is an optional MIME type override (e.g. "image/jpeg").
	// When empty, the type is inferred from the filename or Content-Type header.
	AttachmentType string `json:"-"`
}

// MessageResponse is the response from sending a message.
type MessageResponse struct {
	APIResponse
	Receipt string `json:"receipt,omitempty"`
}

// ReceiptResponse is the response from polling an emergency receipt.
type ReceiptResponse struct {
	APIResponse
	Acknowledged        int    `json:"acknowledged"`
	AcknowledgedAt      int64  `json:"acknowledged_at"`
	AcknowledgedBy      string `json:"acknowledged_by"`
	AcknowledgedByDevice string `json:"acknowledged_by_device"`
	LastDeliveredAt     int64  `json:"last_delivered_at"`
	Expired             int    `json:"expired"`
	ExpiresAt           int64  `json:"expires_at"`
	CalledBack          int    `json:"called_back"`
	CalledBackAt        int64  `json:"called_back_at"`
}

// SoundsResponse is the response from listing sounds.
type SoundsResponse struct {
	APIResponse
	Sounds map[string]string `json:"sounds"`
}

// Sound represents a single Pushover sound.
type Sound struct {
	Key  string
	Name string
}

// ValidateRequest holds fields for validating a user/group key.
type ValidateRequest struct {
	Token  string `json:"token"`
	User   string `json:"user"`
	Device string `json:"device,omitempty"`
}

// ValidateResponse is the response from validating a user.
type ValidateResponse struct {
	APIResponse
	Group   int      `json:"group"`
	Devices []string `json:"devices"`
	Licenses []string `json:"licenses"`
}

// GroupResponse is the response from group API calls.
type GroupResponse struct {
	APIResponse
	Name  string        `json:"name"`
	Users []GroupMember `json:"users"`
}

// GroupMember represents a member in a Pushover group.
type GroupMember struct {
	User     string `json:"user"`
	Device   string `json:"device,omitempty"`
	Memo     string `json:"memo,omitempty"`
	Disabled bool   `json:"disabled"`
}

// SendMessage sends a notification via the Pushover API.
// When Attachment is set, the request is sent as multipart/form-data with the
// image file included. Otherwise, application/x-www-form-urlencoded is used.
func (c *Client) SendMessage(ctx context.Context, req *MessageRequest) (*MessageResponse, error) {
	if req.Token == "" {
		req.Token = c.token
	}

	if req.Attachment != "" {
		return c.sendMessageWithAttachment(ctx, req)
	}

	params := url.Values{}
	params.Set("token", req.Token)
	params.Set("user", req.User)
	params.Set("message", req.Message)
	if req.Title != "" {
		params.Set("title", req.Title)
	}
	if req.URL != "" {
		params.Set("url", req.URL)
	}
	if req.URLTitle != "" {
		params.Set("url_title", req.URLTitle)
	}
	params.Set("priority", strconv.Itoa(req.Priority))
	if req.Sound != "" {
		params.Set("sound", req.Sound)
	}
	if req.Device != "" {
		params.Set("device", req.Device)
	}
	if req.Timestamp != 0 {
		params.Set("timestamp", strconv.FormatInt(req.Timestamp, 10))
	}
	if req.HTML != 0 {
		params.Set("html", strconv.Itoa(req.HTML))
	}
	if req.Monospace != 0 {
		params.Set("monospace", strconv.Itoa(req.Monospace))
	}
	if req.TTL != 0 {
		params.Set("ttl", strconv.Itoa(req.TTL))
	}
	if req.Priority == 2 {
		params.Set("retry", strconv.Itoa(req.Retry))
		params.Set("expire", strconv.Itoa(req.Expire))
		if req.Callback != "" {
			params.Set("callback", req.Callback)
		}
	}

	var resp MessageResponse
	if err := c.doPost(ctx, "/messages.json", params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// attachmentData holds loaded attachment bytes and metadata.
type attachmentData struct {
	filename    string
	contentType string
	data        []byte
}

// loadAttachment reads attachment bytes from a local path or remote http(s) URL.
func (c *Client) loadAttachment(ctx context.Context, source, typeOverride string) (*attachmentData, error) {
	if isRemoteURL(source) {
		return c.downloadAttachment(ctx, source, typeOverride)
	}
	return readLocalAttachment(source, typeOverride)
}

func isRemoteURL(source string) bool {
	return strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://")
}

func readLocalAttachment(path, typeOverride string) (*attachmentData, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("reading attachment file: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("attachment path is a directory: %s", path)
	}
	if info.Size() > MaxAttachmentBytes {
		return nil, fmt.Errorf("attachment size %d exceeds Pushover limit of %d bytes (5 MiB)", info.Size(), MaxAttachmentBytes)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading attachment file: %w", err)
	}
	if len(data) > MaxAttachmentBytes {
		return nil, fmt.Errorf("attachment size %d exceeds Pushover limit of %d bytes (5 MiB)", len(data), MaxAttachmentBytes)
	}

	filename := filepath.Base(path)
	contentType := typeOverride
	if contentType == "" {
		contentType = mimeTypeFromFilename(filename)
	}

	return &attachmentData{
		filename:    filename,
		contentType: contentType,
		data:        data,
	}, nil
}

func (c *Client) downloadAttachment(ctx context.Context, rawURL, typeOverride string) (*attachmentData, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating attachment download request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading attachment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("downloading attachment: unexpected status %s", resp.Status)
	}

	// Cap read at MaxAttachmentBytes+1 so we can detect oversize without loading everything.
	limited := io.LimitReader(resp.Body, MaxAttachmentBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("reading attachment body: %w", err)
	}
	if len(data) > MaxAttachmentBytes {
		return nil, fmt.Errorf("attachment size exceeds Pushover limit of %d bytes (5 MiB)", MaxAttachmentBytes)
	}

	filename := filenameFromURL(rawURL)
	if filename == "" || filename == "." || filename == "/" {
		filename = "attachment"
	}

	contentType := typeOverride
	if contentType == "" {
		if ct := resp.Header.Get("Content-Type"); ct != "" {
			// Strip parameters such as "; charset=utf-8".
			if mediaType, _, err := mime.ParseMediaType(ct); err == nil && mediaType != "" {
				contentType = mediaType
			} else {
				contentType = ct
			}
		}
	}
	if contentType == "" || contentType == "application/octet-stream" {
		if inferred := mimeTypeFromFilename(filename); inferred != "application/octet-stream" {
			contentType = inferred
		}
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return &attachmentData{
		filename:    filename,
		contentType: contentType,
		data:        data,
	}, nil
}

func filenameFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "attachment"
	}
	base := filepath.Base(u.Path)
	if base == "" || base == "." || base == "/" {
		return "attachment"
	}
	return base
}

func mimeTypeFromFilename(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		return "application/octet-stream"
	}
	if t := mime.TypeByExtension(ext); t != "" {
		// Strip parameters.
		if mediaType, _, err := mime.ParseMediaType(t); err == nil {
			return mediaType
		}
		return t
	}
	// Common image types that may not be registered on all platforms.
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	default:
		return "application/octet-stream"
	}
}

func (c *Client) sendMessageWithAttachment(ctx context.Context, req *MessageRequest) (*MessageResponse, error) {
	att, err := c.loadAttachment(ctx, req.Attachment, req.AttachmentType)
	if err != nil {
		return nil, err
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	fields := map[string]string{
		"token":    req.Token,
		"user":     req.User,
		"message":  req.Message,
		"priority": strconv.Itoa(req.Priority),
	}
	if req.Title != "" {
		fields["title"] = req.Title
	}
	if req.URL != "" {
		fields["url"] = req.URL
	}
	if req.URLTitle != "" {
		fields["url_title"] = req.URLTitle
	}
	if req.Sound != "" {
		fields["sound"] = req.Sound
	}
	if req.Device != "" {
		fields["device"] = req.Device
	}
	if req.Timestamp != 0 {
		fields["timestamp"] = strconv.FormatInt(req.Timestamp, 10)
	}
	if req.HTML != 0 {
		fields["html"] = strconv.Itoa(req.HTML)
	}
	if req.Monospace != 0 {
		fields["monospace"] = strconv.Itoa(req.Monospace)
	}
	if req.TTL != 0 {
		fields["ttl"] = strconv.Itoa(req.TTL)
	}
	if req.Priority == 2 {
		fields["retry"] = strconv.Itoa(req.Retry)
		fields["expire"] = strconv.Itoa(req.Expire)
		if req.Callback != "" {
			fields["callback"] = req.Callback
		}
	}

	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return nil, fmt.Errorf("writing multipart field %q: %w", k, err)
		}
	}

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="attachment"; filename="%s"`, escapeQuotes(att.filename)))
	h.Set("Content-Type", att.contentType)
	part, err := w.CreatePart(h)
	if err != nil {
		return nil, fmt.Errorf("creating attachment part: %w", err)
	}
	if _, err := part.Write(att.data); err != nil {
		return nil, fmt.Errorf("writing attachment data: %w", err)
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("closing multipart writer: %w", err)
	}

	var resp MessageResponse
	if err := c.doPostMultipart(ctx, "/messages.json", w.FormDataContentType(), &body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func escapeQuotes(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

func (c *Client) doPostMultipart(ctx context.Context, path, contentType string, body io.Reader, out interface{}) error {
	u := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, body)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	type statusChecker struct {
		Status int      `json:"status"`
		Errors []string `json:"errors"`
	}
	var sc statusChecker
	_ = json.Unmarshal(respBody, &sc)
	if sc.Status != 1 {
		return fmt.Errorf("pushover API error: %s", strings.Join(sc.Errors, "; "))
	}

	return nil
}

// GetReceipt retrieves delivery status for an emergency message receipt.
func (c *Client) GetReceipt(ctx context.Context, receipt string) (*ReceiptResponse, error) {
	path := fmt.Sprintf("/receipts/%s.json?token=%s", receipt, url.QueryEscape(c.token))
	var resp ReceiptResponse
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CancelReceipt cancels an outstanding emergency notification.
func (c *Client) CancelReceipt(ctx context.Context, receipt string) (*APIResponse, error) {
	params := url.Values{}
	params.Set("token", c.token)
	var resp APIResponse
	if err := c.doPost(ctx, fmt.Sprintf("/receipts/%s/cancel.json", receipt), params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetSounds returns the list of available Pushover sounds.
func (c *Client) GetSounds(ctx context.Context) ([]Sound, error) {
	path := fmt.Sprintf("/sounds.json?token=%s", url.QueryEscape(c.token))
	var resp SoundsResponse
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, err
	}
	sounds := make([]Sound, 0, len(resp.Sounds))
	for k, v := range resp.Sounds {
		sounds = append(sounds, Sound{Key: k, Name: v})
	}
	return sounds, nil
}

// ValidateUser validates a Pushover user or group key.
func (c *Client) ValidateUser(ctx context.Context, req *ValidateRequest) (*ValidateResponse, error) {
	if req.Token == "" {
		req.Token = c.token
	}
	params := url.Values{}
	params.Set("token", req.Token)
	params.Set("user", req.User)
	if req.Device != "" {
		params.Set("device", req.Device)
	}
	var resp ValidateResponse
	if err := c.doPost(ctx, "/users/validate.json", params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetGroup retrieves information about a Pushover delivery group.
func (c *Client) GetGroup(ctx context.Context, groupKey string) (*GroupResponse, error) {
	path := fmt.Sprintf("/groups/%s.json?token=%s", groupKey, url.QueryEscape(c.token))
	var resp GroupResponse
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RenameGroup renames a Pushover delivery group.
func (c *Client) RenameGroup(ctx context.Context, groupKey, name string) (*APIResponse, error) {
	params := url.Values{}
	params.Set("token", c.token)
	params.Set("name", name)
	var resp APIResponse
	if err := c.doPost(ctx, fmt.Sprintf("/groups/%s/rename.json", groupKey), params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// AddGroupUser adds a user to a Pushover delivery group.
func (c *Client) AddGroupUser(ctx context.Context, groupKey, user, device, memo string) (*APIResponse, error) {
	params := url.Values{}
	params.Set("token", c.token)
	params.Set("user", user)
	if device != "" {
		params.Set("device", device)
	}
	if memo != "" {
		params.Set("memo", memo)
	}
	var resp APIResponse
	if err := c.doPost(ctx, fmt.Sprintf("/groups/%s/add_user.json", groupKey), params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RemoveGroupUser removes a user from a Pushover delivery group.
func (c *Client) RemoveGroupUser(ctx context.Context, groupKey, user, device string) (*APIResponse, error) {
	params := url.Values{}
	params.Set("token", c.token)
	params.Set("user", user)
	if device != "" {
		params.Set("device", device)
	}
	var resp APIResponse
	if err := c.doPost(ctx, fmt.Sprintf("/groups/%s/delete_user.json", groupKey), params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// EnableGroupUser re-enables a disabled user in a Pushover delivery group.
func (c *Client) EnableGroupUser(ctx context.Context, groupKey, user, device string) (*APIResponse, error) {
	params := url.Values{}
	params.Set("token", c.token)
	params.Set("user", user)
	if device != "" {
		params.Set("device", device)
	}
	var resp APIResponse
	if err := c.doPost(ctx, fmt.Sprintf("/groups/%s/enable_user.json", groupKey), params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DisableGroupUser disables a user in a Pushover delivery group.
func (c *Client) DisableGroupUser(ctx context.Context, groupKey, user, device string) (*APIResponse, error) {
	params := url.Values{}
	params.Set("token", c.token)
	params.Set("user", user)
	if device != "" {
		params.Set("device", device)
	}
	var resp APIResponse
	if err := c.doPost(ctx, fmt.Sprintf("/groups/%s/disable_user.json", groupKey), params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) doPost(ctx context.Context, path string, params url.Values, out interface{}) error {
	return c.doWithRetry(ctx, http.MethodPost, path, params.Encode(), "application/x-www-form-urlencoded", out)
}

func (c *Client) doGet(ctx context.Context, path string, out interface{}) error {
	return c.doWithRetry(ctx, http.MethodGet, path, "", "", out)
}

func (c *Client) maxRetriesOrDefault() int {
	if c.retryConfigured {
		return c.maxRetries
	}
	return defaultMaxRetries
}

func (c *Client) baseDelayOrDefault() time.Duration {
	if c.baseDelay > 0 {
		return c.baseDelay
	}
	return defaultBaseDelay
}

func (c *Client) maxDelayOrDefault() time.Duration {
	if c.maxDelay > 0 {
		return c.maxDelay
	}
	return defaultMaxDelay
}

func (c *Client) doWithRetry(ctx context.Context, method, path, body, contentType string, out interface{}) error {
	maxRetries := c.maxRetriesOrDefault()
	var nextDelay time.Duration

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			if err := c.wait(ctx, nextDelay); err != nil {
				return err
			}
		}

		u := c.baseURL + path
		var bodyReader io.Reader
		if method != http.MethodGet {
			bodyReader = strings.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("sending request: %w", err)
		}

		respBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("reading response: %w", err)
		}

		if isRetryableStatus(resp.StatusCode) {
			nextDelay = c.retryDelay(attempt, resp.Header)
			if attempt < maxRetries {
				continue
			}
			return fmt.Errorf("pushover API returned HTTP %d after %d attempts", resp.StatusCode, attempt+1)
		}

		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}

		type statusChecker struct {
			Status int      `json:"status"`
			Errors []string `json:"errors"`
		}
		var sc statusChecker
		_ = json.Unmarshal(respBody, &sc)
		if sc.Status != 1 {
			return fmt.Errorf("pushover API error: %s", strings.Join(sc.Errors, "; "))
		}

		return nil
	}

	return fmt.Errorf("pushover API request failed")
}

func isRetryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= 500
}

// retryDelay returns how long to wait before the next attempt.
// Prefer Retry-After when present; otherwise exponential backoff from baseDelay.
// attempt is the zero-based index of the attempt that just failed.
func (c *Client) retryDelay(attempt int, header http.Header) time.Duration {
	if d, ok := parseRetryAfter(header.Get("Retry-After")); ok {
		return d
	}
	base := c.baseDelayOrDefault()
	max := c.maxDelayOrDefault()
	delay := base
	for i := 0; i < attempt; i++ {
		if delay >= max {
			return max
		}
		delay *= 2
	}
	if delay > max {
		return max
	}
	return delay
}

// parseRetryAfter parses a Retry-After header value (delta-seconds or HTTP-date).
func parseRetryAfter(v string) (time.Duration, bool) {
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
		if secs < 0 {
			return 0, true
		}
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		d := time.Until(t)
		if d < 0 {
			return 0, true
		}
		return d, true
	}
	return 0, false
}

func (c *Client) wait(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	if c.sleep != nil {
		return c.sleep(ctx, d)
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// IsGroupKey returns true if the validation response indicates the key is a group key.
func (v *ValidateResponse) IsGroupKey() bool {
	return v.Group == 1
}
