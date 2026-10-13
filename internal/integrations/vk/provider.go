package vk

import "github.com/NIDJEL/assistant-ai/internal/integrations/oauth"

func New(id, secret, redirect string) *oauth.Provider {
	return &oauth.Provider{Name: "vk", ClientID: id, ClientSecret: secret, RedirectURI: redirect, AuthorizeEndpoint: "https://id.vk.ru/authorize", TokenEndpoint: "https://id.vk.ru/oauth2/auth", UserEndpoint: "https://id.vk.ru/oauth2/user_info"}
}
