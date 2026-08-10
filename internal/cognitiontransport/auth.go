package cognitiontransport

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
)

type Authenticator func(*http.Request) error

func NewBearerAuthenticator(expected string) (Authenticator, error) {
	if expected == "" || expected != strings.TrimSpace(expected) || len(expected) > 4096 {
		return nil, fmt.Errorf("cognition environment bearer token is invalid")
	}
	return func(request *http.Request) error {
		provided := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		if len(provided) != len(expected) || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			return ErrAuthentication
		}
		return nil
	}, nil
}
