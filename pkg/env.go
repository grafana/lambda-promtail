package main

import (
	"context"
	"errors"
	"os"
)

// ErrNotApplicable is returned by a Provider when it cannot handle the given key.
var ErrNotApplicable = errors.New("provider not applicable for this key")

// Provider retrieves a secret value for the given key or reference.
// It returns ErrNotApplicable if it cannot handle the key.
type Provider interface {
	Retrieve(ctx context.Context, key string) (string, error)
}

func loadSensitiveEnv(ctx context.Context, provider Provider, name string) (string, error) {
	envValue, ok := os.LookupEnv(name)
	if !ok {
		return "", nil
	}
	return provider.Retrieve(ctx, envValue)
}
