package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
)

// apiClient talks to the apage-api data plane using an instance api key. It is
// the bridge between the MCP tools and the platform: upload a local file to cloud
// storage, then create/manage cloud preview links.
type apiClient struct {
	base   string
	apiKey string
	hc     *http.Client
}

func newAPIClient(base, apiKey string) *apiClient {
	return &apiClient{base: base, apiKey: apiKey, hc: &http.Client{Timeout: 60 * time.Second}}
}

// LinkSummary is the subset of a preview link surfaced to the agent.
type LinkSummary struct {
	LinkID      string     `json:"linkId"`
	DisplayName string     `json:"displayName"`
	Mode        string     `json:"mode"`
	FileID      *string    `json:"fileId,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt"`
	RevokedAt   *time.Time `json:"revokedAt"`
	FrozenAt    *time.Time `json:"frozenAt"`
	ViewCount   int64      `json:"viewCount"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// doJSON performs an authenticated JSON request and decodes the response into out
// (out may be nil). Non-2xx responses become an error carrying the body.
func (a *apiClient) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.base+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "cli-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("apage-api %s %s: status %d: %s", method, path, resp.StatusCode, string(data))
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// uploadFile presigns, PUTs the local file to object storage, and completes the
// upload. It returns the cloud file id (not yet scanned/ready).
func (a *apiClient) uploadFile(ctx context.Context, path, displayName, mime string, expiresInSeconds int64) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	var presign struct {
		UploadURL string            `json:"uploadUrl"`
		FileID    string            `json:"fileId"`
		Headers   map[string]string `json:"headers"`
	}
	if err := a.doJSON(ctx, http.MethodPost, "/api/v1/uploads/presign", map[string]any{
		"fileName": displayName, "mimeType": mime, "size": fi.Size(), "expiresInSeconds": expiresInSeconds,
	}, &presign); err != nil {
		return "", err
	}

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	put, err := http.NewRequestWithContext(ctx, http.MethodPut, presign.UploadURL, f)
	if err != nil {
		return "", err
	}
	put.ContentLength = fi.Size()
	for k, v := range presign.Headers {
		put.Header.Set(k, v)
	}
	putResp, err := a.hc.Do(put)
	if err != nil {
		return "", err
	}
	defer putResp.Body.Close()
	if putResp.StatusCode >= 300 {
		b, _ := io.ReadAll(putResp.Body)
		return "", fmt.Errorf("object upload failed: status %d: %s", putResp.StatusCode, string(b))
	}

	// Complete: the server re-stats the object and trusts its actual size.
	if err := a.doJSON(ctx, http.MethodPost, "/api/v1/uploads/"+presign.FileID+"/complete",
		map[string]any{"size": fi.Size()}, nil); err != nil {
		return "", err
	}
	return presign.FileID, nil
}

// waitReady polls the file until it finishes scanning. Returns nil once ready, or
// an error if it is rejected/failed or the timeout elapses.
func (a *apiClient) waitReady(ctx context.Context, fileID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		var f struct {
			Status       string `json:"status"`
			RejectReason string `json:"rejectReason"`
		}
		if err := a.doJSON(ctx, http.MethodGet, "/api/v1/files/"+fileID, nil, &f); err != nil {
			return err
		}
		switch f.Status {
		case "ready":
			return nil
		case "rejected", "failed", "expired", "deleted":
			reason := f.RejectReason
			if reason == "" {
				reason = f.Status
			}
			return fmt.Errorf("file not previewable: %s", reason)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for file %s to be ready (status=%s)", fileID, f.Status)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(750 * time.Millisecond):
		}
	}
}

// createLink creates a cloud preview link backed by fileID.
func (a *apiClient) createLink(ctx context.Context, fileID, displayName string, expiresInSeconds int64, accessPolicy json.RawMessage, password string) (url, linkID string, expiresAt *time.Time, err error) {
	body := map[string]any{"mode": "cloud", "fileId": fileID, "expiresInSeconds": expiresInSeconds}
	if displayName != "" {
		body["displayName"] = displayName
	}
	if len(accessPolicy) > 0 {
		body["accessPolicy"] = accessPolicy
	}
	if password != "" {
		body["password"] = password
	}
	var out struct {
		LinkID    string     `json:"linkId"`
		URL       string     `json:"url"`
		ExpiresAt *time.Time `json:"expiresAt"`
	}
	if err = a.doJSON(ctx, http.MethodPost, "/api/v1/preview-links", body, &out); err != nil {
		return "", "", nil, err
	}
	return out.URL, out.LinkID, out.ExpiresAt, nil
}

// listLinks returns the tenant's links (first page).
func (a *apiClient) listLinks(ctx context.Context) ([]LinkSummary, error) {
	var out struct {
		Items []LinkSummary `json:"items"`
	}
	if err := a.doJSON(ctx, http.MethodGet, "/api/v1/preview-links", nil, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// revokeLink revokes a link.
func (a *apiClient) revokeLink(ctx context.Context, linkID string) error {
	return a.doJSON(ctx, http.MethodPost, "/api/v1/preview-links/"+linkID+"/revoke", map[string]any{}, nil)
}

// updateLink modifies an existing link in place (PATCH). Only non-nil fields are
// sent so the server leaves the rest unchanged.
func (a *apiClient) updateLink(ctx context.Context, linkID string, body map[string]any) error {
	return a.doJSON(ctx, http.MethodPatch, "/api/v1/preview-links/"+linkID, body, nil)
}
