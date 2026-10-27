package google

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GabrielLima2803/prazo-sync/internal/store"
)

// RevokeURL é o endpoint do Google que invalida um token (e todos os tokens
// da mesma autorização).
const RevokeURL = "https://oauth2.googleapis.com/revoke"

const revokeTimeout = 15 * time.Second

// ErrRevoke indica que o token foi apagado daqui, mas o Google não confirmou
// a revogação. O usuário ainda pode remover o acesso em
// https://myaccount.google.com/permissions.
var ErrRevoke = errors.New("não foi possível revogar o acesso no Google")

// Revoker revoga tokens no Google. endpoint e client existem para os testes.
type Revoker struct {
	endpoint string
	client   *http.Client
}

func NewRevoker(client *http.Client) *Revoker {
	if client == nil {
		client = &http.Client{Timeout: revokeTimeout}
	}
	return &Revoker{endpoint: RevokeURL, client: client}
}

// Forget revoga no Google o token guardado no KV e o apaga daqui. O token
// local é apagado mesmo se o Google não responder: nesse caso o erro
// devolvido é ErrRevoke, que o chamador registra e segue.
func (r *Revoker) Forget(ctx context.Context, kv store.KV) error {
	st, err := LoadToken(ctx, kv)
	if errors.Is(err, ErrNotAuthorized) {
		return nil
	}
	var revokeErr error
	if err == nil && st.Token != nil {
		tok := st.Token.RefreshToken
		if tok == "" {
			tok = st.Token.AccessToken
		}
		revokeErr = r.revoke(ctx, tok)
	}
	if err := kv.Delete(ctx, store.KeyGoogleToken); err != nil {
		return err
	}
	return revokeErr
}

func (r *Revoker) revoke(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, revokeTimeout)
	defer cancel()
	form := url.Values{"token": {token}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, strings.NewReader(form))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRevoke, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := r.client.Do(req)
	if err != nil {
		// Sem a URL: o erro de rede do net/http a repetiria.
		return ErrRevoke
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	// 400 = token já inválido (revogado ou expirado): o objetivo foi atingido.
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusBadRequest {
		return nil
	}
	return fmt.Errorf("%w (HTTP %d)", ErrRevoke, resp.StatusCode)
}
