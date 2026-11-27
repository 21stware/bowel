package main

import (
	"encoding/xml"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// Docker Hub shows docs/docker.md with none of the files beside it, so every
// link in the overview that .github/workflows/docker.yml writes must be
// absolute, and it must fit Docker Hub's 25,000 bytes.
func TestDockerHubOverview(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("needs bash")
	}
	const base = "https://github.com/21stware/bowel/blob/main/docs/"
	out, err := exec.Command("bash", "scripts/render-dockerhub-readme.sh", "docs/docker.md", base).Output()
	if err != nil {
		t.Fatal(err)
	}
	overview := string(out)
	if !strings.HasPrefix(overview, "# bowel\n") {
		t.Errorf("overview starts with %q", strings.SplitN(overview, "\n", 2)[0])
	}
	if strings.Contains(overview, "\n# Docker\n") {
		t.Error("overview keeps the doc's own title")
	}
	if len(out) > 25000 {
		t.Errorf("overview is %d bytes; Docker Hub keeps 25,000", len(out))
	}
	links := regexp.MustCompile(`\]\(([^)]*)\)`).FindAllStringSubmatch(overview, -1)
	if len(links) == 0 {
		t.Fatal("overview has no links")
	}
	for _, m := range links {
		if !strings.HasPrefix(m[1], "https://") {
			t.Errorf("relative link left in the overview: %s", m[1])
		}
	}
	for _, want := range []string{base + "cli.md", base + "docker/compose.yaml", base + "docker.md#agents-mcp"} {
		if !strings.Contains(overview, "]("+want+")") {
			t.Errorf("overview has no link to %s", want)
		}
	}
}

// The Unraid template names what the image has: its ports and volume, and
// variables that bowel reads.
func TestUnraidTemplate(t *testing.T) {
	data, err := os.ReadFile("docs/docker/unraid.xml")
	if err != nil {
		t.Fatal(err)
	}
	var tmpl struct {
		Version     string `xml:"version,attr"`
		Name        string
		Repository  string
		TemplateURL string
		Configs     []struct {
			Target  string `xml:",attr"`
			Default string `xml:",attr"`
			Type    string `xml:",attr"`
			Value   string `xml:",chardata"`
		} `xml:"Config"`
	}
	if err := xml.Unmarshal(data, &tmpl); err != nil {
		t.Fatal(err)
	}
	if tmpl.Version != "2" || tmpl.Name != "bowel" || tmpl.Repository != "ghcr.io/21stware/bowel" {
		t.Errorf("template: version %q, name %q, repository %q", tmpl.Version, tmpl.Name, tmpl.Repository)
	}
	if tmpl.TemplateURL != "https://raw.githubusercontent.com/21stware/bowel/main/docs/docker/unraid.xml" {
		t.Errorf("TemplateURL %q is not where the template is", tmpl.TemplateURL)
	}
	dockerfile, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	container, err := os.ReadFile("app_container.go")
	if err != nil {
		t.Fatal(err)
	}
	expose := regexp.MustCompile(`(?m)^EXPOSE (.*)$`).FindSubmatch(dockerfile)
	volume := regexp.MustCompile(`(?m)^VOLUME \["([^"]*)"\]`).FindSubmatch(dockerfile)
	if expose == nil || volume == nil {
		t.Fatal("Dockerfile has no EXPOSE or VOLUME")
	}
	for _, c := range tmpl.Configs {
		if c.Value != c.Default {
			t.Errorf("%s: value %q is not its default %q", c.Target, c.Value, c.Default)
		}
		switch c.Type {
		case "Port":
			if !strings.Contains(" "+string(expose[1])+" ", " "+c.Target+" ") {
				t.Errorf("port %s is not one the image exposes (%s)", c.Target, expose[1])
			}
		case "Path":
			if c.Target != string(volume[1]) {
				t.Errorf("path %s is not the image's volume %s", c.Target, volume[1])
			}
		case "Variable":
			if !strings.Contains(string(container), `"`+c.Target+`"`) {
				t.Errorf("variable %s is not one app_container.go reads", c.Target)
			}
		default:
			t.Errorf("%s: unexpected type %q", c.Target, c.Type)
		}
	}
}
