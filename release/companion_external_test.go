package release_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestCompanionModulesTrackCurrentRepoLocalVersions(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Join("..")
	rootVersion, companionVersion := repoLocalDevVersions(t, repoRoot)
	expectations := map[string]map[string]string{
		"go.mod": {
			"github.com/yiiilin/harness-core/adapters":             companionVersion,
			"github.com/yiiilin/harness-core/modules":              companionVersion,
			"github.com/yiiilin/harness-core/pkg/harness/builtins": companionVersion,
		},
		"modules/go.mod": {
			"github.com/yiiilin/harness-core": rootVersion,
		},
		"pkg/harness/builtins/go.mod": {
			"github.com/yiiilin/harness-core":         rootVersion,
			"github.com/yiiilin/harness-core/modules": companionVersion,
		},
		"adapters/go.mod": {
			"github.com/yiiilin/harness-core":                      rootVersion,
			"github.com/yiiilin/harness-core/modules":              companionVersion,
			"github.com/yiiilin/harness-core/pkg/harness/builtins": companionVersion,
		},
		"cmd/harness-core/go.mod": {
			"github.com/yiiilin/harness-core":                      rootVersion,
			"github.com/yiiilin/harness-core/adapters":             companionVersion,
			"github.com/yiiilin/harness-core/modules":              companionVersion,
			"github.com/yiiilin/harness-core/pkg/harness/builtins": companionVersion,
		},
	}

	for rel, modules := range expectations {
		rel := rel
		modules := modules
		t.Run(rel, func(t *testing.T) {
			t.Parallel()

			requirements := goModRequireVersions(t, filepath.Join(repoRoot, rel))
			for modulePath, want := range modules {
				if got := requirements[modulePath]; got != want {
					t.Fatalf("%s requires %s at %q; want %q", rel, modulePath, got, want)
				}
			}
		})
	}
}

