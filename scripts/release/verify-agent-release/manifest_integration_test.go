package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

const manifestFixtureTag = "v9.9.9-test"

type manifestFixture struct {
	root       string
	assetsDir  string
	mockBinDir string
	publicKey  string
	releases   map[string]*fixtureRelease
}

type fixtureRelease struct {
	TagName string         `json:"tag_name"`
	Draft   bool           `json:"draft"`
	Assets  []fixtureAsset `json:"assets"`
}

type fixtureAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
	State              string `json:"state"`
}

type generatedManifest struct {
	HubVersion string                         `json:"hub_version"`
	Agents     map[string]generatedAgentEntry `json:"agents"`
}

type generatedAgentEntry struct {
	Version  string                          `json:"version"`
	Binaries map[string]generatedBinaryEntry `json:"binaries"`
}

type generatedBinaryEntry struct {
	Name      string `json:"name"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
	URL       string `json:"url"`
	Signature string `json:"signature"`
}

func TestGenerateAgentManifestVerifiesCompleteSignedRelease(t *testing.T) {
	fixture := newManifestFixture(t)
	outputDir := filepath.Join(t.TempDir(), "agent-dist")
	output, err := fixture.runGenerator(t, outputDir)
	if err != nil {
		t.Fatalf("generate-agent-manifest.sh failed: %v\n%s", err, output)
	}

	manifestBytes, err := os.ReadFile(filepath.Join(outputDir, "agent-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest generatedManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.HubVersion != manifestFixtureTag {
		t.Fatalf("hub version = %q, want %q", manifest.HubVersion, manifestFixtureTag)
	}

	goAgent := manifest.Agents["labtether-agent"]
	if goAgent.Version != manifestFixtureTag {
		t.Fatalf("Go agent version = %q, want %q", goAgent.Version, manifestFixtureTag)
	}
	wantPlatforms := []string{"linux-amd64", "linux-arm64", "windows-amd64", "windows-arm64"}
	gotPlatforms := make([]string, 0, len(goAgent.Binaries))
	for platform, binary := range goAgent.Binaries {
		gotPlatforms = append(gotPlatforms, platform)
		if len(binary.SHA256) != 64 || binary.SizeBytes <= 0 || binary.Signature == "" {
			t.Fatalf("incomplete verified manifest entry for %s: %#v", platform, binary)
		}
		info, err := os.Stat(filepath.Join(outputDir, binary.Name))
		if err != nil {
			t.Fatalf("stat published %s: %v", platform, err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Fatalf("published %s mode = %o, want 755", platform, info.Mode().Perm())
		}
	}
	sort.Strings(gotPlatforms)
	if strings.Join(gotPlatforms, ",") != strings.Join(wantPlatforms, ",") {
		t.Fatalf("Go platforms = %v, want %v", gotPlatforms, wantPlatforms)
	}

	for agent, platform := range map[string]string{
		"labtether-mac": "darwin-universal",
		"labtether-win": "windows-x64",
	} {
		entry, ok := manifest.Agents[agent].Binaries[platform]
		if !ok || len(entry.SHA256) != 64 || entry.SizeBytes <= 0 {
			t.Fatalf("incomplete native manifest entry for %s/%s: %#v", agent, platform, entry)
		}
	}
}

func TestGenerateAgentManifestFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, fixture *manifestFixture)
	}{
		{
			name: "missing required Windows signature",
			mutate: func(t *testing.T, fixture *manifestFixture) {
				t.Helper()
				fixture.removeReleaseAsset(t, "go", "labtether-agent-windows-arm64.exe-"+manifestFixtureTag+".sig")
			},
		},
		{
			name: "release API tag mismatch",
			mutate: func(t *testing.T, fixture *manifestFixture) {
				t.Helper()
				fixture.releases["go"].TagName = "v9.9.8"
				fixture.writeReleases(t)
			},
		},
		{
			name: "draft release",
			mutate: func(t *testing.T, fixture *manifestFixture) {
				t.Helper()
				fixture.releases["go"].Draft = true
				fixture.writeReleases(t)
			},
		},
		{
			name: "required asset is not fully uploaded",
			mutate: func(t *testing.T, fixture *manifestFixture) {
				t.Helper()
				name := "labtether-agent-windows-amd64.exe-" + manifestFixtureTag + ".metadata.json"
				for index := range fixture.releases["go"].Assets {
					if fixture.releases["go"].Assets[index].Name == name {
						fixture.releases["go"].Assets[index].State = "starter"
					}
				}
				fixture.writeReleases(t)
			},
		},
		{
			name: "binary differs from GitHub digest",
			mutate: func(t *testing.T, fixture *manifestFixture) {
				t.Helper()
				fixture.writeAsset(t, "labtether-agent-linux-amd64", []byte("tampered after API metadata\n"))
			},
		},
		{
			name: "signature is invalid despite matching GitHub digest",
			mutate: func(t *testing.T, fixture *manifestFixture) {
				t.Helper()
				name := "labtether-agent-linux-amd64-" + manifestFixtureTag + ".sig"
				fixture.writeAsset(t, name, []byte(base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))+"\n"))
				fixture.refreshReleaseAsset(t, "go", name)
			},
		},
		{
			name: "native checksum differs from binary digest",
			mutate: func(t *testing.T, fixture *manifestFixture) {
				t.Helper()
				name := "labtether-agent-macos-universal.tar.gz.sha256"
				fixture.writeAsset(t, name, []byte(strings.Repeat("0", 64)+"  labtether-agent-macos-universal.tar.gz\n"))
				fixture.refreshReleaseAsset(t, "mac", name)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newManifestFixture(t)
			test.mutate(t, fixture)
			outputDir := filepath.Join(t.TempDir(), "agent-dist")
			if output, err := fixture.runGenerator(t, outputDir); err == nil {
				t.Fatalf("generator accepted an untrusted or incomplete release\n%s", output)
			}
			if _, err := os.Stat(filepath.Join(outputDir, "agent-manifest.json")); !os.IsNotExist(err) {
				t.Fatalf("generator published a manifest after verification failure: %v", err)
			}
		})
	}
}

func newManifestFixture(t *testing.T) *manifestFixture {
	t.Helper()
	root := t.TempDir()
	fixture := &manifestFixture{
		root:       root,
		assetsDir:  filepath.Join(root, "assets"),
		mockBinDir: filepath.Join(root, "bin"),
		publicKey:  filepath.Join(root, "public-key.b64"),
		releases: map[string]*fixtureRelease{
			"go":  {TagName: manifestFixtureTag},
			"mac": {TagName: manifestFixtureTag},
			"win": {TagName: manifestFixtureTag},
		},
	}
	if err := os.MkdirAll(fixture.assetsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(fixture.mockBinDir, 0o700); err != nil {
		t.Fatal(err)
	}

	seed := bytes.Repeat([]byte{0x24}, ed25519.SeedSize)
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	fixture.writeFile(t, fixture.publicKey, []byte(base64.StdEncoding.EncodeToString(publicKey)+"\n"), 0o600)

	goPlatforms := []struct {
		name string
		os   string
		arch string
	}{
		{name: "labtether-agent-linux-amd64", os: "linux", arch: "amd64"},
		{name: "labtether-agent-linux-arm64", os: "linux", arch: "arm64"},
		{name: "labtether-agent-windows-amd64.exe", os: "windows", arch: "amd64"},
		{name: "labtether-agent-windows-arm64.exe", os: "windows", arch: "arm64"},
	}
	for _, platform := range goPlatforms {
		binary := []byte("signed fixture binary for " + platform.os + "/" + platform.arch + "\n")
		fixture.writeAsset(t, platform.name, binary)
		digestBytes := sha256.Sum256(binary)
		digest := hex.EncodeToString(digestBytes[:])
		metadata := signedMetadata{
			Version:   manifestFixtureTag,
			OS:        platform.os,
			Arch:      platform.arch,
			SHA256:    digest,
			SizeBytes: int64(len(binary)),
		}
		signature := ed25519.Sign(privateKey, []byte(canonicalPayload(metadata)))
		metadata.Signature = base64.StdEncoding.EncodeToString(signature)
		metadataJSON, err := json.Marshal(metadata)
		if err != nil {
			t.Fatal(err)
		}
		fixture.writeAsset(t, platform.name+".sha256", []byte(digest+"  "+platform.name+"\n"))
		fixture.writeAsset(t, platform.name+"-"+manifestFixtureTag+".sig", []byte(metadata.Signature+"\n"))
		fixture.writeAsset(t, platform.name+"-"+manifestFixtureTag+".metadata.json", append(metadataJSON, '\n'))
		for _, suffix := range []string{"", ".sha256", "-" + manifestFixtureTag + ".sig", "-" + manifestFixtureTag + ".metadata.json"} {
			fixture.addReleaseAsset(t, "go", "labtether/labtether-agent", platform.name+suffix)
		}
	}

	fixture.addNativeRelease(t, "mac", "labtether/labtether-mac", "labtether-agent-macos-universal.tar.gz")
	fixture.addNativeRelease(t, "win", "labtether/labtether-win", "labtether-agent-win-x64.zip")
	fixture.writeReleases(t)
	fixture.writeMockCurl(t)
	return fixture
}

func (fixture *manifestFixture) addNativeRelease(t *testing.T, releaseKey, repo, name string) {
	t.Helper()
	binary := []byte("native fixture for " + repo + "\n")
	fixture.writeAsset(t, name, binary)
	digestBytes := sha256.Sum256(binary)
	digest := hex.EncodeToString(digestBytes[:])
	fixture.writeAsset(t, name+".sha256", []byte(digest+"  "+name+"\n"))
	fixture.addReleaseAsset(t, releaseKey, repo, name)
	fixture.addReleaseAsset(t, releaseKey, repo, name+".sha256")
}

func (fixture *manifestFixture) addReleaseAsset(t *testing.T, releaseKey, repo, name string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixture.assetsDir, name))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	fixture.releases[releaseKey].Assets = append(fixture.releases[releaseKey].Assets, fixtureAsset{
		Name:               name,
		BrowserDownloadURL: fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, manifestFixtureTag, name),
		Size:               int64(len(data)),
		Digest:             "sha256:" + hex.EncodeToString(digest[:]),
		State:              "uploaded",
	})
}

func (fixture *manifestFixture) removeReleaseAsset(t *testing.T, releaseKey, name string) {
	t.Helper()
	release := fixture.releases[releaseKey]
	filtered := release.Assets[:0]
	for _, asset := range release.Assets {
		if asset.Name != name {
			filtered = append(filtered, asset)
		}
	}
	if len(filtered) == len(release.Assets) {
		t.Fatalf("fixture asset %s was not present", name)
	}
	release.Assets = filtered
	fixture.writeReleases(t)
}

func (fixture *manifestFixture) refreshReleaseAsset(t *testing.T, releaseKey, name string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixture.assetsDir, name))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	found := false
	for index := range fixture.releases[releaseKey].Assets {
		asset := &fixture.releases[releaseKey].Assets[index]
		if asset.Name == name {
			asset.Size = int64(len(data))
			asset.Digest = "sha256:" + hex.EncodeToString(digest[:])
			found = true
		}
	}
	if !found {
		t.Fatalf("fixture asset %s was not present", name)
	}
	fixture.writeReleases(t)
}

func (fixture *manifestFixture) writeReleases(t *testing.T) {
	t.Helper()
	for key, release := range fixture.releases {
		data, err := json.Marshal(release)
		if err != nil {
			t.Fatal(err)
		}
		fixture.writeFile(t, filepath.Join(fixture.root, key+"-release.json"), append(data, '\n'), 0o600)
	}
}

func (fixture *manifestFixture) writeAsset(t *testing.T, name string, data []byte) {
	t.Helper()
	fixture.writeFile(t, filepath.Join(fixture.assetsDir, name), data, 0o600)
}

func (fixture *manifestFixture) writeFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}

func (fixture *manifestFixture) writeMockCurl(t *testing.T) {
	t.Helper()
	const mockCurl = `#!/usr/bin/env bash
set -euo pipefail
output=""
url=""
authorization_header=false
while (($#)); do
  case "$1" in
    -o)
      output="$2"
      shift 2
      ;;
    -H)
      if [[ "$2" == Authorization:* ]]; then
        authorization_header=true
      fi
      shift 2
      ;;
    --max-filesize|--max-redirs|--proto|--retry|--retry-delay)
      shift 2
      ;;
    --)
      shift
      url="${1:-}"
      break
      ;;
    *)
      shift
      ;;
  esac
