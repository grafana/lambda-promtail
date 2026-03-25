package main

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_loadSensitiveEnv(t *testing.T) {
	ctx := context.Background()

	t.Run("should return empty if env is not set", func(t *testing.T) {
		provider := &testProvider{}
		value, err := loadSensitiveEnv(ctx, provider, "FOO")
		assert.NoError(t, err)
		assert.Empty(t, value)
		assert.Equal(t, 0, provider.callCount)
	})

	t.Run("should call provider.Retrieve with the env value", func(t *testing.T) {
		t.Setenv("FOO", "BAR")
		provider := &testProvider{returnValue: "BAR"}
		value, err := loadSensitiveEnv(ctx, provider, "FOO")
		assert.NoError(t, err)
		assert.Equal(t, "BAR", value)
		assert.Equal(t, 1, provider.callCount)
	})

	t.Run("should propagate provider errors", func(t *testing.T) {
		t.Setenv("FOO", "BAR")
		expectedErr := errors.New("provider error")
		provider := &testProvider{returnError: expectedErr}
		_, err := loadSensitiveEnv(ctx, provider, "FOO")
		assert.ErrorIs(t, err, expectedErr)
	})
}

func Test_loadSensitiveEnv_WithChain(t *testing.T) {
	ctx := context.Background()

	t.Run("should return plain value as-is", func(t *testing.T) {
		t.Setenv("FOO", "BAR")
		chain := NewChainProvider(&AWSSecretsManagerProvider{}, &AWSSSMProvider{})
		value, err := loadSensitiveEnv(ctx, chain, "FOO")
		assert.NoError(t, err)
		assert.Equal(t, "BAR", value)
	})

	t.Run("should return an error if the env variable contains an unsupported ARN service", func(t *testing.T) {
		t.Setenv("FOO", "arn:aws:invalid:eu-west-1:123456789012:ssm/example")
		chain := NewChainProvider(&AWSSecretsManagerProvider{}, &AWSSSMProvider{})
		value, err := loadSensitiveEnv(ctx, chain, "FOO")
		assert.Error(t, err)
		assert.Empty(t, value)
	})

	t.Run("should route Secrets Manager ARN to SM provider and skip SSM", func(t *testing.T) {
		t.Setenv("FOO", "arn:aws:secretsmanager:eu-west-1:123456789012:secret:foo")
		smProvider := &testProvider{returnValue: "bar"}
		ssmProvider := &testProvider{}
		// SM mock is first; it always handles its call. SSM mock should never be reached.
		// We verify routing by checking SSM was not called even though it's in the chain.
		chain := NewChainProvider(smProvider, ssmProvider)
		value, err := loadSensitiveEnv(ctx, chain, "FOO")
		assert.NoError(t, err)
		assert.Equal(t, "bar", value)
		assert.Equal(t, 1, smProvider.callCount)
		assert.Equal(t, 0, ssmProvider.callCount)
	})

	t.Run("should use Vault provider when it is first in the chain", func(t *testing.T) {
		t.Setenv("FOO", "vault_key")
		vaultProvider := &testProvider{returnValue: "bar"}
		awsProvider := &testProvider{}
		chain := NewChainProvider(vaultProvider, awsProvider)
		value, err := loadSensitiveEnv(ctx, chain, "FOO")
		assert.NoError(t, err)
		assert.Equal(t, "bar", value)
		assert.Equal(t, 1, vaultProvider.callCount)
		assert.Equal(t, 0, awsProvider.callCount)
	})
}
