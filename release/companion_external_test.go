package release_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCompanionModulesTrackCommittedCompatibilityMatrix(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Join("..")
	matrix := committedCompatibilityMatrix(t, repoRoot)
	expectations := map[string]map[string]string{
		"go.mod": {
			"github.com/yiiilin/harness-core/adapters":             matrix.companionVersion,
			"github.com/yiiilin/harness-core/modules":              matrix.companionVersion,
			"github.com/yiiilin/harness-core/pkg/harness/builtins": matrix.companionVersion,
		},
		"modules/go.mod": {
			"github.com/yiiilin/harness-core": matrix.rootVersion,
		},
		"pkg/harness/builtins/go.mod": {
			"github.com/yiiilin/harness-core":         matrix.rootVersion,
			"github.com/yiiilin/harness-core/modules": matrix.companionVersion,
		},
		"adapters/go.mod": {
			"github.com/yiiilin/harness-core":                      matrix.rootVersion,
			"github.com/yiiilin/harness-core/modules":              matrix.companionVersion,
			"github.com/yiiilin/harness-core/pkg/harness/builtins": matrix.companionVersion,
		},
		"cmd/harness-core/go.mod": {
			"github.com/yiiilin/harness-core":                      matrix.rootVersion,
			"github.com/yiiilin/harness-core/adapters":             matrix.companionVersion,
			"github.com/yiiilin/harness-core/modules":              matrix.companionVersion,
			"github.com/yiiilin/harness-core/pkg/harness/builtins": matrix.companionVersion,
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
	matrix := committedCompatibilityMatrix(t, repoRoot)
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
				"github.com/yiiilin/harness-core/adapters":             matrix.companionVersion,
				"github.com/yiiilin/harness-core":                      "dev",
				"github.com/yiiilin/harness-core/modules":              matrix.companionVersion,
				"github.com/yiiilin/harness-core/pkg/harness/builtins": matrix.companionVersion,
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
				"github.com/yiiilin/harness-core/adapters":             matrix.companionVersion,
				"github.com/yiiilin/harness-core":                      "dev",
				"github.com/yiiilin/harness-core/modules":              matrix.companionVersion,
				"github.com/yiiilin/harness-core/pkg/harness/builtins": matrix.companionVersion,
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
				"github.com/yiiilin/harness-core":         matrix.rootVersion,
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
				"github.com/yiiilin/harness-core":                      matrix.rootVersion,
				"github.com/yiiilin/harness-core/modules":              matrix.companionVersion,
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
				"github.com/yiiilin/harness-core":                      matrix.rootVersion,
				"github.com/yiiilin/harness-core/adapters":             "dev",
				"github.com/yiiilin/harness-core/modules":              matrix.companionVersion,
				"github.com/yiiilin/harness-core/pkg/harness/builtins": matrix.companionVersion,
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
				"github.com/yiiilin/harness-core":                      matrix.rootVersion,
				"github.com/yiiilin/harness-core/adapters":             "dev",
				"github.com/yiiilin/harness-core/modules":              matrix.companionVersion,
				"github.com/yiiilin/harness-core/pkg/harness/builtins": matrix.companionVersion,
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
					if isPseudoVersion(got) {
						continue
					}
					if tagged := taggedReleaseVersionForModule(t, snapshotRepo, modulePath); tagged != "" && got == tagged {
						continue
					}
					t.Fatalf("%s resolved %s at %q; want a dev pseudo-version or tagged release version", tc.name, modulePath, got)
				default:
					if got != want {
						t.Fatalf("%s resolved %s at %q; want %q", tc.name, modulePath, got, want)
					}
				}
			}
		})
	}
}

type compatibilityMatrix struct {
	rootVersion      string
	companionVersion string
}

