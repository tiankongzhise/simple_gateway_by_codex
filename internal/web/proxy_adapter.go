package web

import (
	"context"

	"simple_gateway_by_codex/internal/authclient"
	"simple_gateway_by_codex/internal/proxy"
)

type proxyAuthAdapter struct {
	client *authclient.Client
}

func newProxyAuthAdapter(client *authclient.Client) proxy.AuthClient {
	return proxyAuthAdapter{client: client}
}

func (a proxyAuthAdapter) LatestServiceGroupToken(ctx context.Context, serviceGroupName, authorizationCode string) (proxy.GroupToken, error) {
	token, err := a.client.LatestServiceGroupToken(ctx, serviceGroupName, authorizationCode)
	if err != nil {
		return proxy.GroupToken{}, err
	}
	return proxy.GroupToken{AccessToken: token.AccessToken}, nil
}

func (a proxyAuthAdapter) Verify(ctx context.Context, headers proxy.VerifyHeaders) (proxy.VerifyResult, error) {
	result, err := a.client.Verify(ctx, authclient.VerifyHeaders{
		ServiceName:       headers.ServiceName,
		TargetServiceName: headers.TargetServiceName,
		AccessToken:       headers.AccessToken,
		Origin:            headers.Origin,
		Referer:           headers.Referer,
		Model:             headers.Model,
	})
	if err != nil {
		return proxy.VerifyResult{}, err
	}
	return proxy.VerifyResult{OK: result.OK}, nil
}
