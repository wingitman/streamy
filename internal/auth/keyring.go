package auth

import "github.com/zalando/go-keyring"

type OSKeyring struct{}

func (OSKeyring) Get(service, user string) (string, error) {
	return keyring.Get(service, user)
}

func (OSKeyring) Set(service, user, secret string) error {
	return keyring.Set(service, user, secret)
}

func (OSKeyring) Delete(service, user string) error {
	return keyring.Delete(service, user)
}
