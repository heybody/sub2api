//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdateServiceCaresBuildThreeLayers(t *testing.T) {
	gh := &updateServiceRepoStub{latestByRepo: map[string]*GitHubRelease{
		"KlN-4096/sub2api": {TagName: "v0.2.19-klno.1"},
		"Wei-Shaw/sub2api": {TagName: "v0.2.20"},
		caresRepo:          {TagName: "v0.2.19-klno.1-cares.19.1", HTMLURL: "https://github.com/heybody/sub2api/releases/tag/v0.2.19-klno.1-cares.19.1"},
	}}
	svc := NewUpdateService(&updateServiceCacheStub{}, gh, "0.2.19-klno.1-cares.19", "release")

	info, err := svc.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	require.False(t, info.HasUpdate, "klno 层只比 klno 序号，补丁后缀不能被当成 klno.0")
	require.Equal(t, "source", info.BuildType, "补丁版对前端报告为源码构建，不出一键升级")
	require.True(t, info.Upstream.HasUpdate)
	require.Equal(t, "0.2.19", info.Upstream.CurrentVersion)
	require.NotNil(t, info.Cares)
	require.True(t, info.Cares.HasUpdate)
	require.Equal(t, "0.2.19-klno.1-cares.19.1", info.Cares.LatestVersion)

	cached, err := svc.CheckUpdate(context.Background(), false)
	require.NoError(t, err)
	require.True(t, cached.Cares.HasUpdate)

	require.ErrorIs(t, svc.PerformUpdate(context.Background()), ErrCaresManualUpdate)
	require.ErrorIs(t, svc.RollbackToVersion(context.Background(), "0.2.19-klno.1"), ErrCaresManualUpdate)
	_, err = svc.ListRollbackVersions(context.Background())
	require.ErrorIs(t, err, ErrCaresManualUpdate)
}

func TestUpdateServiceKlnoUpdateVisibleFromCares(t *testing.T) {
	gh := &updateServiceRepoStub{latestByRepo: map[string]*GitHubRelease{
		"KlN-4096/sub2api": {TagName: "v0.2.19-klno.2"},
		"Wei-Shaw/sub2api": {TagName: "v0.2.19"},
		caresRepo:          {TagName: "v0.2.19-klno.1-cares.19"},
	}}
	svc := NewUpdateService(&updateServiceCacheStub{}, gh, "0.2.19-klno.1-cares.19", "release")
	info, err := svc.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	require.True(t, info.HasUpdate)
	require.False(t, info.Cares.HasUpdate)
	require.False(t, info.Upstream.HasUpdate)
}

func TestUpdateServiceNonCaresUnchanged(t *testing.T) {
	gh := &updateServiceRepoStub{latestByRepo: map[string]*GitHubRelease{
		"KlN-4096/sub2api": {TagName: "v0.2.19-klno.1"},
		"Wei-Shaw/sub2api": {TagName: "v0.2.19"},
	}}
	svc := NewUpdateService(&updateServiceCacheStub{}, gh, "0.2.19-klno.1", "release")
	info, err := svc.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	require.Nil(t, info.Cares)
	require.Equal(t, "release", info.BuildType)
	require.NotContains(t, gh.calls, "latest:"+caresRepo)
}

func TestCompareCaresVersions(t *testing.T) {
	require.Equal(t, -1, compareCaresVersions("0.2.19-klno.1-cares.2", "0.2.19-klno.1-cares.19"))
	require.Equal(t, -1, compareCaresVersions("0.2.19-klno.1-cares.19", "0.2.19-klno.1-cares.19.1"))
	require.Equal(t, 1, compareCaresVersions("0.2.19-klno.1-cares.19.2", "0.2.19-klno.1-cares.19.1"))
	require.Equal(t, -1, compareCaresVersions("0.2.19-klno.1-cares.19.3", "0.2.20-klno.1-cares.20"))
	require.Equal(t, 0, compareCaresVersions("0.2.19-klno.1-cares.19", "0.2.19-klno.1-cares.19"))
}
