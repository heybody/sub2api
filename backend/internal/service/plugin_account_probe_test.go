//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type probeRepoStub struct {
	AccountRepository
	byID map[int64]*Account
}

func (r *probeRepoStub) GetByID(_ context.Context, id int64) (*Account, error) {
	return r.byID[id], nil
}

func probeTestService(accounts ...*Account) *OpenAIGatewayService {
	repo := &probeRepoStub{byID: map[int64]*Account{}}
	for _, a := range accounts {
		repo.byID[a.ID] = a
	}
	return &OpenAIGatewayService{cfg: cprTestConfig(), accountRepo: repo, httpUpstream: &httpUpstreamRecorder{}}
}

func probeAPIKeyAccount() *Account {
	return &Account{ID: 4202, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive,
		Credentials: map[string]any{"api_key": "sk-plain", "base_url": "https://relay.example.com"}, Extra: map[string]any{}}
}

func oauthOnlyScope() PluginAccountScope {
	return newPluginAccountScope(pluginAccountScopeEntry{Platform: PlatformOpenAI, AccountType: AccountTypeOAuth})
}

func TestAccountProbeGrantRequiresConfigTrustAndExistingScope(t *testing.T) {
	cfg := &config.Config{}
	cfg.Plugins.AccountProbePlugins = []string{"local.identity-test"}
	m := &PluginManager{cfg: cfg}
	trusted := &PluginInstallation{PluginKey: "local.identity-test", SignatureStatus: PluginSignatureTrusted}

	granted := m.withAccountProbeGrant(trusted, oauthOnlyScope())
	require.True(t, granted.Contains(PlatformOpenAI, AccountTypeCPR))
	require.True(t, granted.Contains(PlatformOpenAI, AccountTypeAPIKey))
	require.True(t, granted.Contains(PlatformOpenAI, AccountTypeSetupToken))
	require.True(t, granted.Contains(PlatformOpenAI, AccountTypeOAuth))
	require.False(t, granted.Contains(PlatformAnthropic, AccountTypeAPIKey), "只放宽 openai 平台")

	other := &PluginInstallation{PluginKey: "local.other", SignatureStatus: PluginSignatureTrusted}
	require.False(t, m.withAccountProbeGrant(other, oauthOnlyScope()).Contains(PlatformOpenAI, AccountTypeCPR), "不在名单")
	unsigned := &PluginInstallation{PluginKey: "local.identity-test", SignatureStatus: PluginSignatureUnsigned}
	require.False(t, m.withAccountProbeGrant(unsigned, oauthOnlyScope()).Contains(PlatformOpenAI, AccountTypeCPR), "未签名")
	require.True(t, m.withAccountProbeGrant(trusted, newPluginAccountScope()).Empty(), "没有账号能力的插件不授予")
	require.False(t, (&PluginManager{cfg: &config.Config{}}).withAccountProbeGrant(trusted, oauthOnlyScope()).Contains(PlatformOpenAI, AccountTypeCPR), "默认不授予")
}

// 探测目标必须与真实转发的出站 URL 一致，避免两套逻辑漂移。
func TestPluginProbeTargetMatchesForwarding(t *testing.T) {
	body := convTestBody(t)
	for _, account := range []*Account{newCPRTestAccount(), probeAPIKeyAccount()} {
		svc := probeTestService(account)
		token, _, err := svc.GetAccessToken(context.Background(), account)
		require.NoError(t, err)
		req, err := svc.buildUpstreamRequest(context.Background(), newConvTestContext(t, body), account, body, token, false, "", false)
		require.NoError(t, err)
		endpoint, protocol, err := svc.pluginProbeTarget(account)
		require.NoError(t, err)
		require.Equal(t, PluginWireProtocolResponses, protocol)
		require.Equal(t, req.URL.String(), endpoint, account.Type)
	}
}

func TestResolvePluginOutboundIdentityProbeTypes(t *testing.T) {
	cpr := newCPRTestAccount()
	apikey := probeAPIKeyAccount()
	svc := probeTestService(cpr, apikey)
	cfg := &config.Config{}
	cfg.Plugins.AccountProbePlugins = []string{"local.identity-test"}
	scope := (&PluginManager{cfg: cfg}).withAccountProbeGrant(
		&PluginInstallation{PluginKey: "local.identity-test", SignatureStatus: PluginSignatureTrusted}, oauthOnlyScope())

	got, err := svc.ResolvePluginOutboundIdentity(context.Background(), scope, cpr.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, cprTestClientKey, got.Token)
	require.Equal(t, "http://127.0.0.1:18081/v1/responses", got.EndpointURL)
	require.Equal(t, PluginWireProtocolResponses, got.WireProtocol)
	require.Empty(t, got.Headers, "非 Codex 账号不带 ChatGPT 身份头")

	got, err = svc.ResolvePluginOutboundIdentity(context.Background(), scope, apikey.ID)
	require.NoError(t, err)
	require.Equal(t, "sk-plain", got.Token)
	require.Equal(t, "https://relay.example.com/v1/responses", got.EndpointURL)

	// 未授权时与上游一致：OAuth 之外一律不给。
	got, err = svc.ResolvePluginOutboundIdentity(context.Background(), oauthOnlyScope(), cpr.ID)
	require.NoError(t, err)
	require.Nil(t, got)

	// CPR 缺 base_url 时报错，绝不回落官方地址。
	broken := newCPRTestAccount()
	broken.ID = 4203
	delete(broken.Credentials, "base_url")
	_, err = probeTestService(broken).ResolvePluginOutboundIdentity(context.Background(), scope, broken.ID)
	require.Error(t, err)
}
