package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequest("POST", "https://example.com/loki/api/v1/push", nil)
	require.NoError(t, err)
	return req
}

func Test_basicAuthOption(t *testing.T) {
	req := newTestRequest(t)
	require.NoError(t, basicAuthOption{username: "user", password: "pass"}.Apply(context.Background(), req))

	user, pass, ok := req.BasicAuth()
	assert.True(t, ok)
	assert.Equal(t, "user", user)
	assert.Equal(t, "pass", pass)
}

func Test_bearerTokenOption(t *testing.T) {
	req := newTestRequest(t)
	require.NoError(t, bearerTokenOption{token: "tok"}.Apply(context.Background(), req))
	assert.Equal(t, "Bearer tok", req.Header.Get("Authorization"))
}

func Test_tenantIDOption(t *testing.T) {
	req := newTestRequest(t)
	require.NoError(t, tenantIDOption{tenantID: "tenant-1"}.Apply(context.Background(), req))
	assert.Equal(t, "tenant-1", req.Header.Get("X-Scope-OrgID"))
}

// fakeSTSClient is a stub implementation of stsWebIdentityTokenClient.
type fakeSTSClient struct {
	calls      int
	token      string
	expiration *time.Time
	err        error
	gotInput   *sts.GetWebIdentityTokenInput
}

func (f *fakeSTSClient) GetWebIdentityToken(_ context.Context, params *sts.GetWebIdentityTokenInput, _ ...func(*sts.Options)) (*sts.GetWebIdentityTokenOutput, error) {
	f.calls++
	f.gotInput = params
	if f.err != nil {
		return nil, f.err
	}
	tok := f.token
	return &sts.GetWebIdentityTokenOutput{WebIdentityToken: &tok, Expiration: f.expiration}, nil
}

func Test_stsWebIdentityOption_Apply(t *testing.T) {
	exp := time.Now().Add(time.Hour)
	fake := &fakeSTSClient{token: "jwt-123", expiration: &exp}
	opt := &stsWebIdentityOption{
		client:      fake,
		tenantID:    "tenant-1",
		wifAudience: "https://grafana-dev.com/v1/workload-identities/dev-eu-west-2:7161:alloy-ec2",
	}

	req := newTestRequest(t)
	require.NoError(t, opt.Apply(context.Background(), req))

	// Header has the form `Bearer <tenantID>:<JWT>`.
	assert.Equal(t, "Bearer tenant-1:jwt-123", req.Header.Get("Authorization"))
	// The audience sent to STS is wifAudience, not the tenant ID.
	require.NotNil(t, fake.gotInput)
	assert.Equal(t, []string{opt.wifAudience}, fake.gotInput.Audience)
	require.NotNil(t, fake.gotInput.SigningAlgorithm)
	assert.Equal(t, stsSigningAlg, *fake.gotInput.SigningAlgorithm)
}

func Test_stsWebIdentityOption_CachesToken(t *testing.T) {
	exp := time.Now().Add(time.Hour)
	fake := &fakeSTSClient{token: "jwt-123", expiration: &exp}
	opt := &stsWebIdentityOption{client: fake, tenantID: "t", wifAudience: "aud"}

	require.NoError(t, opt.Apply(context.Background(), newTestRequest(t)))
	require.NoError(t, opt.Apply(context.Background(), newTestRequest(t)))

	assert.Equal(t, 1, fake.calls, "token should be fetched once and then cached")
}

func Test_stsWebIdentityOption_RefreshesNearExpiry(t *testing.T) {
	// Token already within the refresh margin, so each Apply must re-fetch.
	exp := time.Now().Add(stsTokenRefreshMargin / 2)
	fake := &fakeSTSClient{token: "jwt-123", expiration: &exp}
	opt := &stsWebIdentityOption{client: fake, tenantID: "t", wifAudience: "aud"}

	require.NoError(t, opt.Apply(context.Background(), newTestRequest(t)))
	require.NoError(t, opt.Apply(context.Background(), newTestRequest(t)))

	assert.Equal(t, 2, fake.calls, "near-expiry token should be refreshed on each use")
}
