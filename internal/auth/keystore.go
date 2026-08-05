// Package auth resolves the active environment (base-URL + API key) and stores
// API keys in the OS keychain. The NORMATIK_API_KEY env var has strict
// precedence over the keychain so CI/headless/agent use never needs a keychain.
package auth

import (
	"errors"

	"github.com/zalando/go-keyring"
)

// Service is the keychain service name under which keys are stored, keyed by
// profile name.
const Service = "normatik-cli"

// SetKey stores the API key for a profile in the OS keychain.
func SetKey(profile, key string) error { return keyring.Set(Service, profile, key) }

// GetKey reads the API key for a profile from the OS keychain.
func GetKey(profile string) (string, error) { return keyring.Get(Service, profile) }

// DeleteKey removes the stored API key for a profile.
func DeleteKey(profile string) error { return keyring.Delete(Service, profile) }

// DeleteKeyIfPresent removes the stored API key for a profile; an absent key is
// not an error (used by `auth remove`, where the profile may never have had a
// key or was already logged out).
func DeleteKeyIfPresent(profile string) error {
	err := keyring.Delete(Service, profile)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
