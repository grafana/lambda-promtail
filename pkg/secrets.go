package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/smithy-go/ptr"
	vault "github.com/hashicorp/vault/api"
	auth "github.com/hashicorp/vault/api/auth/aws"
)

const (
	awsServiceSecretsManager = "secretsmanager"
	awsServiceSSM            = "ssm"
)

// ChainProvider tries each Provider in order, skipping those that return ErrNotApplicable.
// If no provider handles the key and it is an ARN, an error is returned.
// Otherwise the key is returned as a plain value.
type ChainProvider struct {
	providers []Provider
}

var _ Provider = &ChainProvider{}

func NewChainProvider(providers ...Provider) *ChainProvider {
	return &ChainProvider{providers: providers}
}

func (c *ChainProvider) Retrieve(ctx context.Context, key string) (string, error) {
	for _, p := range c.providers {
		val, err := p.Retrieve(ctx, key)
		if errors.Is(err, ErrNotApplicable) {
			continue
		}
		return val, err
	}
	if parsedArn, err := arn.Parse(key); err == nil {
		return "", fmt.Errorf("unsupported ARN service: %s", parsedArn.Service)
	}
	return key, nil
}

type VaultKVCredentials struct {
	role  string
	mount string
	path  string
}

// VaultProvider retrieves secrets from a Vault KVv2 engine using AWS auth.
// It is always applicable when present in a chain.
type VaultProvider struct {
	config    *VaultKVCredentials
	vaultData map[string]any
}

var _ Provider = &VaultProvider{}

func NewVaultProvider(config *VaultKVCredentials) *VaultProvider {
	return &VaultProvider{config: config}
}

func (p *VaultProvider) Retrieve(ctx context.Context, key string) (string, error) {
	if p.vaultData == nil {
		vaultConfig := vault.DefaultConfig()
		client, err := vault.NewClient(vaultConfig)
		if err != nil {
			return "", fmt.Errorf("unable to initialize Vault client: %w", err)
		}

		awsAuth, err := auth.NewAWSAuth(
			auth.WithRole(p.config.role),
		)
		if err != nil {
			return "", fmt.Errorf("unable to initialize AWS auth method: %w", err)
		}

		authInfo, err := client.Auth().Login(ctx, awsAuth)
		if err != nil {
			return "", fmt.Errorf("unable to login to AWS auth method: %w", err)
		}
		if authInfo == nil {
			return "", fmt.Errorf("no auth info was returned after login")
		}

		data, err := client.KVv2(p.config.mount).Get(ctx, p.config.path)
		if err != nil {
			return "", fmt.Errorf("unable to read secret: %w", err)
		}
		p.vaultData = data.Data
	}

	value, ok := p.vaultData[key].(string)
	if !ok {
		return "", fmt.Errorf("value type assertion failed: %T %#v", p.vaultData[key], p.vaultData[key])
	}

	return value, nil
}

// AWSSecretsManagerProvider retrieves secrets from AWS Secrets Manager.
// Returns ErrNotApplicable for keys that are not Secrets Manager ARNs.
type AWSSecretsManagerProvider struct{}

var _ Provider = &AWSSecretsManagerProvider{}

func (p *AWSSecretsManagerProvider) Retrieve(ctx context.Context, key string) (string, error) {
	parsedArn, err := arn.Parse(key)
	if err != nil || parsedArn.Service != awsServiceSecretsManager {
		return "", ErrNotApplicable
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("error loading AWS config: %w", err)
	}

	client := secretsmanager.NewFromConfig(cfg)
	out, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: &key,
	})
	if err != nil {
		return "", fmt.Errorf("error fetching secret %s: %w", key, err)
	}

	return *out.SecretString, nil
}

// AWSSSMProvider retrieves parameters from AWS SSM Parameter Store.
// Returns ErrNotApplicable for keys that are not SSM ARNs.
type AWSSSMProvider struct{}

var _ Provider = &AWSSSMProvider{}

func (p *AWSSSMProvider) Retrieve(ctx context.Context, key string) (string, error) {
	parsedArn, err := arn.Parse(key)
	if err != nil || parsedArn.Service != awsServiceSSM {
		return "", ErrNotApplicable
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("error loading AWS config: %w", err)
	}

	client := ssm.NewFromConfig(cfg)
	out, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           &key,
		WithDecryption: ptr.Bool(true),
	})
	if err != nil {
		return "", fmt.Errorf("error fetching SSM parameter %s: %w", key, err)
	}

	return *out.Parameter.Value, nil
}
