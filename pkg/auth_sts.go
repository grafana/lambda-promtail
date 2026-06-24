package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const (
	// stsSigningAlg is the signing algorithm requested from STS for the web identity token.
	stsSigningAlg = "ES384"

	// stsTokenRefreshMargin is how long before its expiry a cached token is refreshed.
	stsTokenRefreshMargin = 1 * time.Minute
)

// stsWebIdentityTokenClient is the subset of the STS client used to fetch web identity tokens.
// It is an interface so the option can be unit tested without calling AWS.
type stsWebIdentityTokenClient interface {
	GetWebIdentityToken(ctx context.Context, params *sts.GetWebIdentityTokenInput, optFns ...func(*sts.Options)) (*sts.GetWebIdentityTokenOutput, error)
}

// stsWebIdentityOption fetches a web identity JWT from AWS STS and sets it as a bearer token.
// The audience of the token is wifAudience (e.g.
// https://grafana-dev.com/v1/workload-identities/dev-eu-west-2:7161:alloy-ec2), and the header
// is set to:
//
//	Authorization: Bearer <tenantID>:<JWT>
type stsWebIdentityOption struct {
	client      stsWebIdentityTokenClient
	tenantID    string
	wifAudience string

	mu          sync.Mutex
	cachedToken string
	expiresAt   time.Time
}

// newSTSWebIdentityOption builds an stsWebIdentityOption. If roleARN is non-empty the option
// assumes that role before requesting the token, mirroring the behaviour of Alloy's gcomawsauth.
func newSTSWebIdentityOption(ctx context.Context, tenantID, wifAudience, roleARN string) (*stsWebIdentityOption, error) {
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	if roleARN != "" {
		creds := stscreds.NewAssumeRoleProvider(sts.NewFromConfig(awsCfg), roleARN)
		awsCfg.Credentials = aws.NewCredentialsCache(creds)
	}

	return &stsWebIdentityOption{
		client:      sts.NewFromConfig(awsCfg),
		tenantID:    tenantID,
		wifAudience: wifAudience,
	}, nil
}

func (o *stsWebIdentityOption) Apply(ctx context.Context, req *http.Request) error {
	token, err := o.token(ctx)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s:%s", o.tenantID, token))
	return nil
}

// token returns a cached web identity token, fetching a fresh one from STS when none is cached
// or the cached one is close to expiry.
func (o *stsWebIdentityOption) token(ctx context.Context) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.cachedToken != "" && time.Now().Before(o.expiresAt.Add(-stsTokenRefreshMargin)) {
		return o.cachedToken, nil
	}

	alg := stsSigningAlg
	output, err := o.client.GetWebIdentityToken(ctx, &sts.GetWebIdentityTokenInput{
		Audience:         []string{o.wifAudience},
		SigningAlgorithm: &alg,
	})
	if err != nil {
		return "", fmt.Errorf("failed to get JWT from AWS STS: %w", err)
	}
	if output.WebIdentityToken == nil {
		return "", fmt.Errorf("AWS STS returned an empty web identity token")
	}

	o.cachedToken = *output.WebIdentityToken
	if output.Expiration != nil {
		o.expiresAt = *output.Expiration
	} else {
		o.expiresAt = time.Time{}
	}

	return o.cachedToken, nil
}
