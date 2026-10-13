package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Provider struct {
	Name, ClientID, ClientSecret, RedirectURI, AuthorizeEndpoint, TokenEndpoint, UserEndpoint string
	Client                                                                                    *http.Client
}

func (p *Provider) AuthorizationURL(state, challenge string) string {
	q := url.Values{"client_id": {p.ClientID}, "response_type": {"code"}, "redirect_uri": {p.RedirectURI}, "state": {state}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	if p.Name == "yandex" {
		q.Set("scope", "login:info login:email login:avatar")
	} else {
		q.Set("scope", "email")
		q.Set("code_challenge_method", "s256")
	}
	return p.AuthorizeEndpoint + "?" + q.Encode()
}
func (p *Provider) request(ctx context.Context, method, endpoint string, form url.Values, authorization string, out any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return errors.New("provider request failed")
	}
	req.Header.Set("Accept", "application/json")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	res, err := client.Do(req)
	if err != nil {
		return errors.New("provider unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return errors.New("provider rejected request")
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out); err != nil {
		return errors.New("invalid provider response")
	}
	return nil
}
func (p *Provider) Identity(ctx context.Context, code, verifier, state, device string) (repository.Identity, error) {
	i := repository.Identity{Provider: p.Name}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {p.ClientID}, "redirect_uri": {p.RedirectURI}, "code_verifier": {verifier}}
	if p.ClientSecret != "" {
		form.Set("client_secret", p.ClientSecret)
	}
	if p.Name == "vk" {
		if device == "" || len(device) > 1024 {
			return i, errors.New("device id required")
		}
		form.Set("device_id", device)
		form.Set("state", state)
	}
	var token struct {
		AccessToken string          `json:"access_token"`
		State       string          `json:"state"`
		UserID      json.Number     `json:"user_id"`
		Error       json.RawMessage `json:"error"`
	}
	if err := p.request(ctx, "POST", p.TokenEndpoint, form, "", &token); err != nil {
		return i, err
	}
	if token.AccessToken == "" || len(token.Error) > 0 {
		return i, errors.New("invalid token response")
	}
	if p.Name == "vk" && token.State != state {
		return i, errors.New("token state mismatch")
	}
	if p.Name == "yandex" {
		var raw struct {
			ID       string `json:"id"`
			Email    string `json:"default_email"`
			Name     string `json:"display_name"`
			RealName string `json:"real_name"`
			Avatar   string `json:"default_avatar_id"`
			Empty    bool   `json:"is_avatar_empty"`
		}
		if err := p.request(ctx, "GET", p.UserEndpoint+"?format=json", nil, "OAuth "+token.AccessToken, &raw); err != nil {
			return i, err
		}
		i.Subject = raw.ID
		i.Email = raw.Email
		i.Name = raw.RealName
		if i.Name == "" {
			i.Name = raw.Name
		}
		if !raw.Empty && raw.Avatar != "" {
			i.AvatarURL = "https://avatars.yandex.net/get-yapic/" + url.PathEscape(raw.Avatar) + "/islands-200"
		}
	} else {
		var profile struct {
			User struct {
				ID     json.Number `json:"user_id"`
				Email  string      `json:"email"`
				First  string      `json:"first_name"`
				Last   string      `json:"last_name"`
				Avatar string      `json:"avatar"`
			} `json:"user"`
		}
		if err := p.request(ctx, "POST", p.UserEndpoint, url.Values{"client_id": {p.ClientID}, "access_token": {token.AccessToken}}, "", &profile); err != nil {
			return i, err
		}
		i.Subject = profile.User.ID.String()
		if token.UserID.String() != "" && token.UserID.String() != i.Subject {
			return i, errors.New("subject mismatch")
		}
		i.Email = profile.User.Email
		i.Name = strings.TrimSpace(profile.User.First + " " + profile.User.Last)
		i.AvatarURL = profile.User.Avatar
	}
	if i.Subject == "" {
		return i, errors.New("missing provider subject")
	}
	if len(i.Name) > 100 {
		i.Name = string([]rune(i.Name)[:min(100, len([]rune(i.Name)))])
	}
	if i.AvatarURL != "" {
		u, err := url.Parse(i.AvatarURL)
		if err != nil || u.Scheme != "https" || u.User != nil || len(i.AvatarURL) > 2048 {
			i.AvatarURL = ""
		}
	}
	return i, nil
}
