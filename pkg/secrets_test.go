package main

import (
	"context"
	"errors"
)

var (
	_             secretFetcher = &testSecretsClient{}
	errInvalidArn               = errors.New("invalid arn")
)

type testSecretsClient struct {
	CallsFetchFromAWSSecretsManager    int
	CallsFetchFromAWSSSMParameterStore int

	ExpectedArn string
	ReturnValue string
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
