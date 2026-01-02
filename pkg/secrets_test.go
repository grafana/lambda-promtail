package main

import (
	"context"
	"errors"
)

var (
	_                secretFetcher = &testSecretsClient{}
	errInvalidArn                  = errors.New("invalid arn")
	errNoVaultConfig               = errors.New("no vault config")
)

type testSecretsClient struct {
	CallsFetchFromAWSSecretsManager    int
	CallsFetchFromAWSSSMParameterStore int
	CallsFetchFromVault                int

	ExpectedArn      string
	ReturnValue      string
	VaultConfigured  bool
	VaultCredentials *VaultKVCredentials
}

func (c *testSecretsClient) FetchFromAWSSecretsManager(_ context.Context, secretArn string) (string, error) {
	c.CallsFetchFromAWSSecretsManager++

	if c.ExpectedArn != "" && secretArn != c.ExpectedArn {
		return "", errInvalidArn
	}

	return c.ReturnValue, nil
}

func (c *testSecretsClient) FetchFromAWSSSMParameterStore(_ context.Context, parameterArn string) (string, error) {
	c.CallsFetchFromAWSSSMParameterStore++

	if c.ExpectedArn != "" && parameterArn != c.ExpectedArn {
		return "", errInvalidArn
	}

	return c.ReturnValue, nil
}

func (c *testSecretsClient) FetchFromVault(_ context.Context, key string) (string, error) {
	c.CallsFetchFromVault++

	if c.VaultCredentials == nil {
		return "", errNoVaultConfig
	}
	if c.ExpectedArn != "" && key != c.ExpectedArn {
		return "", errInvalidArn
	}

	return c.ReturnValue, nil
}

func (c *testSecretsClient) HasVaultConfig() bool {
	return c.VaultConfigured
}

func (c *testSecretsClient) SetVaultConfig(config *VaultKVCredentials) {
	c.VaultCredentials = config
}
