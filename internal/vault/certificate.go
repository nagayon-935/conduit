package vault

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// httpDoer is a minimal interface around *http.Client so it can be swapped in tests.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

const (
	defaultHTTPTimeout = 15 * time.Second
	certTTL            = "5m"
)

func newHTTPClient(timeout time.Duration) httpDoer {
	return &http.Client{Timeout: timeout}
}

// signRequest is the JSON body sent to Vault's SSH sign endpoint.
type signRequest struct {
	PublicKey       string `json:"public_key"`
	TTL             string `json:"ttl"`
	ValidPrincipals string `json:"valid_principals"`
}

// vaultResponse is the outer envelope returned by Vault.
type vaultResponse struct {
	Data   *signData `json:"data"`
	Errors []string  `json:"errors"`
}

// signData holds the signed certificate returned in data.signed_key.
type signData struct {
	SignedKey string `json:"signed_key"`
}

// SignPublicKey calls POST /v1/{mount}/sign/{role} and returns the signed certificate.
func (c *Client) SignPublicKey(ctx context.Context, publicKey string, validPrincipal string) (string, error) {
	endpoint := fmt.Sprintf("%s/v1/%s/sign/%s", strings.TrimRight(c.addr, "/"), c.sshMount, c.sshRole)

	body := signRequest{
		PublicKey:       strings.TrimSpace(publicKey),
		TTL:             certTTL,
		ValidPrincipals: validPrincipal,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("vault: failed to marshal sign request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("vault: failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Vault-Token", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("vault: HTTP request to %s failed: %w", endpoint, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("vault: failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Try to parse Vault error messages.
		var errResp vaultResponse
		if jsonErr := json.Unmarshal(respBytes, &errResp); jsonErr == nil && len(errResp.Errors) > 0 {
			return "", fmt.Errorf("vault: sign request failed (HTTP %d): %s", resp.StatusCode, strings.Join(errResp.Errors, "; "))
		}
		return "", fmt.Errorf("vault: sign request failed (HTTP %d): %s", resp.StatusCode, string(respBytes))
	}

	var vr vaultResponse
	if err := json.Unmarshal(respBytes, &vr); err != nil {
		return "", fmt.Errorf("vault: failed to parse sign response: %w", err)
	}

	if vr.Data == nil || strings.TrimSpace(vr.Data.SignedKey) == "" {
		return "", fmt.Errorf("vault: sign response contained no signed_key")
	}

	return strings.TrimSpace(vr.Data.SignedKey), nil
}