func committedCompatibilityMatrix(t *testing.T, repoRoot string) compatibilityMatrix {
	t.Helper()

	requirements := map[string]map[string]string{
		"go.mod":                      goModRequireVersions(t, filepath.Join(repoRoot, "go.mod")),
		"modules/go.mod":              goModRequireVersions(t, filepath.Join(repoRoot, "modules/go.mod")),
		"pkg/harness/builtins/go.mod": goModRequireVersions(t, filepath.Join(repoRoot, "pkg/harness/builtins/go.mod")),
		"adapters/go.mod":             goModRequireVersions(t, filepath.Join(repoRoot, "adapters/go.mod")),
		"cmd/harness-core/go.mod":     goModRequireVersions(t, filepath.Join(repoRoot, "cmd/harness-core/go.mod")),
	}

	rootVersion := uniqueRequiredVersion(t, []requiredVersion{
		{file: "modules/go.mod", modulePath: "github.com/yiiilin/harness-core", version: requirements["modules/go.mod"]["github.com/yiiilin/harness-core"]},
		{file: "pkg/harness/builtins/go.mod", modulePath: "github.com/yiiilin/harness-core", version: requirements["pkg/harness/builtins/go.mod"]["github.com/yiiilin/harness-core"]},
		{file: "adapters/go.mod", modulePath: "github.com/yiiilin/harness-core", version: requirements["adapters/go.mod"]["github.com/yiiilin/harness-core"]},
		{file: "cmd/harness-core/go.mod", modulePath: "github.com/yiiilin/harness-core", version: requirements["cmd/harness-core/go.mod"]["github.com/yiiilin/harness-core"]},
	})

	companionVersion := uniqueRequiredVersion(t, []requiredVersion{
		{file: "go.mod", modulePath: "github.com/yiiilin/harness-core/adapters", version: requirements["go.mod"]["github.com/yiiilin/harness-core/adapters"]},
		{file: "go.mod", modulePath: "github.com/yiiilin/harness-core/modules", version: requirements["go.mod"]["github.com/yiiilin/harness-core/modules"]},
		{file: "go.mod", modulePath: "github.com/yiiilin/harness-core/pkg/harness/builtins", version: requirements["go.mod"]["github.com/yiiilin/harness-core/pkg/harness/builtins"]},
		{file: "pkg/harness/builtins/go.mod", modulePath: "github.com/yiiilin/harness-core/modules", version: requirements["pkg/harness/builtins/go.mod"]["github.com/yiiilin/harness-core/modules"]},
		{file: "adapters/go.mod", modulePath: "github.com/yiiilin/harness-core/modules", version: requirements["adapters/go.mod"]["github.com/yiiilin/harness-core/modules"]},
		{file: "adapters/go.mod", modulePath: "github.com/yiiilin/harness-core/pkg/harness/builtins", version: requirements["adapters/go.mod"]["github.com/yiiilin/harness-core/pkg/harness/builtins"]},
		{file: "cmd/harness-core/go.mod", modulePath: "github.com/yiiilin/harness-core/adapters", version: requirements["cmd/harness-core/go.mod"]["github.com/yiiilin/harness-core/adapters"]},
		{file: "cmd/harness-core/go.mod", modulePath: "github.com/yiiilin/harness-core/modules", version: requirements["cmd/harness-core/go.mod"]["github.com/yiiilin/harness-core/modules"]},
		{file: "cmd/harness-core/go.mod", modulePath: "github.com/yiiilin/harness-core/pkg/harness/builtins", version: requirements["cmd/harness-core/go.mod"]["github.com/yiiilin/harness-core/pkg/harness/builtins"]},
	})

	return compatibilityMatrix{
		rootVersion:      rootVersion,
		companionVersion: companionVersion,
	}
}

type requiredVersion struct {
	file       string
	modulePath string
	version    string
}

func uniqueRequiredVersion(t *testing.T, requirements []requiredVersion) string {
	t.Helper()

	want := ""
	for _, requirement := range requirements {
		if requirement.version == "" {
			t.Fatalf("%s is missing requirement for %s", requirement.file, requirement.modulePath)
		}
		if want == "" {
			want = requirement.version
			continue
		}
		if requirement.version != want {
			t.Fatalf("repo-local manifest matrix drift: %s requires %s at %q; expected %q", requirement.file, requirement.modulePath, requirement.version, want)
		}
	}
	if want == "" {
		t.Fatalf("empty requirement set")
	}
	return want
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
		"GOPROXY=direct",
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

func taggedReleaseVersionForModule(t *testing.T, repoRoot, modulePath string) string {
	t.Helper()

	raw := runCommand(t, repoRoot, nil, "git", "tag", "--points-at", "HEAD")
	tags := strings.Fields(raw)
	for _, tag := range tags {
		switch modulePath {
		case "github.com/yiiilin/harness-core":
			if strings.HasPrefix(tag, "v") && !strings.Contains(tag, "/") {
				return tag
			}
		case "github.com/yiiilin/harness-core/modules":
			if strings.HasPrefix(tag, "modules/") {
				return strings.TrimPrefix(tag, "modules/")
			}
		case "github.com/yiiilin/harness-core/pkg/harness/builtins":
			if strings.HasPrefix(tag, "pkg/harness/builtins/") {
				return strings.TrimPrefix(tag, "pkg/harness/builtins/")
			}
		case "github.com/yiiilin/harness-core/adapters":
			if strings.HasPrefix(tag, "adapters/") {
				return strings.TrimPrefix(tag, "adapters/")
			}
		case "github.com/yiiilin/harness-core/cmd/harness-core":
			if strings.HasPrefix(tag, "cmd/harness-core/") {
				return strings.TrimPrefix(tag, "cmd/harness-core/")
			}
		}
	}
	return ""
}

func runCommand(t *testing.T, dir string, extraEnv []string, name string, args ...string) string {
	t.Helper()

	for attempt := 1; attempt <= 3; attempt++ {
		cmd := exec.Command(name, args...)
		if dir != "" {
			cmd.Dir = dir
		}
		cmd.Env = append(os.Environ(), extraEnv...)
		output, err := cmd.CombinedOutput()
		if err == nil {
			return string(output)
		}
		if name == "go" && attempt < 3 && isTransientGoFetchFailure(output) {
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			continue
		}
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, output)
	}
	return ""
}

func isTransientGoFetchFailure(output []byte) bool {
	text := string(output)
	for _, marker := range []string{
		"unexpected EOF",
		"TLS handshake timeout",
		"connection reset by peer",
		"i/o timeout",
		"temporary failure",
		"502 Bad Gateway",
		"503 Service Unavailable",
		"504 Gateway Timeout",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
