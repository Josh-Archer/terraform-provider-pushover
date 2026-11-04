// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

// Package pushover provides a client for the Pushover REST API.
package pushover

import (
	"bytes"
	"context"
	"encoding/base64"
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
	"sort"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.pushover.net/1"

// Default retry policy for transient HTTP failures (5xx and 429).
const (
	defaultMaxRetries = 3
	defaultBaseDelay  = 500 * time.Millisecond
	defaultMaxDelay   = 8 * time.Second
)

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
	// AttachmentBase64 is a base64-encoded image to include with the message.
	// When set, it is sent in application/x-www-form-urlencoded params. Max decoded size: MaxAttachmentBytes.
	AttachmentBase64 string `json:"-"`
	// AttachmentType is an optional MIME type override (e.g. "image/jpeg").
	// When empty with Attachment, the type is inferred from the filename or Content-Type header.
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
	Acknowledged         int    `json:"acknowledged"`
	AcknowledgedAt       int64  `json:"acknowledged_at"`
	AcknowledgedBy       string `json:"acknowledged_by"`
	AcknowledgedByDevice string `json:"acknowledged_by_device"`
	LastDeliveredAt      int64  `json:"last_delivered_at"`
	Expired              int    `json:"expired"`
	ExpiresAt            int64  `json:"expires_at"`
	CalledBack           int    `json:"called_back"`
	CalledBackAt         int64  `json:"called_back_at"`
}

// SoundsResponse is the response from listing sounds.
type SoundsResponse struct {
	APIResponse
	Sounds map[string]string `json:"sounds"`
}

// LimitsResponse is the response from querying application message limits.
type LimitsResponse struct {
	APIResponse
	Limit     int   `json:"limit"`
	Remaining int   `json:"remaining"`
	Reset     int64 `json:"reset"`
}

// LicensesResponse is the response from querying or assigning application license credits.
// See https://pushover.net/api/licensing.
type LicensesResponse struct {
	APIResponse
	Credits int `json:"credits"`
}

