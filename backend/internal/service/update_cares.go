package service

import (
	"context"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// 本地补丁版（cares）：版本号 X.Y.Z-klno.N-cares.Z[.R]（补丁号跟随上游 Z，R 为同版修订），发布在 caresRepo。
// 补丁版的升级要在新上游上重新打补丁构建，一键升级只会装上未打补丁的二开版本，
// 所以补丁版禁用一键升级 / 回滚，并对前端报告为源码构建（只显示发布页链接）。

const caresRepo = "heybody/sub2api"

var ErrCaresManualUpdate = infraerrors.BadRequest("CARES_MANUAL_UPDATE",
	"本地补丁版不支持一键升级或回滚，请按 SJ 升级流程（重新打补丁构建）更新")

func isCaresVersion(v string) bool {
	return strings.Contains(v, "-cares.")
}

// caresParts 返回 -cares.M[.R] 的 (M, R)：M 跟随上游补丁号（0.2.19 → cares.19），
// R 是同一上游版本上的修订号；无后缀为 (0, 0)。
func caresParts(v string) [2]int {
	var out [2]int
	_, tail, ok := strings.Cut(v, "-cares.")
	if !ok {
		return out
	}
	for i, part := range strings.SplitN(tail, ".", 2) {
		if n, err := strconv.Atoi(part); err == nil {
			out[i] = n
		}
	}
	return out
}

// compareCaresVersions 先按上游与二开序号比较，再比补丁号与修订号。
func compareCaresVersions(current, latest string) int {
	if c := compareVersions(current, latest); c != 0 {
		return c
	}
	a, b := caresParts(current), caresParts(latest)
	for i := range a {
		switch {
		case a[i] < b[i]:
			return -1
		case a[i] > b[i]:
			return 1
		}
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
