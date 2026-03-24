package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func main() {
	checkOnly := len(os.Args) > 1 && os.Args[1] == "--check"
	repoRoot, err := os.Getwd()
	if err != nil {
		fatal(err)
	}
	if checkOnly {
		if err := checkCommittedCompatibilityMatrix(repoRoot); err != nil {
			fatal(err)
		}
		return
	}

	rootVersion, companionVersion, err := repoLocalDevVersions(repoRoot)
	if err != nil {
		fatal(err)
	}

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
		if err := os.WriteFile(filepath.Join(repoRoot, update.Path), next, 0o644); err != nil {
			fatal(err)
		}
		fmt.Printf("updated %s\n", update.Path)
	}
}

func checkCommittedCompatibilityMatrix(repoRoot string) error {
	matrix, err := committedCompatibilityMatrix(repoRoot)
	if err != nil {
		return err
	}

	var mismatches []string
	for _, update := range updates(matrix.RootVersion, matrix.CompanionVersion) {
		requirements, err := goModRequireVersions(filepath.Join(repoRoot, update.Path))
		if err != nil {
			return err
		}
		for modulePath, want := range update.Requirements {
			got := requirements[modulePath]
			if got == want {
				continue
			}
			mismatches = append(mismatches, fmt.Sprintf("%s requires %s at %q; want %q", update.Path, modulePath, got, want))
		}
	}

	if len(mismatches) != 0 {
		return errors.New(strings.Join(mismatches, "\n"))
	}
	return nil
}

type compatibilityMatrix struct {
	RootVersion      string
	CompanionVersion string
}

func committedCompatibilityMatrix(repoRoot string) (compatibilityMatrix, error) {
	requirements := map[string]map[string]string{}
	for _, relPath := range []string{
		"go.mod",
		"modules/go.mod",
		"pkg/harness/builtins/go.mod",
		"adapters/go.mod",
		"cmd/harness-core/go.mod",
	} {
		parsed, err := goModRequireVersions(filepath.Join(repoRoot, relPath))
		if err != nil {
			return compatibilityMatrix{}, err
		}
		requirements[relPath] = parsed
	}

	rootVersion, err := uniqueRequiredVersion([]requiredVersion{
		{Path: "modules/go.mod", ModulePath: "github.com/yiiilin/harness-core", Version: requirements["modules/go.mod"]["github.com/yiiilin/harness-core"]},
		{Path: "pkg/harness/builtins/go.mod", ModulePath: "github.com/yiiilin/harness-core", Version: requirements["pkg/harness/builtins/go.mod"]["github.com/yiiilin/harness-core"]},
		{Path: "adapters/go.mod", ModulePath: "github.com/yiiilin/harness-core", Version: requirements["adapters/go.mod"]["github.com/yiiilin/harness-core"]},
		{Path: "cmd/harness-core/go.mod", ModulePath: "github.com/yiiilin/harness-core", Version: requirements["cmd/harness-core/go.mod"]["github.com/yiiilin/harness-core"]},
	})
	if err != nil {
		return compatibilityMatrix{}, err
	}

	companionVersion, err := uniqueRequiredVersion([]requiredVersion{
		{Path: "go.mod", ModulePath: "github.com/yiiilin/harness-core/adapters", Version: requirements["go.mod"]["github.com/yiiilin/harness-core/adapters"]},
		{Path: "go.mod", ModulePath: "github.com/yiiilin/harness-core/modules", Version: requirements["go.mod"]["github.com/yiiilin/harness-core/modules"]},
		{Path: "go.mod", ModulePath: "github.com/yiiilin/harness-core/pkg/harness/builtins", Version: requirements["go.mod"]["github.com/yiiilin/harness-core/pkg/harness/builtins"]},
		{Path: "pkg/harness/builtins/go.mod", ModulePath: "github.com/yiiilin/harness-core/modules", Version: requirements["pkg/harness/builtins/go.mod"]["github.com/yiiilin/harness-core/modules"]},
		{Path: "adapters/go.mod", ModulePath: "github.com/yiiilin/harness-core/modules", Version: requirements["adapters/go.mod"]["github.com/yiiilin/harness-core/modules"]},
		{Path: "adapters/go.mod", ModulePath: "github.com/yiiilin/harness-core/pkg/harness/builtins", Version: requirements["adapters/go.mod"]["github.com/yiiilin/harness-core/pkg/harness/builtins"]},
		{Path: "cmd/harness-core/go.mod", ModulePath: "github.com/yiiilin/harness-core/adapters", Version: requirements["cmd/harness-core/go.mod"]["github.com/yiiilin/harness-core/adapters"]},
		{Path: "cmd/harness-core/go.mod", ModulePath: "github.com/yiiilin/harness-core/modules", Version: requirements["cmd/harness-core/go.mod"]["github.com/yiiilin/harness-core/modules"]},
		{Path: "cmd/harness-core/go.mod", ModulePath: "github.com/yiiilin/harness-core/pkg/harness/builtins", Version: requirements["cmd/harness-core/go.mod"]["github.com/yiiilin/harness-core/pkg/harness/builtins"]},
	})
	if err != nil {
		return compatibilityMatrix{}, err
	}

	return compatibilityMatrix{
		RootVersion:      rootVersion,
		CompanionVersion: companionVersion,
	}, nil
}

type requiredVersion struct {
	Path       string
	ModulePath string
	Version    string
}

func uniqueRequiredVersion(requirements []requiredVersion) (string, error) {
	want := ""
	for _, requirement := range requirements {
		if requirement.Version == "" {
			return "", fmt.Errorf("%s is missing requirement for %s", requirement.Path, requirement.ModulePath)
		}
		if want == "" {
			want = requirement.Version
			continue
		}
		if requirement.Version != want {
			return "", fmt.Errorf("repo-local manifest matrix drift: %s requires %s at %q; expected %q", requirement.Path, requirement.ModulePath, requirement.Version, want)
		}
	}
	if want == "" {
		return "", errors.New("empty requirement set")
	}
	return want, nil
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
				"github.com/yiiilin/harness-core/adapters":             companionVersion,
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

func goModRequireVersions(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 || !strings.HasPrefix(fields[0], "github.com/yiiilin/harness-core") {
			continue
		}
		out[fields[0]] = fields[1]
	}
	return out, nil
}

func rewriteFile(path string, requirements map[string]string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := string(data)
	keys := make([]string, 0, len(requirements))
	for modulePath := range requirements {
		keys = append(keys, modulePath)
	}
	sort.Strings(keys)
	for _, modulePath := range keys {
		version := requirements[modulePath]
		re := regexp.MustCompile(`(?m)^(\s*)` + regexp.QuoteMeta(modulePath) + `\s+v[^\s]+(?:\s*//.*)?$`)
		if re.MatchString(out) {
			out = re.ReplaceAllString(out, "\t"+modulePath+" "+version)
			continue
		}
		var inserted bool
		out, inserted = insertRequireLine(out, modulePath, version)
		if !inserted {
			return nil, fmt.Errorf("%s: could not insert requirement for %s", path, modulePath)
		}
	}
	return []byte(out), nil
}

func insertRequireLine(goMod, modulePath, version string) (string, bool) {
	lines := strings.Split(goMod, "\n")
	inRequire := false
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "require (":
			inRequire = true
		case inRequire && trimmed == ")":
			inserted := append([]string{}, lines[:idx]...)
			inserted = append(inserted, "\t"+modulePath+" "+version)
			inserted = append(inserted, lines[idx:]...)
			return strings.Join(inserted, "\n"), true
		}
	}
	return goMod, false
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