// LicenseAssignRequest holds fields for assigning a pre-paid Pushover license credit.
// Exactly one of User or Email must be set.
// OS is optional ("Android", "iOS", or "Desktop").
type LicenseAssignRequest struct {
	// Token overrides the client application token when non-empty.
	Token string
	// User is the recipient's Pushover user key (mutually exclusive with Email).
	User string
	// Email is the recipient's email address (mutually exclusive with User).
	Email string
	// OS optionally restricts the license to "Android", "iOS", or "Desktop".
	OS string
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
	Group    int      `json:"group"`
	Devices  []string `json:"devices"`
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

	if req.Attachment != "" && req.AttachmentBase64 != "" {
		return nil, fmt.Errorf("cannot specify both Attachment and AttachmentBase64")
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
	if req.AttachmentBase64 != "" {
		if strings.TrimSpace(req.AttachmentType) == "" {
			return nil, fmt.Errorf("attachment_type is required when AttachmentBase64 is set")
		}
		trimmed := strings.TrimSpace(req.AttachmentBase64)
		decoded, err := base64.StdEncoding.DecodeString(trimmed)
		if err != nil {
			if d, errRaw := base64.RawStdEncoding.DecodeString(trimmed); errRaw == nil {
				decoded = d
			} else {
				return nil, fmt.Errorf("invalid base64 encoding in AttachmentBase64: %w", err)
			}
		}
		if int64(len(decoded)) > MaxAttachmentBytes {
			return nil, fmt.Errorf("attachment size %d exceeds Pushover limit of %d bytes (5 MiB)", len(decoded), MaxAttachmentBytes)
		}
		params.Set("attachment_base64", trimmed)
		if req.AttachmentType != "" {
			params.Set("attachment_type", req.AttachmentType)
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
	defer func() { _ = resp.Body.Close() }()

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
	if err := c.doPostMultipart(ctx, "/messages.json", w.FormDataContentType(), body.Bytes(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func escapeQuotes(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

func (c *Client) doPostMultipart(ctx context.Context, path, contentType string, body []byte, out interface{}) error {
	maxRetries := c.maxRetriesOrDefault()
	var nextDelay time.Duration

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			if err := c.wait(ctx, nextDelay); err != nil {
				return err
			}
		}

		u := c.baseURL + path
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Content-Type", contentType)

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

		return unmarshalResponse(resp.StatusCode, respBody, out)
	}

	return fmt.Errorf("pushover API request failed")
}

// GetReceipt retrieves delivery status for an emergency message receipt using the client's token.
func (c *Client) GetReceipt(ctx context.Context, receipt string) (*ReceiptResponse, error) {
	return c.GetReceiptWithToken(ctx, receipt, "")
}

// GetReceiptWithToken retrieves delivery status using an optional token override.
func (c *Client) GetReceiptWithToken(ctx context.Context, receipt, token string) (*ReceiptResponse, error) {
	if token == "" {
		token = c.token
	}
	path := fmt.Sprintf("/receipts/%s.json?token=%s", url.PathEscape(receipt), url.QueryEscape(token))
	var resp ReceiptResponse
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CancelReceipt cancels an outstanding emergency notification using the client's token.
func (c *Client) CancelReceipt(ctx context.Context, receipt string) (*APIResponse, error) {
	return c.CancelReceiptWithToken(ctx, receipt, "")
}

// CancelReceiptWithToken cancels an outstanding emergency notification using an optional token override.
func (c *Client) CancelReceiptWithToken(ctx context.Context, receipt, token string) (*APIResponse, error) {
	if token == "" {
		token = c.token
	}
	params := url.Values{}
	params.Set("token", token)
	var resp APIResponse
	if err := c.doPost(ctx, fmt.Sprintf("/receipts/%s/cancel.json", url.PathEscape(receipt)), params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetSounds returns the list of available Pushover sounds, sorted by key.
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
	sort.Slice(sounds, func(i, j int) bool {
		return sounds[i].Key < sounds[j].Key
	})
	return sounds, nil
}

// GetLimits retrieves the current message quota and remaining limit for an application.
func (c *Client) GetLimits(ctx context.Context, token ...string) (*LimitsResponse, error) {
	tok := c.token
	if len(token) > 0 && token[0] != "" {
		tok = token[0]
	}
	path := fmt.Sprintf("/apps/limits.json?token=%s", url.QueryEscape(tok))
	var resp LimitsResponse
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
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
func (c *Client) GetGroup(ctx context.Context, groupKey string, token ...string) (*GroupResponse, error) {
	tok := c.token
	if len(token) > 0 && token[0] != "" {
		tok = token[0]
	}
	path := fmt.Sprintf("/groups/%s.json?token=%s", url.PathEscape(groupKey), url.QueryEscape(tok))
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
	params.Set("memo", memo)
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

// TeamResponse is the response from GET /teams.json.
// See https://pushover.net/api/teams.
type TeamResponse struct {
	APIResponse
	Name  string     `json:"name"`
	Users []TeamUser `json:"users"`
}

// TeamUser is one member of a Pushover team, as returned by the team listing.
type TeamUser struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	Administrator bool   `json:"administrator"`
}

// AddTeamUserRequest adds a user to a Pushover team.
// Token is the team API token, not an application API token.
type AddTeamUserRequest struct {
	Token    string
	Email    string
	Name     string
	Password string
	// Instant, when true, sends the welcome email with an Instant Login link.
	Instant bool
	// Admin, when true, adds the user as a team administrator.
	Admin bool
	// Group is a delivery group name. Pushover creates the group if it does not exist.
	// Empty means the team's default delivery group.
	Group string
}

// GetTeam returns the team name and current members.
// GET https://api.pushover.net/1/teams.json?token=TEAM_TOKEN
func (c *Client) GetTeam(ctx context.Context, token string) (*TeamResponse, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("team token is required")
	}
	path := fmt.Sprintf("/teams.json?token=%s", url.QueryEscape(token))
	var resp TeamResponse
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, redactSecret(err, token)
	}
	return &resp, nil
}

// AddTeamUser adds a user to a team.
// POST https://api.pushover.net/1/teams/add_user.json
func (c *Client) AddTeamUser(ctx context.Context, req *AddTeamUserRequest) (*APIResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("add team user request is nil")
	}
	token := strings.TrimSpace(req.Token)
	email := strings.TrimSpace(req.Email)
	if token == "" {
		return nil, fmt.Errorf("team token is required")
	}
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}

	params := url.Values{}
	params.Set("token", token)
	params.Set("email", email)
	if name := strings.TrimSpace(req.Name); name != "" {
		params.Set("name", name)
	}
	// Do not trim passwords; leading or trailing spaces can be significant.
	if req.Password != "" {
		params.Set("password", req.Password)
	}
	// Pushover treats the string "true" as the flag. Omit it otherwise.
	if req.Instant {
		params.Set("instant", "true")
	}
	if req.Admin {
		params.Set("admin", "true")
	}
	if group := strings.TrimSpace(req.Group); group != "" {
		params.Set("group", group)
	}

	var resp APIResponse
	if err := c.doPost(ctx, "/teams/add_user.json", params, &resp); err != nil {
		return nil, redactSecret(redactSecret(err, token), req.Password)
	}
	return &resp, nil
}

// RemoveTeamUser removes a team member without deleting their Pushover account.
// POST https://api.pushover.net/1/teams/remove_user.json
func (c *Client) RemoveTeamUser(ctx context.Context, token, email string) (*APIResponse, error) {
	token = strings.TrimSpace(token)
	email = strings.TrimSpace(email)
	if token == "" {
		return nil, fmt.Errorf("team token is required")
	}
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}
	params := url.Values{}
	params.Set("token", token)
	params.Set("email", email)
	var resp APIResponse
	if err := c.doPost(ctx, "/teams/remove_user.json", params, &resp); err != nil {
		return nil, redactSecret(err, token)
	}
	return &resp, nil
}

// RevokeTeamInvitation revokes a pending invitation for someone who has not joined.
// POST https://api.pushover.net/1/teams/revoke_invitation.json
func (c *Client) RevokeTeamInvitation(ctx context.Context, token, email string) (*APIResponse, error) {
	token = strings.TrimSpace(token)
	email = strings.TrimSpace(email)
	if token == "" {
		return nil, fmt.Errorf("team token is required")
	}
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}
	params := url.Values{}
	params.Set("token", token)
	params.Set("email", email)
	var resp APIResponse
	if err := c.doPost(ctx, "/teams/revoke_invitation.json", params, &resp); err != nil {
		return nil, redactSecret(err, token)
	}
	return &resp, nil
}

// redactSecret removes a credential from an error string. Transport errors from
// net/http include the request URL, and team calls put the team token in that URL.
// Short values are left alone so a short password cannot blank out unrelated words.
func redactSecret(err error, secret string) error {
	if err == nil || len(secret) < 8 {
		return err
	}
	msg := err.Error()
	redacted := strings.ReplaceAll(msg, secret, "[redacted]")
	if escaped := url.QueryEscape(secret); escaped != secret {
		redacted = strings.ReplaceAll(redacted, escaped, "[redacted]")
	}
	if redacted == msg {
		return err
	}
	return fmt.Errorf("%s", redacted)
}

// SubscriptionMigrateRequest migrates an existing user key onto an application subscription.
// See https://pushover.net/api/subscriptions (User Key Migration).
type SubscriptionMigrateRequest struct {
	// Token overrides the client application token when non-empty.
	Token string
	// Subscription is the subscription code from the Pushover dashboard.
	Subscription string
	// User is the existing Pushover user key to migrate.
	User string
	// DeviceName optionally limits the subscription to one device.
	DeviceName string
	// Sound optionally sets the subscriber's preferred default sound.
	Sound string
}

// SubscriptionMigrateResponse is the response from POST /subscriptions/migrate.json.
type SubscriptionMigrateResponse struct {
	APIResponse
	// SubscribedUserKey is the key to store in place of the original user key.
	SubscribedUserKey string `json:"subscribed_user_key"`
}

// MigrateSubscriptionUser subscribes an existing Pushover user by migrating their user key.
// POST https://api.pushover.net/1/subscriptions/migrate.json
//
// On success the response includes subscribed_user_key. Pushover does not provide an
// API to list or unsubscribe subscribers; users unsubscribe themselves.
func (c *Client) MigrateSubscriptionUser(ctx context.Context, req *SubscriptionMigrateRequest) (*SubscriptionMigrateResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("subscription migrate request is nil")
	}
	if strings.TrimSpace(req.Subscription) == "" {
		return nil, fmt.Errorf("subscription code is required")
	}
	if strings.TrimSpace(req.User) == "" {
		return nil, fmt.Errorf("user key is required")
	}

	token := req.Token
	if token == "" {
		token = c.token
	}

	params := url.Values{}
	params.Set("token", token)
	params.Set("subscription", req.Subscription)
	params.Set("user", req.User)
	if req.DeviceName != "" {
		params.Set("device_name", req.DeviceName)
	}
	if req.Sound != "" {
		params.Set("sound", req.Sound)
	}

	var resp SubscriptionMigrateResponse
	if err := c.doPost(ctx, "/subscriptions/migrate.json", params, &resp); err != nil {
		return nil, err
	}
	resp.SubscribedUserKey = strings.TrimSpace(resp.SubscribedUserKey)
	if resp.SubscribedUserKey == "" {
		return nil, fmt.Errorf("pushover API error: migrate response missing subscribed_user_key")
	}
	return &resp, nil
}

// GlanceRequest holds fields for updating a Pushover glance widget.
//
// Only fields present in DataFields are sent. An empty string value clears a
// previously set field on the widget (per the Glances API).
// Valid data field keys: title, text, subtext, count, percent.
type GlanceRequest struct {
	Token      string
	User       string
	Device     string
	DataFields map[string]string
}

// UpdateGlance pushes data to a Pushover glance widget (smartwatch/lock screen).
// POST https://api.pushover.net/1/glances.json
func (c *Client) UpdateGlance(ctx context.Context, req *GlanceRequest) (*APIResponse, error) {
	token := req.Token
	if token == "" {
		token = c.token
	}

	params := url.Values{}
	params.Set("token", token)
	params.Set("user", req.User)
	if req.Device != "" {
		params.Set("device", req.Device)
	}
	for k, v := range req.DataFields {
		params.Set(k, v)
	}

	var resp APIResponse
	if err := c.doPost(ctx, "/glances.json", params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetLicenses retrieves the number of remaining license credits for an application.
// GET https://api.pushover.net/1/licenses.json?token=YOUR_API_TOKEN
func (c *Client) GetLicenses(ctx context.Context, token ...string) (*LicensesResponse, error) {
	tok := strings.TrimSpace(c.token)
	if len(token) > 0 && strings.TrimSpace(token[0]) != "" {
		tok = strings.TrimSpace(token[0])
	}
	if tok == "" {
		return nil, fmt.Errorf("application token is required")
	}
	path := fmt.Sprintf("/licenses.json?token=%s", url.QueryEscape(tok))
	var resp LicensesResponse
	if err := c.doGet(ctx, path, &resp); err != nil {
		return nil, redactSecret(err, tok)
	}
	return &resp, nil
}

// AssignLicense permanently assigns one pre-paid license credit to a Pushover user or email.
// POST https://api.pushover.net/1/licenses/assign.json
//
// Per Pushover's Licensing API guidance, license assignment requests are never
// automatically retried so transient failures cannot consume multiple credits.
func (c *Client) AssignLicense(ctx context.Context, req *LicenseAssignRequest) (*LicensesResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("license assign request is nil")
	}
	tok := strings.TrimSpace(req.Token)
	if tok == "" {
		tok = strings.TrimSpace(c.token)
	}
	if tok == "" {
		return nil, fmt.Errorf("application token is required")
	}

	user := strings.TrimSpace(req.User)
	email := strings.TrimSpace(req.Email)
	if user == "" && email == "" {
		return nil, fmt.Errorf("either user or email is required")
	}
	if user != "" && email != "" {
		return nil, fmt.Errorf("cannot specify both user and email")
	}

	osVal := strings.TrimSpace(req.OS)
	if osVal != "" {
		switch osVal {
		case "Android", "iOS", "Desktop":
		default:
			return nil, fmt.Errorf("invalid os %q: must be Android, iOS, or Desktop", osVal)
		}
	}

	params := url.Values{}
	params.Set("token", tok)
	if user != "" {
		params.Set("user", user)
	}
	if email != "" {
		params.Set("email", email)
	}
	if osVal != "" {
		params.Set("os", osVal)
	}

	var resp LicensesResponse
	if err := c.doPostNoRetry(ctx, "/licenses/assign.json", params, &resp); err != nil {
		return nil, redactSecret(err, tok)
	}
	return &resp, nil
}

func (c *Client) doPost(ctx context.Context, path string, params url.Values, out interface{}) error {
	return c.doWithRetry(ctx, http.MethodPost, path, params.Encode(), "application/x-www-form-urlencoded", out)
}

func (c *Client) doPostNoRetry(ctx context.Context, path string, params url.Values, out interface{}) error {
	return c.doWithMaxRetries(ctx, 0, http.MethodPost, path, params.Encode(), "application/x-www-form-urlencoded", out)
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
	return c.doWithMaxRetries(ctx, c.maxRetriesOrDefault(), method, path, body, contentType, out)
}

func (c *Client) doWithMaxRetries(ctx context.Context, maxRetries int, method, path, body, contentType string, out interface{}) error {
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

		return unmarshalResponse(resp.StatusCode, respBody, out)
	}

	return fmt.Errorf("pushover API request failed")
}

// parseAPIStatusAndErrors extracts the Pushover status field and any error messages
// whether "errors" is returned as a JSON array (["msg"]) or an attribute map
// ({"token":["is out of available license credits"]}).
func parseAPIStatusAndErrors(body []byte) (int, []string, bool) {
	var raw struct {
		Status *int            `json:"status"`
		Errors json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return 0, nil, false
	}
	status := 0
	if raw.Status != nil {
		status = *raw.Status
	}
	var errs []string
	if len(raw.Errors) > 0 {
		var list []string
		if err := json.Unmarshal(raw.Errors, &list); err == nil {
			errs = list
		} else {
			var obj map[string][]string
			if err := json.Unmarshal(raw.Errors, &obj); err == nil && len(obj) > 0 {
				keys := make([]string, 0, len(obj))
				for k := range obj {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					for _, msg := range obj[k] {
						errs = append(errs, fmt.Sprintf("%s: %s", k, msg))
					}
				}
			}
		}
	}
	return status, errs, raw.Status != nil || len(errs) > 0
}

func unmarshalResponse(statusCode int, body []byte, out interface{}) error {
	if statusCode < 200 || statusCode >= 300 {
		if _, errs, _ := parseAPIStatusAndErrors(body); len(errs) > 0 {
			return fmt.Errorf("pushover API error: %s", strings.Join(errs, "; "))
		}
		trimmed := strings.TrimSpace(string(body))
		if len(trimmed) > 200 {
			trimmed = trimmed[:200] + "..."
		}
		if trimmed == "" {
			return fmt.Errorf("pushover API returned HTTP %d", statusCode)
		}
		return fmt.Errorf("pushover API returned HTTP %d: %s", statusCode, trimmed)
	}

	status, errs, _ := parseAPIStatusAndErrors(body)
	if err := json.Unmarshal(body, out); err != nil {
		if len(errs) > 0 {
			return fmt.Errorf("pushover API error: %s", strings.Join(errs, "; "))
		}
		return fmt.Errorf("decoding response: %w", err)
	}

	if status != 1 {
		if len(errs) > 0 {
			return fmt.Errorf("pushover API error: %s", strings.Join(errs, "; "))
		}
		return fmt.Errorf("pushover API error (status %d)", status)
	}

	return nil
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