func TestExternalConsumersBuildAgainstSnapshotRepo(t *testing.T) {
	repoRoot := filepath.Join("..")
	rootVersion, companionVersion := repoLocalDevVersions(t, repoRoot)
	snapshotRepo := snapshotRepository(t, repoRoot)

	cases := []struct {
		name           string
		modulePath     string
		mainSource     string
		build          bool
		expectedModule map[string]string
	}{
		{
			name:       "root-module",
			modulePath: "github.com/yiiilin/harness-core",
			build:      false,
			expectedModule: map[string]string{
				"github.com/yiiilin/harness-core/adapters":             companionVersion,
				"github.com/yiiilin/harness-core":                      "dev",
				"github.com/yiiilin/harness-core/modules":              companionVersion,
				"github.com/yiiilin/harness-core/pkg/harness/builtins": companionVersion,
			},
		},
		{
			name:       "root-kernel-package",
			modulePath: "github.com/yiiilin/harness-core/pkg/harness",
			build:      true,
			mainSource: `package main

import harness "github.com/yiiilin/harness-core/pkg/harness"

func main() {
	_ = harness.NewDefault
}
`,
			expectedModule: map[string]string{
				"github.com/yiiilin/harness-core/adapters":             companionVersion,
				"github.com/yiiilin/harness-core":                      "dev",
				"github.com/yiiilin/harness-core/modules":              companionVersion,
				"github.com/yiiilin/harness-core/pkg/harness/builtins": companionVersion,
			},
		},
		{
			name:       "modules-shell-package",
			modulePath: "github.com/yiiilin/harness-core/modules/shell",
			build:      true,
			mainSource: `package main

import (
	shellmodule "github.com/yiiilin/harness-core/modules/shell"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

func main() {
	var _ = shellmodule.RegisterWithOptions
	var _ hruntime.Options
}
`,
			expectedModule: map[string]string{
				"github.com/yiiilin/harness-core":         rootVersion,
				"github.com/yiiilin/harness-core/modules": "dev",
			},
		},
		{
			name:       "builtins-package",
			modulePath: "github.com/yiiilin/harness-core/pkg/harness/builtins",
			build:      true,
			mainSource: `package main

import (
	builtins "github.com/yiiilin/harness-core/pkg/harness/builtins"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

func main() {
	var opts hruntime.Options
	builtins.Register(&opts)
}
`,
			expectedModule: map[string]string{
				"github.com/yiiilin/harness-core":                      rootVersion,
				"github.com/yiiilin/harness-core/modules":              companionVersion,
				"github.com/yiiilin/harness-core/pkg/harness/builtins": "dev",
			},
		},
		{
			name:       "adapters-http-package",
			modulePath: "github.com/yiiilin/harness-core/adapters/http",
			build:      true,
			mainSource: `package main

import (
	httpadapter "github.com/yiiilin/harness-core/adapters/http"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

func main() {
	_ = httpadapter.New(hruntime.New(hruntime.Options{}))
}
`,
			expectedModule: map[string]string{
				"github.com/yiiilin/harness-core":                      rootVersion,
				"github.com/yiiilin/harness-core/adapters":             "dev",
				"github.com/yiiilin/harness-core/modules":              companionVersion,
				"github.com/yiiilin/harness-core/pkg/harness/builtins": companionVersion,
			},
		},
		{
			name:       "adapters-websocket-package",
			modulePath: "github.com/yiiilin/harness-core/adapters/websocket",
			build:      true,
			mainSource: `package main

import (
	adapterws "github.com/yiiilin/harness-core/adapters/websocket"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

func main() {
	_ = adapterws.New(adapterws.Config{Addr: "127.0.0.1:0"}, hruntime.New(hruntime.Options{}))
}
`,
			expectedModule: map[string]string{
				"github.com/yiiilin/harness-core":                      rootVersion,
				"github.com/yiiilin/harness-core/adapters":             "dev",
				"github.com/yiiilin/harness-core/modules":              companionVersion,
				"github.com/yiiilin/harness-core/pkg/harness/builtins": companionVersion,
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			workDir := t.TempDir()
			runCommand(t, workDir, nil, "go", "mod", "init", "example.com/consumer")

			env := externalConsumerEnv(t, workDir, snapshotRepo)
			runCommand(t, workDir, env, "go", "get", tc.modulePath+"@dev")
			if tc.build {
				if err := os.WriteFile(filepath.Join(workDir, "main.go"), []byte(tc.mainSource), 0o644); err != nil {
					t.Fatalf("write main.go: %v", err)
				}
				runCommand(t, workDir, env, "go", "build", "./...")
			}

			modules := listedModules(t, workDir, env)
			for modulePath, want := range tc.expectedModule {
				got := modules[modulePath]
				switch want {
				case "dev":
					if !strings.Contains(got, "-") {
						t.Fatalf("%s resolved %s at %q; want a dev pseudo-version", tc.name, modulePath, got)
					}
				default:
					if got != want {
						t.Fatalf("%s resolved %s at %q; want %q", tc.name, modulePath, got, want)
					}
				}
			}
		})
	}
}

func repoLocalDevVersions(t *testing.T, repoRoot string) (root string, companion string) {
	t.Helper()

	headShort := strings.TrimSpace(runCommand(t, repoRoot, nil, "git", "rev-parse", "--short=12", "HEAD"))
	headTime := strings.TrimSpace(runCommand(t, repoRoot, []string{"TZ=UTC"}, "git", "show", "-s", "--format=%cd", "--date=format-local:%Y%m%d%H%M%S", "HEAD"))
	rootTag := latestRootTag(t, repoRoot)
	major, minor, patch := parseReleaseTag(t, rootTag)
	root = fmt.Sprintf("v%d.%d.%d-0.%s-%s", major, minor, patch+1, headTime, headShort)
	companion = fmt.Sprintf("v0.0.0-%s-%s", headTime, headShort)
	return root, companion
}

func latestRootTag(t *testing.T, repoRoot string) string {
	t.Helper()

	raw := runCommand(t, repoRoot, nil, "git", "tag", "--list", "v*")
	tags := strings.Fields(raw)
	best := ""
	bestMajor, bestMinor, bestPatch := -1, -1, -1
	for _, tag := range tags {
		if !releaseTagPattern.MatchString(tag) {
			continue
		}
		major, minor, patch := parseReleaseTag(t, tag)
		if major > bestMajor || (major == bestMajor && minor > bestMinor) || (major == bestMajor && minor == bestMinor && patch > bestPatch) {
			best = tag
			bestMajor, bestMinor, bestPatch = major, minor, patch
		}
	}
	if best == "" {
		t.Fatalf("no root semver tags found")
	}
	return best
}

