package authclient

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

// Client calls the external auth-service public APIs.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New creates an auth-service client.
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// GroupToken is the token/latest response.
type GroupToken struct {
	AccessToken               string `json:"accessToken"`
	AccessTokenExpiresAt      int64  `json:"accessTokenExpiresAt"`
	AccessTokenExpiresAtLocal string `json:"accessTokenExpiresAtLocal"`
}

// VerifyResult is the auth/verify response.
type VerifyResult struct {
	OK               bool   `json:"ok"`
	ServiceName      string `json:"serviceName"`
	ServiceGroupName string `json:"serviceGroupName,omitempty"`
}

// VerifyHeaders are forwarded to /api/auth/verify.
type VerifyHeaders struct {
	ServiceName       string
	TargetServiceName string
	AccessToken       string
	Origin            string
	Referer           string
	Model             string
}

// LatestServiceGroupToken exchanges a service group permanent code for the latest access token.
func (c *Client) LatestServiceGroupToken(ctx context.Context, serviceGroupName, authorizationCode string) (GroupToken, error) {
	var out GroupToken
	err := c.postJSON(ctx, "/api/service-groups/token/latest", map[string]string{
		"serviceGroupName":  serviceGroupName,
		"authorizationCode": authorizationCode,
	}, nil, &out)
	if err != nil {
		return GroupToken{}, err
	}
	return out, nil
}

// Verify verifies a caller service token or a service group token against a target service.
func (c *Client) Verify(ctx context.Context, headers VerifyHeaders) (VerifyResult, error) {
	httpHeaders := map[string]string{
		"Service-Name": headers.ServiceName,
		"Access-Token": headers.AccessToken,
	}
	if headers.TargetServiceName != "" {
		httpHeaders["Target-Service-Name"] = headers.TargetServiceName
	}
	if headers.Origin != "" {
		httpHeaders["Origin"] = headers.Origin
	}
	if headers.Referer != "" {
		httpHeaders["Referer"] = headers.Referer
	}
	if headers.Model != "" {
		httpHeaders["model"] = headers.Model
	}

	var out VerifyResult
	if err := c.postJSON(ctx, "/api/auth/verify", nil, httpHeaders, &out); err != nil {
		return VerifyResult{}, err
	}
	return out, nil
}

func (c *Client) postJSON(ctx context.Context, path string, body any, headers map[string]string, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal auth-service request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build auth-service request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		if strings.TrimSpace(value) != "" {
			req.Header.Set(key, value)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call auth-service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Error{StatusCode: resp.StatusCode, Message: authStatusMessage(resp.StatusCode)}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode auth-service response: %w", err)
	}
	return nil
}

// Error describes a non-success response from auth-service.
type Error struct {
	StatusCode int
	Message    string
}

func (e Error) Error() string {
	return fmt.Sprintf("auth-service returned %d: %s", e.StatusCode, e.Message)
}

// Status returns the HTTP status code from auth-service.
func (e Error) Status() int {
	return e.StatusCode
}

func authStatusMessage(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "请求参数无效"
	case http.StatusUnauthorized:
		return "授权码或 token 无效"
	case http.StatusForbidden:
		return "服务组无权限管理目标服务或来源不匹配"
	case http.StatusNotFound:
		return "服务组或服务不存在"
	case http.StatusConflict:
		return "鉴权服务数据冲突"
	case http.StatusTooManyRequests:
		return "鉴权服务限流"
	default:
		return "鉴权服务异常"
	}
}
