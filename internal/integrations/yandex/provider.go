package yandex

import "github.com/NIDJEL/assistant-ai/internal/integrations/oauth"

func New(id, secret, redirect string) *oauth.Provider {
	return &oauth.Provider{Name: "yandex", ClientID: id, ClientSecret: secret, RedirectURI: redirect, AuthorizeEndpoint: "https://oauth.yandex.ru/authorize", TokenEndpoint: "https://oauth.yandex.ru/token", UserEndpoint: "https://login.yandex.ru/info"}
}
