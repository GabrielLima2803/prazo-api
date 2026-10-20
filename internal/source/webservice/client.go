package webservice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"strings"

	"prazo/internal/httpx"
)

func NewClient(baseURL string, http *httpx.Client) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: http}
}

func (c *Client) BaseURL() string { return c.baseURL }

// Call chama uma função REST e decodifica a resposta em out. O token vai no
// corpo do POST, nunca na URL.
func (c *Client) Call(ctx context.Context, token, function string, args url.Values, out any) error {
	form := url.Values{"wstoken": {token}, "wsfunction": {function}, "moodlewsrestformat": {"json"}}
	maps.Copy(form, args)

	body, err := c.http.PostForm(ctx, c.baseURL+"/webservice/rest/server.php", form)
	if err != nil {
		return fmt.Errorf("%s: %w", function, err)
	}
	if bytes.HasPrefix(bytes.TrimSpace(body), []byte("{")) {
		var e exceptionResponse
		if json.Unmarshal(body, &e) == nil && e.Exception != "" {
			return &MoodleError{Function: function, Code: e.ErrorCode, Message: e.Message}
		}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s: %w: %v", function, ErrUnexpectedResponse, err)
	}
	return nil
}

// callAJAX chama um endpoint público (sem login) de lib/ajax/service-nologin.php.
func (c *Client) callAJAX(ctx context.Context, method string, out any) error {
	req, err := json.Marshal([]map[string]any{{"index": 0, "methodname": method, "args": map[string]any{}}})
	if err != nil {
		return err
	}
	body, err := c.http.PostJSON(ctx, c.baseURL+"/lib/ajax/service-nologin.php?info="+url.QueryEscape(method), req)
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	var resp []struct {
		Error bool            `json:"error"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || len(resp) != 1 || resp[0].Error {
		return fmt.Errorf("%s: %w", method, ErrUnexpectedResponse)
	}
	if err := json.Unmarshal(resp[0].Data, out); err != nil {
		return fmt.Errorf("%s: %w: %v", method, ErrUnexpectedResponse, err)
	}
	return nil
}
