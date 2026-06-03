package builder

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/openfaas/go-sdk/stack"
)

func Test_resolveTemplateDirFromHandler_UsesClosestAncestor(t *testing.T) {
	root := t.TempDir()
	templateRoot := filepath.Join(root, "template")
	templateYAML := filepath.Join(templateRoot, "python3-http", "template.yml")
	handlerDir := filepath.Join(root, "workflows", "linear2", "a")

	if err := os.MkdirAll(filepath.Dir(templateYAML), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(templateYAML, []byte("fprocess: python index.py\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(handlerDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got := resolveTemplateDirFromHandler(handlerDir, "python3-http")
	want := templateRoot

	if got != want {
		t.Fatalf("resolveTemplateDirFromHandler(%q) = %q; want %q", handlerDir, got, want)
	}
}

func Test_resolveTemplateDirFromHandler_FallbackToDefault(t *testing.T) {
	handlerDir := filepath.Join(t.TempDir(), "handler")
	if err := os.MkdirAll(handlerDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got := resolveTemplateDirFromHandler(handlerDir, "python3-http")
	want := "./template"

	if got != want {
		t.Fatalf("resolveTemplateDirFromHandler(%q) = %q; want %q", handlerDir, got, want)
	}
}

func Test_getDockerBuildCommand_NoOpts(t *testing.T) {
	dockerBuildVal := dockerBuild{
		Image:       "imagename:latest",
		NoCache:     false,
		Squash:      false,
		HTTPProxy:   "",
		HTTPSProxy:  "",
		BuildArgMap: make(map[string]string),
	}

	want := "build --tag imagename:latest ."
	wantCommand := "docker"

	command, args := getDockerBuildCommand(dockerBuildVal)

	joined := strings.Join(args, " ")

	if joined != want {
		t.Errorf("getDockerBuildCommand want: \"%s\", got: \"%s\"", want, joined)
	}

	if command != wantCommand {
		t.Errorf("getDockerBuildCommand want command: \"%s\", got: \"%s\"", wantCommand, command)
	}
}

func Test_getDockerBuildCommand_WithNoCache(t *testing.T) {
	dockerBuildVal := dockerBuild{
		Image:       "imagename:latest",
		NoCache:     true,
		Squash:      false,
		HTTPProxy:   "",
		HTTPSProxy:  "",
		BuildArgMap: make(map[string]string),
	}

	want := "build --no-cache --tag imagename:latest ."

	wantCommand := "docker"

	command, args := getDockerBuildCommand(dockerBuildVal)

	joined := strings.Join(args, " ")

	if joined != want {
		t.Errorf("getDockerBuildCommand want: \"%s\", got: \"%s\"", want, joined)
	}

	if command != wantCommand {
		t.Errorf("getDockerBuildCommand want command: \"%s\", got: \"%s\"", wantCommand, command)
	}
}

func Test_getDockerBuildCommand_WithProxies(t *testing.T) {
	dockerBuildVal := dockerBuild{
		Image:       "imagename:latest",
		NoCache:     false,
		Squash:      false,
		HTTPProxy:   "http://127.0.0.1:3128",
		HTTPSProxy:  "https://127.0.0.1:3128",
		BuildArgMap: make(map[string]string),
	}

	want := "build --build-arg http_proxy=http://127.0.0.1:3128 --build-arg https_proxy=https://127.0.0.1:3128 --tag imagename:latest ."

	wantCommand := "docker"

	command, args := getDockerBuildCommand(dockerBuildVal)

	joined := strings.Join(args, " ")

	if joined != want {
		t.Errorf("getDockerBuildCommand want: \"%s\", got: \"%s\"", want, joined)
	}

	if command != wantCommand {
		t.Errorf("getDockerBuildCommand want command: \"%s\", got: \"%s\"", wantCommand, command)
	}
}

func Test_getDockerBuildCommand_WithBuildArg(t *testing.T) {
	dockerBuildVal := dockerBuild{
		Image:   "imagename:latest",
		NoCache: false,
		Squash:  false,
		BuildArgMap: map[string]string{
			"USERNAME": "admin",
			"PASSWORD": "1234",
		},
	}

	_, values := getDockerBuildCommand(dockerBuildVal)

	joined := strings.Join(values, " ")
	wantArg1 := "--build-arg USERNAME=admin"
	wantArg2 := "--build-arg PASSWORD=1234"

	if strings.Contains(joined, wantArg1) == false {
		t.Errorf("want %s in %s, but didn't find it", wantArg1, joined)
	}
	if strings.Contains(joined, wantArg2) == false {
		t.Errorf("want %s in %s, but didn't find it", wantArg2, joined)
	}
}

func Test_getImageArchiveBuildCommand_DefaultsToDockerBuildx(t *testing.T) {
	output := filepath.Join(t.TempDir(), "dist", "fn.tar")
	dockerBuildVal := dockerBuild{
		Image:       "faasd.local/fn:latest",
		Platforms:   "linux/amd64,linux/arm64",
		Output:      output,
		BuildArgMap: make(map[string]string),
	}

	command, args, err := getImageArchiveBuildCommand(dockerBuildVal, "")
	if err != nil {
		t.Fatal(err)
	}

	want := "buildx build --progress=plain --platform=linux/amd64,linux/arm64 --output=type=oci,dest=" + output + " --tag faasd.local/fn:latest ."
	if got := strings.Join(args, " "); got != want {
		t.Fatalf("archive build args = %q, want %q", got, want)
	}
	if command != "docker" {
		t.Fatalf("archive build command = %q, want docker", command)
	}
}

func Test_getImageArchiveBuildCommand_Nerdctl(t *testing.T) {
	output := filepath.Join(t.TempDir(), "dist", "fn.tar")
	dockerBuildVal := dockerBuild{
		Image:       "faasd.local/fn:latest",
		Platforms:   "linux/amd64",
		Output:      output,
		BuildArgMap: make(map[string]string),
	}

	command, args, err := getImageArchiveBuildCommand(dockerBuildVal, buildEngineNerdctl)
	if err != nil {
		t.Fatal(err)
	}

	want := "build --progress=plain --platform=linux/amd64 --output=type=oci,dest=" + output + " --tag faasd.local/fn:latest ."
	if got := strings.Join(args, " "); got != want {
		t.Fatalf("archive build args = %q, want %q", got, want)
	}
	if command != "nerdctl" {
		t.Fatalf("archive build command = %q, want nerdctl", command)
	}
}

func Test_getImageArchiveBuildCommand_RejectsUnsupportedEngine(t *testing.T) {
	_, _, err := getImageArchiveBuildCommand(dockerBuild{}, "podman")
	if err == nil {
		t.Fatal("expected unsupported engine error")
	}
	if !strings.Contains(err.Error(), "unsupported FAAS_CLI_BUILD_ENGINE") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func Test_prepareLocalArchiveOutput(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	got, err := prepareLocalArchiveOutput("./dist/fn.tar")
	if err != nil {
		t.Fatal(err)
	}

	want, err := filepath.Abs(filepath.Join("dist", "fn.tar"))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("archive output path = %q, want %q", got, want)
	}
	if info, err := os.Stat(filepath.Join(tempDir, "dist")); err != nil {
		t.Fatalf("expected archive parent directory to exist: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("archive parent is not a directory")
	}
}

func Test_resolveArchiveBuildPlatforms(t *testing.T) {
	t.Setenv(buildPlatformsEnv, "")
	if got := resolveArchiveBuildPlatforms(""); got != defaultArchiveTarget {
		t.Fatalf("default archive platforms = %q, want %q", got, defaultArchiveTarget)
	}
	if got := resolveArchiveBuildPlatforms("linux/arm64"); got != "linux/arm64" {
		t.Fatalf("stack archive platforms = %q, want linux/arm64", got)
	}

	t.Setenv(buildPlatformsEnv, "linux/amd64")
	if got := resolveArchiveBuildPlatforms("linux/amd64,linux/arm64"); got != "linux/amd64" {
		t.Fatalf("override archive platforms = %q, want linux/amd64", got)
	}
}

func Test_buildFlagSlice(t *testing.T) {

	var buildFlagOpts = []struct {
		title         string
		nocache       bool
		squash        bool
		httpProxy     string
		httpsProxy    string
		buildArgMap   map[string]string
		expectedSlice []string
		buildLabelMap map[string]string
	}{
		{
			title:         "no cache only",
			nocache:       true,
			squash:        false,
			httpProxy:     "",
			httpsProxy:    "",
			buildArgMap:   make(map[string]string),
			expectedSlice: []string{"--no-cache"},
		},
		{
			title:         "no cache & squash only",
			nocache:       true,
			squash:        true,
			httpProxy:     "",
			httpsProxy:    "",
			buildArgMap:   make(map[string]string),
			expectedSlice: []string{"--no-cache", "--squash"},
		},
		{
			title:         "no cache & squash & http proxy only",
			nocache:       true,
			squash:        true,
			httpProxy:     "192.168.0.1",
			httpsProxy:    "",
			buildArgMap:   make(map[string]string),
			expectedSlice: []string{"--no-cache", "--squash", "--build-arg", "http_proxy=192.168.0.1"},
		},
		{
			title:         "no cache & squash & https-proxy only",
			nocache:       true,
			squash:        true,
			httpProxy:     "",
			httpsProxy:    "127.0.0.1",
			buildArgMap:   make(map[string]string),
			expectedSlice: []string{"--no-cache", "--squash", "--build-arg", "https_proxy=127.0.0.1"},
		},
		{
			title:         "no cache & squash & http-proxy & https-proxy only",
			nocache:       true,
			squash:        true,
			httpProxy:     "192.168.0.1",
			httpsProxy:    "127.0.0.1",
			buildArgMap:   make(map[string]string),
			expectedSlice: []string{"--no-cache", "--squash", "--build-arg", "http_proxy=192.168.0.1", "--build-arg", "https_proxy=127.0.0.1"},
		},
		{
			title:         "http-proxy & https-proxy only",
			nocache:       false,
			squash:        false,
			httpProxy:     "192.168.0.1",
			httpsProxy:    "127.0.0.1",
			buildArgMap:   make(map[string]string),
			expectedSlice: []string{"--build-arg", "http_proxy=192.168.0.1", "--build-arg", "https_proxy=127.0.0.1"},
		},
		{
			title:      "build arg map no spaces",
			nocache:    false,
			squash:     false,
			httpProxy:  "",
			httpsProxy: "",
			buildArgMap: map[string]string{
				"muppet": "ernie",
			},
			expectedSlice: []string{"--build-arg", "muppet=ernie"},
		},
		{
			title:      "build arg map with spaces",
			nocache:    false,
			squash:     false,
			httpProxy:  "",
			httpsProxy: "",
			buildArgMap: map[string]string{
				"muppets": "burt and ernie",
			},
			expectedSlice: []string{"--build-arg", "muppets=burt and ernie"},
		},
		{
			title:      "multiple build arg map with spaces",
			nocache:    false,
			squash:     false,
			httpProxy:  "",
			httpsProxy: "",
			buildArgMap: map[string]string{
				"muppets":    "burt and ernie",
				"playschool": "Jemima",
			},
			expectedSlice: []string{"--build-arg", "muppets=burt and ernie", "--build-arg", "playschool=Jemima"},
		},
		{
			title:      "no-cache and squash with multiple build arg map with spaces",
			nocache:    true,
			squash:     true,
			httpProxy:  "",
			httpsProxy: "",
			buildArgMap: map[string]string{
				"muppets":    "burt and ernie",
				"playschool": "Jemima",
			},
			expectedSlice: []string{"--no-cache", "--squash", "--build-arg", "muppets=burt and ernie", "--build-arg", "playschool=Jemima"},
		},
		{
			title:      "single build-label value",
			nocache:    false,
			squash:     false,
			httpProxy:  "",
			httpsProxy: "",
			buildArgMap: map[string]string{
				"muppets":    "burt and ernie",
				"playschool": "Jemima",
			},
			buildLabelMap: map[string]string{
				"org.label-schema.name": "test function",
			},
			expectedSlice: []string{"--build-arg", "muppets=burt and ernie", "--build-arg", "playschool=Jemima", "--label", "org.label-schema.name=test function"},
		},
		{
			title:      "multiple build-label values",
			nocache:    false,
			squash:     false,
			httpProxy:  "",
			httpsProxy: "",
			buildArgMap: map[string]string{
				"muppets":    "burt and ernie",
				"playschool": "Jemima",
			},
			buildLabelMap: map[string]string{
				"org.label-schema.name":        "test function",
				"org.label-schema.description": "This is a test function",
			},
			expectedSlice: []string{"--build-arg", "muppets=burt and ernie", "--build-arg", "playschool=Jemima", "--label", "org.label-schema.name=test function", "--label", "org.label-schema.description=This is a test function"},
		},
	}

	for _, test := range buildFlagOpts {

		t.Run(test.title, func(t *testing.T) {

			forcePull := false
			flagSlice := buildFlagSlice(test.nocache, test.squash, test.httpProxy, test.httpsProxy, test.buildArgMap, test.buildLabelMap, forcePull)
			fmt.Println(flagSlice)
			if len(flagSlice) != len(test.expectedSlice) {
				t.Errorf("Slices differ in size - wanted: %d, found %d", len(test.expectedSlice), len(flagSlice))
			}

			isMatch := compareSliceValues(test.expectedSlice, flagSlice)
			if !isMatch {
				t.Errorf("Slices differ in values - wanted: %v, found %s", test.expectedSlice, flagSlice)
			}

		})
	}

}

func compareSliceValues(expectedSlice, actualSlice []string) bool {
	var expectedValueMap = make(map[string]int)
	for _, expectedValue := range expectedSlice {
		if _, ok := expectedValueMap[expectedValue]; ok {
			expectedValueMap[expectedValue]++
		} else {
			expectedValueMap[expectedValue] = 1
		}
	}

	var actualValueMap = make(map[string]int)
	for _, actualValue := range actualSlice {
		if _, ok := actualValueMap[actualValue]; ok {
			actualValueMap[actualValue]++
		} else {
			actualValueMap[actualValue] = 1
		}
	}

	if len(expectedValueMap) != len(actualValueMap) {
		return false
	}

	for key, expectedValue := range expectedValueMap {
		actualValue, exists := actualValueMap[key]
		if !exists || actualValue != expectedValue {
			return false
		}
	}

	return true
}

func Test_getPackages(t *testing.T) {
	var buildOpts = []struct {
		title                 string
		availableBuildOptions []stack.BuildOption
		requestedBuildOptions []string
		expectedPackages      []string
	}{
		{
			title: "Single Option",
			availableBuildOptions: []stack.BuildOption{
				{Name: "dev",
					Packages: []string{"jq", "hw", "ke"}},
			},
			requestedBuildOptions: []string{"dev"},
			expectedPackages:      []string{"jq", "hw", "ke"},
		},
		{
			title: "Two Options one chosen",
			availableBuildOptions: []stack.BuildOption{
				{Name: "dev",
					Packages: []string{"jq", "hw", "ke"}},
				{Name: "debug",
					Packages: []string{"lr", "kt", "jy"}},
			},
			requestedBuildOptions: []string{"dev"},
			expectedPackages:      []string{"jq", "hw", "ke"},
		},
		{
			title: "Two Options two chosen",
			availableBuildOptions: []stack.BuildOption{
				{Name: "dev",
					Packages: []string{"jq", "hw", "ke"}},
				{Name: "debug",
					Packages: []string{"lr", "kt", "jy"}},
			},
			requestedBuildOptions: []string{"dev", "debug"},
			expectedPackages:      []string{"jq", "hw", "ke", "lr", "kt", "jy"},
		},
		{
			title: "Two Options two chosen with overlaps",
			availableBuildOptions: []stack.BuildOption{
				{Name: "dev",
					Packages: []string{"jq", "hw", "ke"}},
				{Name: "debug",
					Packages: []string{"lr", "jq", "hw"}},
			},
			requestedBuildOptions: []string{"dev", "debug"},
			expectedPackages:      []string{"jq", "hw", "ke", "lr"},
		},
	}
	for _, test := range buildOpts {

		t.Run(test.title, func(t *testing.T) {

			buildOptPackages, _ := getPackages(test.availableBuildOptions, test.requestedBuildOptions)

			if len(buildOptPackages) != len(test.expectedPackages) {
				t.Errorf("Slices differ in size - wanted: %d, found %d", len(test.expectedPackages), len(buildOptPackages))
			}
			for _, expectedOptPackage := range test.expectedPackages {
				found := false
				for _, buildOptPackage := range buildOptPackages {
					if buildOptPackage == expectedOptPackage {
						found = true
						break
					}
				}
				if found == false {

					t.Errorf("Slices differ in values  - wanted: %s, found %s", strings.Join(test.expectedPackages, " "), strings.Join(buildOptPackages, " "))
				}
			}
		})
	}
}

func Test_deDuplicate(t *testing.T) {
	var stringOpts = []struct {
		title           string
		inputStrings    []string
		expectedStrings []string
	}{
		{
			title:           "No Duplicates",
			inputStrings:    []string{"jq", "hw", "ke"},
			expectedStrings: []string{"jq", "hw", "ke"},
		},
		{
			title:           "Duplicates",
			inputStrings:    []string{"jq", "hw", "ke", "jq", "hw", "ke"},
			expectedStrings: []string{"jq", "hw", "ke"},
		},
	}
	for _, test := range stringOpts {

		t.Run(test.title, func(t *testing.T) {

			uniqueStrings := deDuplicate(test.inputStrings)

			if len(uniqueStrings) != len(test.expectedStrings) {
				t.Errorf("Slices differ in size - wanted: %d, found %d", len(test.expectedStrings), len(uniqueStrings))
			}

			for _, expectedString := range test.expectedStrings {

				found := false

				for _, uniqueString := range uniqueStrings {

					if expectedString == uniqueString {
						found = true
						break
					}
				}

				if found == false {
					t.Errorf("Slices differ in values  - wanted: %s, found %s", strings.Join(test.expectedStrings, " "), strings.Join(uniqueStrings, " "))
				}
			}
		})
	}
}

func Test_pathInScope(t *testing.T) {
	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("unexpected error during test setup: %s", err)
	}

	cases := []struct {
		name         string
		path         string
		expectedPath string
		err          bool
	}{
		{
			name:         "can copy folders without any relative path prefix",
			path:         "common/models/prebuilt",
			expectedPath: filepath.Join(root, "common/models/prebuilt"),
		},
		{
			name:         "can copy folders with relative path prefix",
			path:         "./common/data/cleaned",
			expectedPath: filepath.Join(root, "./common/data/cleaned"),
		},
		{
			name: "error if path equals the current directory",
			path: "./",
			err:  true,
		},
		{
			name: "error if relative path moves out of the current directory",
			path: "../private",
			err:  true,
		},
		{
			name: "error if absolute path is outside of the current directory",
			path: "/private/common",
			err:  true,
		},
		{
			name: "error if relative path moves out of the current directory, even when hidden in the middle of the path",
			path: "./common/../../private",
			err:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			abs, err := pathInScope(tc.path, root)
			switch {
			case tc.err && err == nil:
				t.Fatalf("expected error but got none")
			case tc.err && !strings.HasPrefix(err.Error(), "forbidden path"):
				t.Fatalf("expected forbidden path error, got \"%s\"", err)
			case !tc.err && err != nil:
				t.Fatalf("unexpected err \"%s\"", err)
			default:
				if abs != tc.expectedPath {
					t.Fatalf("expected path %s, got %s", tc.expectedPath, abs)
				}
			}
		})
	}
}

func Test_appendAdditionalPackages(t *testing.T) {
	var testCases = []struct {
		title              string
		buildArgs          map[string]string
		additionalPackages []string
		expectedBuildArgs  map[string]string
	}{
		{
			title:              "empty inputs",
			buildArgs:          make(map[string]string),
			additionalPackages: []string{},
			expectedBuildArgs: map[string]string{
				AdditionalPackageBuildArg: "",
			},
		},
		{
			title: "no ADDITIONAL_PACKAGE key in buildArgs",
			buildArgs: map[string]string{
				"OTHER_ARG": "some_value",
			},
			additionalPackages: []string{"package1", "package2"},
			expectedBuildArgs: map[string]string{
				"OTHER_ARG":               "some_value",
				AdditionalPackageBuildArg: "package1 package2",
			},
		},
		{
			title: "ADDITIONAL_PACKAGE key with single package",
			buildArgs: map[string]string{
				AdditionalPackageBuildArg: "existing_package",
			},
			additionalPackages: []string{"new_package"},
			expectedBuildArgs: map[string]string{
				AdditionalPackageBuildArg: "existing_package new_package",
			},
		},
		{
			title: "ADDITIONAL_PACKAGE key with multiple packages",
			buildArgs: map[string]string{
				AdditionalPackageBuildArg: "package1 package2 package3",
			},
			additionalPackages: []string{"package4", "package5"},
			expectedBuildArgs: map[string]string{
				AdditionalPackageBuildArg: "package1 package2 package3 package4 package5",
			},
		},
		{
			title: "ADDITIONAL_PACKAGE key with duplicates",
			buildArgs: map[string]string{
				AdditionalPackageBuildArg: "package1 package2",
			},
			additionalPackages: []string{"package2", "package3", "package1"},
			expectedBuildArgs: map[string]string{
				AdditionalPackageBuildArg: "package1 package2 package3",
			},
		},
		{
			title: "ADDITIONAL_PACKAGE key with duplicates in buildArgs value",
			buildArgs: map[string]string{
				AdditionalPackageBuildArg: "package1 package2 package1",
			},
			additionalPackages: []string{"package3", "package2"},
			expectedBuildArgs: map[string]string{
				AdditionalPackageBuildArg: "package1 package2 package3",
			},
		},
		{
			title: "ADDITIONAL_PACKAGE key with empty value",
			buildArgs: map[string]string{
				AdditionalPackageBuildArg: "",
			},
			additionalPackages: []string{"package1", "package2"},
			expectedBuildArgs: map[string]string{
				AdditionalPackageBuildArg: "package1 package2",
			},
		},
		{
			title: "ADDITIONAL_PACKAGE key with spaces only",
			buildArgs: map[string]string{
				AdditionalPackageBuildArg: "   ",
			},
			additionalPackages: []string{"package1"},
			expectedBuildArgs: map[string]string{
				AdditionalPackageBuildArg: "package1",
			},
		},
		{
			title: "multiple build args with ADDITIONAL_PACKAGE",
			buildArgs: map[string]string{
				"BUILD_VERSION":           "1.0.0",
				AdditionalPackageBuildArg: "existing_package",
				"ENVIRONMENT":             "production",
			},
			additionalPackages: []string{"new_package"},
			expectedBuildArgs: map[string]string{
				"BUILD_VERSION":           "1.0.0",
				AdditionalPackageBuildArg: "existing_package new_package",
				"ENVIRONMENT":             "production",
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.title, func(t *testing.T) {
			got := appendAdditionalPackages(test.buildArgs, test.additionalPackages)

			if !reflect.DeepEqual(got, test.expectedBuildArgs) {
				t.Errorf("Expected %v, got %v", test.expectedBuildArgs, got)
			}
		})
	}
}
