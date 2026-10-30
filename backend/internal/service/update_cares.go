package service

import (
	"context"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// 本地补丁版（cares，本地补丁）：版本号 X.Y.Z-klno.N-cares.M，发布在 caresRepo。
// 补丁版的升级要在新上游上重新打补丁构建，一键升级只会装上未打补丁的二开版本，
// 所以补丁版禁用一键升级 / 回滚，并对前端报告为源码构建（只显示发布页链接）。

const caresRepo = "heybody/sub2api"

var ErrCaresManualUpdate = infraerrors.BadRequest("CARES_MANUAL_UPDATE",
	"本地补丁版不支持一键升级或回滚，请按 SJ 升级流程（重新打补丁构建）更新")

func isCaresVersion(v string) bool {
	return strings.Contains(v, "-cares.")
}

// caresNumber 返回 -cares.M 的 M，无后缀为 0。
func caresNumber(v string) int {
	_, n, ok := strings.Cut(v, "-cares.")
	if !ok {
		return 0
	}
	parsed, err := strconv.Atoi(n)
	if err != nil {
		return 0
	}
	return parsed
}

// compareCaresVersions 先按上游与二开序号比较，再比补丁序号。
func compareCaresVersions(current, latest string) int {
	if c := compareVersions(current, latest); c != 0 {
		return c
	}
	a, b := caresNumber(current), caresNumber(latest)
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func (s *UpdateService) reportedBuildType() string {
	if isCaresVersion(s.currentVersion) {
		return "source"
	}
	return s.buildType
}

func (s *UpdateService) fetchCaresInfo(ctx context.Context) *UpstreamUpdateInfo {
	if !isCaresVersion(s.currentVersion) {
		return nil
	}
	release, err := s.githubClient.FetchLatestRelease(ctx, caresRepo)
	if err != nil {
		info := s.caresInfo(s.currentVersion, "", "")
		info.Warning = err.Error()
		return info
	}
	return s.caresInfo(strings.TrimPrefix(release.TagName, "v"), release.HTMLURL, release.PublishedAt)
}

func (s *UpdateService) caresInfo(latest, htmlURL, publishedAt string) *UpstreamUpdateInfo {
	return &UpstreamUpdateInfo{
		CurrentVersion: strings.TrimPrefix(s.currentVersion, "v"),
		LatestVersion:  latest,
		HasUpdate:      compareCaresVersions(s.currentVersion, latest) < 0,
		HTMLURL:        htmlURL,
		PublishedAt:    publishedAt,
	}
}
