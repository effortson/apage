// Package gatewayclient lets the API request tunnel file streams from the
// gateway over an internal HTTP endpoint (spec §19.4 routing). In single-box
// there is one gateway; in multi-region the API resolves the gateway serving
// the instance from the Redis registry.
package gatewayclient

import (
	"io"
	"net/http"
	"net/url"
)

// Client forwards stream requests to a gateway's internal endpoint.
type Client struct {
	baseURL string
	secret  string // shared secret for the gateway internal endpoint (spec §19.4)
	http    *http.Client
}

// New builds a gateway client. secret authenticates the internal stream
// endpoint (security review #3); empty in dev/single-box.
func New(baseURL, secret string) *Client {
	return &Client{
		baseURL: baseURL,
		secret:  secret,
		http:    &http.Client{Timeout: 0}, // streaming: no overall timeout
	}
}

// StreamFile asks the gateway at gatewayURL (or the configured fallback) to
// stream a tunnel file to w, forwarding Range and relaying status/headers/body.
// Implements api.GatewayClient.
func (c *Client) StreamFile(w http.ResponseWriter, r *http.Request, gatewayURL, instanceID, fileRef string) error {
	base := gatewayURL
	if base == "" {
		base = c.baseURL
	}
	q := url.Values{}
	q.Set("instance", instanceID)
	q.Set("fileRef", fileRef)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet,
		base+"/internal/v1/stream?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	if rng := r.Header.Get("Range"); rng != "" {
		req.Header.Set("Range", rng)
	}
	if c.secret != "" {
		req.Header.Set("X-Apage-Internal", c.secret)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Relay upstream content headers (Content-Type already set by caller for
	// security; here we add length/range from the agent).
	for _, h := range []string{"Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" && w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	return err
}
