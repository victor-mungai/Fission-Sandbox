package auth

import (
	"crypto/hmac"
	"net/http"
)

const headerName = "x-sandbox-auth"

type Middleware struct {
	token string
}

func New(token string) *Middleware {
	return &Middleware{
		token: token,
	}
}

func (a *Middleware) Validate(r *http.Request) bool {
	provided := r.Header.Get(headerName)
	if provided == "" || a.token == "" {
		return false
	}

	return hmac.Equal([]byte(provided), []byte(a.token))
}
