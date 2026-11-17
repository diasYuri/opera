package app

import "context"

// LoginProvider describes an available sign-in flow, independently of its UI.
type LoginProvider struct {
	ID, Name, Description string
}

type LoginNotice struct {
	URL, Message string
	Prompt       *LoginPrompt `json:"-"`
}

type LoginOption struct{ ID, Name, Description string }

// LoginPrompt requests an explicit choice while a sign-in remains in progress.
// Responses and credentials are never serialized into execution metadata.
type LoginPrompt struct {
	Title    string
	Options  []LoginOption
	Response chan<- string `json:"-"`
}

type Authenticator interface {
	LoginProviders() []LoginProvider
	Login(context.Context, string, func(LoginNotice) error) error
}