var releaseTagPattern = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)\.([0-9]+)$`)

func parseReleaseTag(t *testing.T, tag string) (major, minor, patch int) {
	t.Helper()

	matches := releaseTagPattern.FindStringSubmatch(tag)
	if len(matches) != 4 {
		t.Fatalf("unsupported release tag %q", tag)
	}
	if _, err := fmt.Sscanf(tag, "v%d.%d.%d", &major, &minor, &patch); err != nil {
		t.Fatalf("parse release tag %q: %v", tag, err)
	}
	return major, minor, patch
}

func goModRequireVersions(t *testing.T, path string) map[string]string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 || !strings.HasPrefix(fields[0], "github.com/yiiilin/harness-core") {
			continue
		}
		out[fields[0]] = fields[1]
	}
	return out
}

func snapshotRepository(t *testing.T, repoRoot string) string {
	t.Helper()

	cloneDir := filepath.Join(t.TempDir(), "repo")
	runCommand(t, "", nil, "git", "clone", "--quiet", repoRoot, cloneDir)
	runCommand(t, cloneDir, nil, "git", "checkout", "-B", "dev")
	runCommand(t, "", nil, "bash", "-lc", fmt.Sprintf("cd %q && tar --exclude=.git -cf - . | (cd %q && tar -xf -)", repoRoot, cloneDir))
	runCommand(t, cloneDir, nil, "git", "config", "user.email", "release-test@example.com")
	runCommand(t, cloneDir, nil, "git", "config", "user.name", "release-test")
	runCommand(t, cloneDir, nil, "git", "add", "-A")
	if hasStagedChanges(t, cloneDir) {
		runCommand(t, cloneDir, nil, "git", "commit", "--quiet", "-m", "release test snapshot")
	}
	return cloneDir
}

func hasStagedChanges(t *testing.T, repoRoot string) bool {
	t.Helper()

	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()
	err := cmd.Run()
	if err == nil {
		return false
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return true
	}
	t.Fatalf("git diff --cached --quiet failed: %v", err)
	return false
}

func externalConsumerEnv(t *testing.T, workDir, snapshotRepo string) []string {
	t.Helper()

	configPath := filepath.Join(workDir, "gitconfig")
	config := fmt.Sprintf(`[url "file://%s"]
	insteadOf = https://github.com/yiiilin/harness-core
	insteadOf = ssh://git@github.com/yiiilin/harness-core
	insteadOf = git@github.com:yiiilin/harness-core
`, filepath.ToSlash(snapshotRepo))
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write git config: %v", err)
	}
	return []string{
		"GOWORK=off",
		"GOPROXY=https://proxy.golang.org,direct",
		"GOSUMDB=off",
		"GONOSUMDB=github.com/yiiilin/harness-core",
		"GOPRIVATE=github.com/yiiilin/harness-core",
		"GONOPROXY=github.com/yiiilin/harness-core",
		"GIT_ALLOW_PROTOCOL=file:https:ssh",
		"GIT_CONFIG_GLOBAL=" + configPath,
		"GOMODCACHE=" + filepath.Join(workDir, "gomodcache"),
		"GOCACHE=" + filepath.Join(workDir, "gocache"),
	}
}

func listedModules(t *testing.T, workDir string, env []string) map[string]string {
	t.Helper()

	raw := runCommand(t, workDir, env, "go", "list", "-m", "-json", "all")
	dec := json.NewDecoder(strings.NewReader(raw))
	out := map[string]string{}
	for dec.More() {
		var module struct {
			Path    string
			Version string
			Main    bool
		}
		if err := dec.Decode(&module); err != nil {
			t.Fatalf("decode module listing: %v", err)
		}
		if module.Main {
			continue
		}
		out[module.Path] = module.Version
	}
	return out
}

func runCommand(t *testing.T, dir string, extraEnv []string, name string, args ...string) string {
	t.Helper()

	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), extraEnv...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, output)
	}
	return string(output)
}
