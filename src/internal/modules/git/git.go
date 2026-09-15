package git

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
)

// commitHashRe checks whether a string looks like a full or short commit SHA.
var commitHashRe = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)

// validVersionRe 拒绝包含路径穿越或空白字符的 version 字符串（修复 3）
var validVersionRe = regexp.MustCompile(`^[^\x00-\x1f /\\'"]+$`)

// CheckGitRepoExists verifies that a Git repository (and optionally a
// specific version) exists at the given URL.
//
// Rules:
//   - version == ""      → only check that the repo is reachable
//   - version looks like a commit hash (7-40 hex chars)
//     → fetch that object directly via git fetch
//   - otherwise          → check tags (refs/tags/<version> and
//     refs/tags/v<version>) and branches
//     (refs/heads/<version>)
func CheckGitRepoExists(url, version string) (bool, error) {
	if version != "" && !validVersionRe.MatchString(version) {
		return false, fmt.Errorf("invalid version string %q: contains illegal characters", version)
	}

	refs, err := listRemoteRefs(url)
	if err != nil {
		return false, err
	}

	if version == "" {
		return true, nil
	}

	if commitHashRe.MatchString(version) {
		return fetchCommitHash(url, version)
	}

	return matchRef(refs, version), nil
}

// listRemoteRefs connects to the remote and returns all advertised refs.
func listRemoteRefs(url string) ([]*plumbing.Reference, error) {
	remote := gogit.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: "origin",
		URLs: []string{url},
	})
	return remote.List(&gogit.ListOptions{})
}

// matchRef returns true when any of the candidate ref names exist in refs.
func matchRef(refs []*plumbing.Reference, version string) bool {
	candidates := map[string]struct{}{
		"refs/tags/" + version:  {},
		"refs/tags/v" + version: {},
		"refs/heads/" + version: {},
	}
	for _, ref := range refs {
		if _, ok := candidates[ref.Name().String()]; ok {
			return true
		}
	}
	return false
}

// fetchCommitHash attempts to fetch a specific commit hash from the remote.
func fetchCommitHash(url, hash string) (bool, error) {
	if len(hash) < 40 {
		return false, errors.New(
			"short commit hashes are ambiguous and cannot be reliably verified " +
				"against a remote; please provide the full 40-character SHA",
		)
	}

	storer := memory.NewStorage()
	repo, err := gogit.Init(storer, nil)
	if err != nil {
		return false, err
	}

	remote, err := repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{url},
	})
	if err != nil {
		return false, err
	}

	fetchErr := remote.Fetch(&gogit.FetchOptions{
		RefSpecs: []config.RefSpec{
			config.RefSpec(hash + ":FETCH_HEAD"),
		},
		Depth: 1,
	})

	switch {
	case fetchErr == nil:
		return true, nil
	case errors.Is(fetchErr, gogit.NoErrAlreadyUpToDate):
		return true, nil
	default:
		if isNotFoundError(fetchErr) {
			return false, nil
		}
		return false, fmt.Errorf("fetching commit %s: %w", hash, fetchErr)
	}
}

// isNotFoundError check whether the fetch error is "object not found" error
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, keyword := range []string{
		"object not found",
		"not our ref",
		"upload-pack",
		"no such",
		"not found",
	} {
		if strings.Contains(msg, keyword) {
			return true
		}
	}
	return false
}
