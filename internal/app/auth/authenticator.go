package auth

import "github.com/radynsade/faryengo/internal/security"

type Authenticator struct {
	sessionResolver security.SessionResolver
}

func (a *Authenticator) Authenticate() {

}