done

case "${url}" in
  https://api.github.com/repos/labtether/labtether-agent/releases/tags/*)
    source_file="${LABTETHER_TEST_FIXTURE_ROOT}/go-release.json"
    ;;
  https://api.github.com/repos/labtether/labtether-mac/releases/tags/*)
    source_file="${LABTETHER_TEST_FIXTURE_ROOT}/mac-release.json"
    ;;
  https://api.github.com/repos/labtether/labtether-win/releases/tags/*)
    source_file="${LABTETHER_TEST_FIXTURE_ROOT}/win-release.json"
    ;;
  https://github.com/*/releases/download/*/*)
    if [[ "${authorization_header}" == true ]]; then
      echo "mock curl: authorization header leaked to release asset redirect" >&2
      exit 67
    fi
    source_file="${LABTETHER_TEST_FIXTURE_ROOT}/assets/${url##*/}"
    ;;
  *)
    echo "mock curl: unexpected URL: ${url}" >&2
    exit 66
    ;;
esac

if [[ -n "${output}" ]]; then
  cp -- "${source_file}" "${output}"
else
  cat -- "${source_file}"
fi
`
	// #nosec G306 -- the mock curl must be executable by the integration test.
	fixture.writeFile(t, filepath.Join(fixture.mockBinDir, "curl"), []byte(mockCurl), 0o700)
}

func (fixture *manifestFixture) runGenerator(t *testing.T, outputDir string) (string, error) {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve integration test source path")
	}
	hubRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
	script := filepath.Join(hubRoot, "scripts", "release", "generate-agent-manifest.sh")
	// #nosec G204 -- script is a repository-owned constant path and arguments are test temp directories.
	cmd := exec.Command("bash", script, manifestFixtureTag, outputDir)
	cmd.Dir = hubRoot
	environment := make([]string, 0, len(os.Environ())+5)
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "GITHUB_TOKEN=") ||
			strings.HasPrefix(value, "PATH=") ||
			strings.HasPrefix(value, "LABTETHER_AGENT_RELEASE_TRUSTED_PUBLIC_KEY_FILE=") ||
			strings.HasPrefix(value, "LABTETHER_RELEASE_LOOKUP_") ||
			strings.HasPrefix(value, "LABTETHER_TEST_FIXTURE_ROOT=") {
			continue
		}
		environment = append(environment, value)
	}
	environment = append(environment,
		"PATH="+fixture.mockBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GITHUB_TOKEN=fixture-api-token",
		"LABTETHER_TEST_FIXTURE_ROOT="+fixture.root,
		"LABTETHER_AGENT_RELEASE_TRUSTED_PUBLIC_KEY_FILE="+fixture.publicKey,
		"LABTETHER_RELEASE_LOOKUP_ATTEMPTS=1",
		"LABTETHER_RELEASE_LOOKUP_DELAY_SECONDS=0",
	)
	cmd.Env = environment
	combined, err := cmd.CombinedOutput()
	return string(combined), err
}
