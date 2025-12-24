package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/smithy-go/ptr"
	vault "github.com/hashicorp/vault/api"
	auth "github.com/hashicorp/vault/api/auth/aws"
)

type VaultKVCredentials struct {
	role  string
	mount string
	path  string
}

var _ secretFetcher = &secretClients{}

type secretClients struct {
	vaultConfig *VaultKVCredentials
	vaultData   map[string]any
}

func (c *secretClients) FetchFromAWSSecretsManager(ctx context.Context, secretArn string) (string, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("error loading aws config: %w", err)
	}

	client := secretsmanager.NewFromConfig(cfg)
	out, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: &secretArn,
	})
	if err != nil {
		return "", fmt.Errorf("error fetching secret %s: %w", secretArn, err)
	}

	return *out.SecretString, nil
}

func (c *secretClients) FetchFromAWSSSMParameterStore(ctx context.Context, parameterArn string) (string, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("error loading aws config: %w", err)
	}

	client := ssm.NewFromConfig(cfg)
	out, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           &parameterArn,
		WithDecryption: ptr.Bool(true),
	})
	if err != nil {
		return "", fmt.Errorf("error fetching SSM parameter %s: %w", parameterArn, err)
	}

	return *out.Parameter.Value, nil
}

func (c *secretClients) FetchFromVault(ctx context.Context, key string) (string, error) {
	if c.vaultData == nil {
		if c.vaultConfig == nil {
			return "", errors.New("vault configuration required")
		}
		config := vault.DefaultConfig()

		client, err := vault.NewClient(config)
		if err != nil {
			return "", fmt.Errorf("unable to initialize Vault client: %w", err)
		}

		awsAuth, err := auth.NewAWSAuth(
			auth.WithRole(c.vaultConfig.role), // if not provided, Vault will fall back on looking for a role with the IAM role name if you're using the iam auth type, or the EC2 instance's AMI id if using the ec2 auth type
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

		data, err := client.KVv2(c.vaultConfig.mount).Get(ctx, c.vaultConfig.path)
		if err != nil {
			return "", fmt.Errorf("unable to read secret: %w", err)
		}
		c.vaultData = data.Data
	}

	value, ok := c.vaultData[key].(string)
	if !ok {
		return "", fmt.Errorf("value type assertion failed: %T %#v", c.vaultData[key], c.vaultData[key])
	}

	return value, nil
}

func (c *secretClients) HasVaultConfig() bool {
	return c.vaultConfig != nil
}

func (c *secretClients) SetVaultConfig(config *VaultKVCredentials) {
	c.vaultConfig = config
}
