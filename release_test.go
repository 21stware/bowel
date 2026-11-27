package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// releaseWorkflow is what the tests read of release.yml.
type releaseWorkflow struct {
	Jobs map[string]struct {
		If    string
		Steps []struct {
			Uses string
			With struct{ Path string }
		}
	}
}

func readReleaseWorkflow(t *testing.T) releaseWorkflow {
	t.Helper()
	var workflow releaseWorkflow
	data, err := os.ReadFile(".github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	return workflow
}

// desktopUploads returns the globs of the files that the desktop jobs upload,
// which become the release.
func desktopUploads(t *testing.T, workflow releaseWorkflow) []string {
	t.Helper()
	var patterns []string
	for _, step := range workflow.Jobs["desktop"].Steps {
		if strings.HasPrefix(step.Uses, "actions/upload-artifact@") {
			patterns = append(patterns, strings.Fields(step.With.Path)...)
		}
	}
	if len(patterns) == 0 {
		t.Fatal("desktop artifact upload has no paths")
	}
	return patterns
}

// Exercise the desktop upload's file selection and the script that assembles
// those artifacts into a release. Signing is stubbed; no release key is needed.
func TestLinuxInstallerReleaseAsset(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the release assembly script runs on Linux")
	}
	patterns := desktopUploads(t, readReleaseWorkflow(t))
	root := t.TempDir()
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	installer := "#!/bin/sh\n# Generated for both Linux architectures.\n"
	for _, arch := range []string{"amd64", "arm64"} {
		job := filepath.Join(root, arch)
		build := filepath.Join(job, "build", "linux-"+arch)
		write(filepath.Join(build, "install.sh"), installer)
		write(filepath.Join(build, "bowel-0.3.0-linux-"+arch+".tar.gz"), "archive fixture")
		write(filepath.Join(build, "bowel-0.3.0-1."+arch+".rpm"), "rpm fixture")
		for _, pattern := range patterns {
			matches, err := filepath.Glob(filepath.Join(job, pattern))
			if err != nil {
				t.Fatal(err)
			}
			for _, match := range matches {
				content, err := os.ReadFile(match)
				if err != nil {
					t.Fatal(err)
				}
				write(filepath.Join(root, "release", "linux-"+arch, filepath.Base(match)), string(content))
			}
		}
	}
	bin := filepath.Join(root, "bin")
	write(filepath.Join(bin, "gpg"), "#!/bin/sh\ncase \"$*\" in *--list-secret-keys*) printf 'fpr:::::::::TESTKEY:\\n';; *) cat >/dev/null;; esac\n")
	write(filepath.Join(bin, "gpgconf"), "#!/bin/sh\nexit 0\n")
	write(filepath.Join(bin, "rpmsign"), "#!/bin/sh\nexit 0\n")
	out := filepath.Join(root, "dist")
	cmd := exec.Command("bash", "scripts/prepare-release.sh", filepath.Join(root, "release"), out)
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "PACKAGES_GPG_PRIVATE_KEY=test-only")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("prepare-release: %v\n%s", err, output)
	}
	got, err := os.ReadFile(filepath.Join(out, "install.sh"))
	if err != nil {
		t.Fatalf("Linux installer missing from release: %v", err)
	}
	if string(got) != installer {
		t.Fatalf("installer changed while assembling the release: %q", got)
	}
	sums, err := os.ReadFile(filepath.Join(out, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("%x  install.sh", sha256.Sum256(got))
	if !strings.Contains(string(sums), want+"\n") {
		t.Fatalf("SHA256SUMS missing %q:\n%s", want, sums)
	}
}

// Render the winget and Scoop manifests of a release and check them against
// the files the release holds, and against mygo.json, which names the
// installer, its folder and its key in Settings > Apps.
func TestWindowsPackageManifests(t *testing.T) {
	if runtime.GOOS == "windows" {
		// bash may be WSL's there, and a checkout with CRLF line endings stops it.
		t.Skip("the render scripts run in bash, on the release runners")
	}
	var app struct{ Name, Identifier, Version string }
	data, err := os.ReadFile("mygo.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &app); err != nil {
		t.Fatal(err)
	}
	workflow := readReleaseWorkflow(t)
	for job, variable := range map[string]string{"winget": "WINGET_ENABLED", "scoop": "SCOOP_ENABLED"} {
		if want := "vars." + variable + " == 'true'"; workflow.Jobs[job].If != want {
			t.Errorf("release job %s runs if %q, not %q", job, workflow.Jobs[job].If, want)
		}
	}
	uploads := desktopUploads(t, workflow)
	released := func(t *testing.T, url string) {
		t.Helper()
		for _, pattern := range uploads {
			if ok, _ := filepath.Match(filepath.Base(pattern), path.Base(url)); ok {
				return
			}
		}
		t.Errorf("%s: the desktop jobs upload no file of that name", url)
	}
	download := "https://github.com/21stware/bowel/releases/download/v" + app.Version + "/"
	sums := map[string]string{"amd64": strings.Repeat("0a", 32), "arm64": strings.Repeat("b1", 32)}

	t.Run("winget", func(t *testing.T) {
		out := t.TempDir()
		cmd := exec.Command("bash", "scripts/render-winget.sh", app.Version, sums["amd64"], sums["arm64"], out, "2026-01-02")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("render-winget: %v\n%s", err, output)
		}
		type manifest struct {
			PackageIdentifier string `yaml:"PackageIdentifier"`
			PackageVersion    string `yaml:"PackageVersion"`
			PackageLocale     string `yaml:"PackageLocale"`
			DefaultLocale     string `yaml:"DefaultLocale"`
			Copyright         string `yaml:"Copyright"`
			InstallerType     string `yaml:"InstallerType"`
			Scope             string `yaml:"Scope"`
			InstallerSwitches struct {
				Silent             string `yaml:"Silent"`
				SilentWithProgress string `yaml:"SilentWithProgress"`
			} `yaml:"InstallerSwitches"`
			ProductCode            string `yaml:"ProductCode"`
			ReleaseDate            string `yaml:"ReleaseDate"`
			AppsAndFeaturesEntries []struct {
				DisplayName string `yaml:"DisplayName"`
				ProductCode string `yaml:"ProductCode"`
			} `yaml:"AppsAndFeaturesEntries"`
			InstallationMetadata struct {
				DefaultInstallLocation string `yaml:"DefaultInstallLocation"`
			} `yaml:"InstallationMetadata"`
			Installers []struct {
				Architecture    string `yaml:"Architecture"`
				InstallerURL    string `yaml:"InstallerUrl"`
				InstallerSha256 string `yaml:"InstallerSha256"`
			} `yaml:"Installers"`
			ManifestType    string `yaml:"ManifestType"`
			ManifestVersion string `yaml:"ManifestVersion"`
		}
		entries, err := os.ReadDir(out)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		want := []string{"21stware.bowel.installer.yaml", "21stware.bowel.locale.en-US.yaml", "21stware.bowel.locale.zh-CN.yaml", "21stware.bowel.yaml"}
		if !slices.Equal(names, want) {
			t.Fatalf("render-winget wrote %q, not %q", names, want)
		}
		manifests := map[string]manifest{}
		for _, name := range names {
			data, err := os.ReadFile(filepath.Join(out, name))
			if err != nil {
				t.Fatal(err)
			}
			var m manifest
			if err := yaml.Unmarshal(data, &m); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if m.PackageIdentifier != "21stware.bowel" || m.PackageVersion != app.Version || m.ManifestVersion == "" {
				t.Errorf("%s is of %s %s, manifest version %q", name, m.PackageIdentifier, m.PackageVersion, m.ManifestVersion)
			}
			manifests[name] = m
		}
		version := manifests["21stware.bowel.yaml"]
		for name, m := range manifests {
			if m.ManifestVersion != version.ManifestVersion {
				t.Errorf("%s has manifest version %s, the version manifest %s", name, m.ManifestVersion, version.ManifestVersion)
			}
		}
		if version.ManifestType != "version" {
			t.Errorf("21stware.bowel.yaml is a manifest of type %q", version.ManifestType)
		}
		locale, ok := manifests["21stware.bowel.locale."+version.DefaultLocale+".yaml"]
		if !ok || locale.ManifestType != "defaultLocale" || locale.PackageLocale != version.DefaultLocale {
			t.Errorf("the default locale %q has no defaultLocale manifest", version.DefaultLocale)
		}
		license, err := os.ReadFile("LICENSE")
		if err != nil {
			t.Fatal(err)
		}
		notice := strings.TrimSpace(strings.TrimPrefix(strings.SplitN(string(license), "\n", 2)[0], "Required Notice: "))
		if locale.Copyright != notice {
			t.Errorf("copyright %q is not LICENSE's notice %q", locale.Copyright, notice)
		}

		installer := manifests["21stware.bowel.installer.yaml"]
		if installer.ManifestType != "installer" || installer.InstallerType != "nullsoft" || installer.Scope != "user" {
			t.Errorf("installer manifest of type %q, for a %q installer in scope %q", installer.ManifestType, installer.InstallerType, installer.Scope)
		}
		if s := installer.InstallerSwitches; s.Silent != "/S" || s.SilentWithProgress != "/S" {
			t.Errorf("silent switches %+v, not NSIS's /S", s)
		}
		if installer.ProductCode != app.Identifier || len(installer.AppsAndFeaturesEntries) != 1 ||
			installer.AppsAndFeaturesEntries[0].ProductCode != app.Identifier || installer.AppsAndFeaturesEntries[0].DisplayName != app.Name {
			t.Errorf("product code %q and entries %+v, not the installer's key %s named %s", installer.ProductCode, installer.AppsAndFeaturesEntries, app.Identifier, app.Name)
		}
		if want := `%LOCALAPPDATA%\Programs\` + app.Name; installer.InstallationMetadata.DefaultInstallLocation != want {
			t.Errorf("default install location %q, not %q", installer.InstallationMetadata.DefaultInstallLocation, want)
		}
		if installer.ReleaseDate != "2026-01-02" {
			t.Errorf("release date %q, not the one given", installer.ReleaseDate)
		}
		arches := map[string]string{"x64": "amd64", "arm64": "arm64"}
		if len(installer.Installers) != len(arches) {
			t.Fatalf("%d installers, not one for each of %v", len(installer.Installers), arches)
		}
		for _, i := range installer.Installers {
			goarch, ok := arches[i.Architecture]
			if !ok {
				t.Errorf("installer for architecture %q", i.Architecture)
				continue
			}
			if want := download + app.Name + "-setup-" + app.Version + "-" + goarch + ".exe"; i.InstallerURL != want {
				t.Errorf("%s installer at %s, not %s", i.Architecture, i.InstallerURL, want)
			}
			if want := strings.ToUpper(sums[goarch]); i.InstallerSha256 != want {
				t.Errorf("%s installer has SHA-256 %s, not %s", i.Architecture, i.InstallerSha256, want)
			}
			released(t, i.InstallerURL)
		}
	})

	t.Run("scoop", func(t *testing.T) {
		cmd := exec.Command("bash", "scripts/render-scoop.sh", app.Version, sums["amd64"], sums["arm64"])
		cmd.Stderr = os.Stderr
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("render-scoop: %v", err)
		}
		var m struct {
			Version      string
			Homepage     string
			License      struct{ Identifier, URL string }
			Architecture map[string]struct{ URL, Hash string }
			Shortcuts    [][]string
			Checkver     struct{ Github string }
			Autoupdate   struct {
				Architecture map[string]struct{ URL string }
				Hash         struct{ URL string }
			}
		}
		if err := json.Unmarshal(output, &m); err != nil {
			t.Fatalf("render-scoop wrote no JSON: %v\n%s", err, output)
		}
		if m.Version != app.Version || m.Homepage == "" || m.License.Identifier == "" {
			t.Errorf("version %q, homepage %q, license %q", m.Version, m.Homepage, m.License.Identifier)
		}
		arches := map[string]string{"64bit": "amd64", "arm64": "arm64"}
		if len(m.Architecture) != len(arches) || len(m.Autoupdate.Architecture) != len(arches) {
			t.Fatalf("architectures %v, autoupdate %v, not %v", m.Architecture, m.Autoupdate.Architecture, arches)
		}
		for arch, goarch := range arches {
			a := m.Architecture[arch]
			if want := download + app.Name + "-" + app.Version + "-windows-" + goarch + ".tar.gz"; a.URL != want {
				t.Errorf("%s archive at %s, not %s", arch, a.URL, want)
			}
			if a.Hash != sums[goarch] {
				t.Errorf("%s archive has hash %s, not %s", arch, a.Hash, sums[goarch])
			}
			released(t, a.URL)
			if next := strings.ReplaceAll(m.Autoupdate.Architecture[arch].URL, "$version", app.Version); next != a.URL {
				t.Errorf("%s autoupdate URL makes %s for this version, not %s", arch, next, a.URL)
			}
		}
		if m.Autoupdate.Hash.URL != "$baseurl/SHA256SUMS" {
			t.Errorf("autoupdate finds hashes in %s, not the release's SHA256SUMS", m.Autoupdate.Hash.URL)
		}
		if want := [][]string{{app.Name + ".exe", app.Name}}; !slices.EqualFunc(m.Shortcuts, want, slices.Equal) {
			t.Errorf("shortcuts %q, not %q", m.Shortcuts, want)
		}
		if m.Checkver.Github != "https://github.com/21stware/bowel" {
			t.Errorf("checkver looks at %q", m.Checkver.Github)
		}
	})
}
