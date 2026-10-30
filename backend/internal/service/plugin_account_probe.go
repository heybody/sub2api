package service

import (
	"context"
	"net/http"
	"strings"
)

// 账号探测授权（本地补丁）：让只测试不转发的插件（如账号推理测试）也能读取 OpenAI
// 的 setup-token / apikey / cpr 账号。插件清单保持标准格式（只声明 OAuth 能力），
// 由管理员在 config.yaml 的 plugins.account_probe_plugins 按插件 ID 显式授予；
// 未配置时行为与上游完全一致。授予只扩大账号目录与身份解析范围，不改变 Forward
// 绑定——启用插件也不会接管这些账号的日常流量。

const (
	PluginWireProtocolCodex           = "codex"
	PluginWireProtocolResponses       = "responses"
	PluginWireProtocolChatCompletions = "chat_completions"
)

var pluginAccountProbeGrants = []pluginAccountScopeEntry{
	{Platform: PlatformOpenAI, AccountType: AccountTypeSetupToken},
	{Platform: PlatformOpenAI, AccountType: AccountTypeAPIKey},
	{Platform: PlatformOpenAI, AccountType: AccountTypeCPR},
}

// withAccountProbeGrant 只给「已具备账号目录能力、签名可信、且在配置名单内」的插件追加范围。
func (m *PluginManager) withAccountProbeGrant(installation *PluginInstallation, scope PluginAccountScope) PluginAccountScope {
	if m == nil || m.cfg == nil || installation == nil || scope.Empty() || installation.SignatureStatus != PluginSignatureTrusted {
		return scope
	}
	for _, id := range m.cfg.Plugins.AccountProbePlugins {
		if strings.TrimSpace(id) == installation.PluginKey {
			return newPluginAccountScope(append(append([]pluginAccountScopeEntry{}, scope.entries...), pluginAccountProbeGrants...)...)
		}
	}
	return scope
}

// resolvePluginProbeIdentity 为 apikey / cpr 账号给出与真实转发一致的目标地址和 Bearer 凭据。
// 调用方已校验范围；这里只处理 openai 平台，其余类型返回 (nil, nil)。
func (s *OpenAIGatewayService) resolvePluginProbeIdentity(ctx context.Context, account *Account) (*PluginOutboundIdentity, error) {
	if account.Platform != PlatformOpenAI {
		return nil, nil
	}
	endpoint, protocol, err := s.pluginProbeTarget(account)
	if err != nil {
		return nil, err
	}
	if protocol == "" {
		return nil, nil
	}
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(token) == "" {
		return nil, nil
	}
	return &PluginOutboundIdentity{
		AccountID:    account.ID,
		Platform:     account.Platform,
		AccountType:  account.Type,
		ProxyURL:     resolveAccountProxyURL(account),
		Token:        token,
		Headers:      http.Header{},
		EndpointURL:  endpoint,
		WireProtocol: protocol,
	}, nil
}

// pluginProbeTarget 与 buildUpstreamRequest 的 apikey / cpr 分支保持一致（测试会比对两者）。
func (s *OpenAIGatewayService) pluginProbeTarget(account *Account) (string, string, error) {
	switch account.Type {
	case AccountTypeAPIKey:
		if shouldForwardOpenAIResponsesViaRawChatCompletions(account) {
			return "", PluginWireProtocolChatCompletions, nil
		}
		baseURL := account.GetOpenAIBaseURL()
		if account.UsesNativeCNResponses() && account.IsAdaptiveAPIProtocol() {
			baseURL = account.GetCNProtocolBaseURL(APIProtocolResponses)
		}
		if baseURL == "" {
			return openaiPlatformAPIURL, PluginWireProtocolResponses, nil
		}
		validated, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return "", "", err
		}
		return buildOpenAIResponsesURLForPlatform(account.Platform, validated), PluginWireProtocolResponses, nil
	case AccountTypeCPR:
		validated, err := s.validatedCPRBaseURL(account)
		if err != nil {
			return "", "", err
		}
		return buildOpenAIResponsesURL(validated), PluginWireProtocolResponses, nil
	}
	return "", "", nil
}
