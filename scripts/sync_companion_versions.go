package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

func main() {
	checkOnly := len(os.Args) > 1 && os.Args[1] == "--check"
	repoRoot, err := os.Getwd()
	if err != nil {
		fatal(err)
	}

	rootVersion, companionVersion, err := repoLocalDevVersions(repoRoot)
	if err != nil {
		fatal(err)
	}

	changed := false
	for _, update := range updates(rootVersion, companionVersion) {
		next, err := rewriteFile(filepath.Join(repoRoot, update.Path), update.Requirements)
		if err != nil {
			fatal(err)
		}
		current, err := os.ReadFile(filepath.Join(repoRoot, update.Path))
		if err != nil {
			fatal(err)
		}
		if bytes.Equal(current, next) {
			continue
		}
		changed = true
		if checkOnly {
			fmt.Fprintf(os.Stderr, "%s is out of sync\n", update.Path)
			continue
		}
		if err := os.WriteFile(filepath.Join(repoRoot, update.Path), next, 0o644); err != nil {
			fatal(err)
		}
		fmt.Printf("updated %s\n", update.Path)
	}

	if checkOnly && changed {
		os.Exit(1)
	}
}

type fileUpdate struct {
	Path         string
	Requirements map[string]string
}

func updates(rootVersion, companionVersion string) []fileUpdate {
	return []fileUpdate{
		{
			Path: "go.mod",
			Requirements: map[string]string{
				"github.com/yiiilin/harness-core/modules":              companionVersion,
				"github.com/yiiilin/harness-core/pkg/harness/builtins": companionVersion,
			},
		},
		{
			Path: "modules/go.mod",
			Requirements: map[string]string{
				"github.com/yiiilin/harness-core": rootVersion,
			},
		},
		{
			Path: "pkg/harness/builtins/go.mod",
			Requirements: map[string]string{
				"github.com/yiiilin/harness-core":         rootVersion,
				"github.com/yiiilin/harness-core/modules": companionVersion,
			},
		},
		{
			Path: "adapters/go.mod",
			Requirements: map[string]string{
				"github.com/yiiilin/harness-core":                      rootVersion,
				"github.com/yiiilin/harness-core/modules":              companionVersion,
				"github.com/yiiilin/harness-core/pkg/harness/builtins": companionVersion,
			},
		},
		{
			Path: "cmd/harness-core/go.mod",
			Requirements: map[string]string{
				"github.com/yiiilin/harness-core":                      rootVersion,
				"github.com/yiiilin/harness-core/adapters":             companionVersion,
				"github.com/yiiilin/harness-core/modules":              companionVersion,
				"github.com/yiiilin/harness-core/pkg/harness/builtins": companionVersion,
			},
		},
	}
}

func repoLocalDevVersions(repoRoot string) (string, string, error) {
	headShort, err := gitOutput(repoRoot, "rev-parse", "--short=12", "HEAD")
	if err != nil {
		return "", "", err
	}
	headTime, err := gitOutputWithEnv(repoRoot, []string{"TZ=UTC"}, "show", "-s", "--format=%cd", "--date=format-local:%Y%m%d%H%M%S", "HEAD")
	if err != nil {
		return "", "", err
	}
	tag, err := latestRootTag(repoRoot)
	if err != nil {
		return "", "", err
	}
	major, minor, patch, err := parseReleaseTag(tag)
	if err != nil {
		return "", "", err
	}
	rootVersion := fmt.Sprintf("v%d.%d.%d-0.%s-%s", major, minor, patch+1, headTime, headShort)
	companionVersion := fmt.Sprintf("v0.0.0-%s-%s", headTime, headShort)
	return rootVersion, companionVersion, nil
}

func latestRootTag(repoRoot string) (string, error) {
	raw, err := gitOutput(repoRoot, "tag", "--list", "v*")
	if err != nil {
		return "", err
	}
	best := ""
	bestMajor, bestMinor, bestPatch := -1, -1, -1
	for _, tag := range strings.Fields(raw) {
		major, minor, patch, err := parseReleaseTag(tag)
		if err != nil {
			continue
		}
		if major > bestMajor || (major == bestMajor && minor > bestMinor) || (major == bestMajor && minor == bestMinor && patch > bestPatch) {
			best = tag
			bestMajor, bestMinor, bestPatch = major, minor, patch
		}
	}
	if best == "" {
		return "", errors.New("no root semver tags found")
	}
	return best, nil
}

var releaseTagRE = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)\.([0-9]+)$`)

func parseReleaseTag(tag string) (int, int, int, error) {
	match := releaseTagRE.FindStringSubmatch(tag)
	if len(match) != 4 {
		return 0, 0, 0, fmt.Errorf("invalid release tag %q", tag)
	}
	major, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, 0, 0, err
	}
	minor, err := strconv.Atoi(match[2])
	if err != nil {
		return 0, 0, 0, err
	}
	patch, err := strconv.Atoi(match[3])
	if err != nil {
		return 0, 0, 0, err
	}
	return major, minor, patch, nil
}

func rewriteFile(path string, requirements map[string]string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := string(data)
	for modulePath, version := range requirements {
		re := regexp.MustCompile(regexp.QuoteMeta(modulePath) + `\s+v[^\s]+`)
		out = re.ReplaceAllString(out, modulePath+" "+version)
	}
	return []byte(out), nil
}

func gitOutput(repoRoot string, args ...string) (string, error) {
	return gitOutputWithEnv(repoRoot, nil, args...)
}

func gitOutputWithEnv(repoRoot string, extraEnv []string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), extraEnv...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
