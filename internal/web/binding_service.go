package web

import (
	"context"
	"errors"
	"net/http"

	"simple_gateway_by_codex/internal/authclient"
)

type authClientBindingVerifier struct {
	client *authclient.Client
}

// NewBindingVerifier creates the service group verifier used during registration and route validation.
func NewBindingVerifier(client *authclient.Client) bindingVerifier {
	return authClientBindingVerifier{client: client}
}

func (v authClientBindingVerifier) ValidateServiceGroup(ctx context.Context, serviceGroupName, authorizationCode string) error {
	token, err := v.client.LatestServiceGroupToken(ctx, serviceGroupName, authorizationCode)
	if err != nil {
		return authClientErrorToAppError(err)
	}
	if token.AccessToken == "" {
		return errUnauthorized("鉴权服务未返回有效服务组 token")
	}
	return nil
}

func (v authClientBindingVerifier) ValidateManagedService(ctx context.Context, serviceGroupName, authorizationCode, authServiceName string) error {
	token, err := v.client.LatestServiceGroupToken(ctx, serviceGroupName, authorizationCode)
	if err != nil {
		return authClientErrorToAppError(err)
	}
	if token.AccessToken == "" {
		return errUnauthorized("鉴权服务未返回有效服务组 token")
	}
	result, err := v.client.Verify(ctx, authclient.VerifyHeaders{
		ServiceName:       serviceGroupName,
		TargetServiceName: authServiceName,
		AccessToken:       token.AccessToken,
	})
	if err != nil {
		return authClientErrorToAppError(err)
	}
	if !result.OK {
		return errForbidden("绑定服务组无权限管理该鉴权服务")
	}
	return nil
}

func authClientErrorToAppError(err error) error {
	var authErr authclient.Error
	if errors.As(err, &authErr) {
		switch authErr.StatusCode {
		case http.StatusBadRequest:
			return errBadRequest("鉴权服务拒绝请求参数")
		case http.StatusUnauthorized:
			return errUnauthorized("服务组名称或永久授权码不匹配")
		case http.StatusForbidden:
			return errForbidden("服务组无权限或来源不匹配")
		case http.StatusNotFound:
			return appError{status: http.StatusNotFound, code: "auth_service_not_found", message: "服务组或服务不存在"}
		case http.StatusTooManyRequests:
			return appError{status: http.StatusTooManyRequests, code: "auth_service_rate_limited", message: "鉴权服务限流，请稍后重试"}
		default:
			return appError{status: http.StatusBadGateway, code: "auth_service_error", message: "鉴权服务异常"}
		}
	}
	return errInternal(err)
}
