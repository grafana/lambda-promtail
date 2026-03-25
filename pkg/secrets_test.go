package main

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

var _ Provider = &testProvider{}

// testProvider is a configurable mock Provider for use in tests.
type testProvider struct {
	callCount   int
	returnValue string
	returnError error
}

func (p *testProvider) Retrieve(_ context.Context, _ string) (string, error) {
	p.callCount++
	return p.returnValue, p.returnError
}

// notApplicableProvider always returns ErrNotApplicable.
type notApplicableProvider struct {
	callCount int
}

func (p *notApplicableProvider) Retrieve(_ context.Context, _ string) (string, error) {
	p.callCount++
	return "", ErrNotApplicable
}

func Test_ChainProvider_Retrieve(t *testing.T) {
	ctx := context.Background()

	t.Run("returns plain value when no provider handles key", func(t *testing.T) {
		chain := NewChainProvider(&notApplicableProvider{})
		val, err := chain.Retrieve(ctx, "plainvalue")
		assert.NoError(t, err)
		assert.Equal(t, "plainvalue", val)
	})

	t.Run("returns plain value with empty chain", func(t *testing.T) {
		chain := NewChainProvider()
		val, err := chain.Retrieve(ctx, "plainvalue")
		assert.NoError(t, err)
		assert.Equal(t, "plainvalue", val)
	})

	t.Run("stops at first applicable provider", func(t *testing.T) {
		first := &testProvider{returnValue: "first"}
		second := &testProvider{returnValue: "second"}
		chain := NewChainProvider(first, second)
		val, err := chain.Retrieve(ctx, "key")
		assert.NoError(t, err)
		assert.Equal(t, "first", val)
		assert.Equal(t, 1, first.callCount)
		assert.Equal(t, 0, second.callCount)
	})

	t.Run("skips providers returning ErrNotApplicable", func(t *testing.T) {
		first := &notApplicableProvider{}
		second := &testProvider{returnValue: "result"}
		chain := NewChainProvider(first, second)
		val, err := chain.Retrieve(ctx, "key")
		assert.NoError(t, err)
		assert.Equal(t, "result", val)
		assert.Equal(t, 1, first.callCount)
		assert.Equal(t, 1, second.callCount)
	})

	t.Run("propagates provider error", func(t *testing.T) {
		expectedErr := errors.New("provider error")
		chain := NewChainProvider(&testProvider{returnError: expectedErr})
		_, err := chain.Retrieve(ctx, "key")
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("returns error for unknown ARN service", func(t *testing.T) {
		chain := NewChainProvider()
		_, err := chain.Retrieve(ctx, "arn:aws:invalid:eu-west-1:123456789012:thing/foo")
		assert.Error(t, err)
	})
}

func Test_AWSSecretsManagerProvider_Retrieve(t *testing.T) {
	ctx := context.Background()
	p := &AWSSecretsManagerProvider{}

	t.Run("not applicable for plain value", func(t *testing.T) {
		_, err := p.Retrieve(ctx, "plainvalue")
		assert.ErrorIs(t, err, ErrNotApplicable)
	})

	t.Run("not applicable for SSM ARN", func(t *testing.T) {
		_, err := p.Retrieve(ctx, "arn:aws:ssm:eu-west-1:123456789012:parameter/foo")
		assert.ErrorIs(t, err, ErrNotApplicable)
	})
}

func Test_AWSSSMProvider_Retrieve(t *testing.T) {
	ctx := context.Background()
	p := &AWSSSMProvider{}

	t.Run("not applicable for plain value", func(t *testing.T) {
		_, err := p.Retrieve(ctx, "plainvalue")
		assert.ErrorIs(t, err, ErrNotApplicable)
	})

	t.Run("not applicable for Secrets Manager ARN", func(t *testing.T) {
		_, err := p.Retrieve(ctx, "arn:aws:secretsmanager:eu-west-1:123456789012:secret:foo")
		assert.ErrorIs(t, err, ErrNotApplicable)
	})
}
