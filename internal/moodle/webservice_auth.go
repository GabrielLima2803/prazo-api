package moodle

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
)

func (c *WebServiceClient) PublicConfig(ctx context.Context) (PublicConfig, error) {
	var pc PublicConfig
	err := c.callAJAX(ctx, "tool_mobile_get_public_config", &pc)
	return pc, err
}

// Login troca usuário e senha por um token. A senha não é guardada.
func (c *WebServiceClient) Login(ctx context.Context, username, password string) (string, error) {
	r, err := c.requestToken(ctx, username, password)
	if err != nil {
		return "", fmt.Errorf("login: %w", err)
	}
	if r.Token == "" {
		return "", fmt.Errorf("login: %w", loginError(r.ErrorCode, r.Error))
	}
	return r.Token, nil
}

// ProbeTokenEndpoint descobre se /login/token.php aceita login sem usar uma
// credencial real: tenta um usuário aleatório e interpreta o código de erro.
func (c *WebServiceClient) ProbeTokenEndpoint(ctx context.Context) (TokenProbe, error) {
	r, err := c.requestToken(ctx, "prazo-doctor-"+rand.Text()[:10], rand.Text())
	if err != nil {
		return TokenProbe{}, fmt.Errorf("probe token.php: %w", err)
	}
	return TokenProbe{
		Available: errors.Is(loginError(r.ErrorCode, r.Error), ErrInvalidLogin),
		ErrorCode: r.ErrorCode,
		Message:   r.Error,
	}, nil
}

func (c *WebServiceClient) requestToken(ctx context.Context, username, password string) (tokenResponse, error) {
	form := url.Values{"username": {username}, "password": {password}, "service": {MobileService}}
	body, err := c.http.PostForm(ctx, c.baseURL+"/login/token.php", form)
	if err != nil {
		return tokenResponse{}, err
	}
	var r tokenResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return tokenResponse{}, ErrUnexpectedResponse
	}
	return r, nil
}
