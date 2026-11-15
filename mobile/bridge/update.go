package bridge

// Looking for a newer bowel. A phone's copy came as a file from the project's
// releases and nothing updates it, so the app asks now and then whether there
// is a newer release and says where its file is. Downloading and installing
// it stay with the browser and the system.

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
)

// releasesURL is the project's releases. Its /latest redirects to the newest
// release that is neither a draft nor a prerelease, and unlike GitHub's API
// it is not counted against the address asking, which with a shared node is
// soon used up.
var releasesURL = "https://github.com/21stware/bowel/releases"

// updateTimeout is the most one way out is given to answer.
const updateTimeout = 12 * time.Second

// updateData is a newer version and the link that gets it: the Android file,
// or the release's page when that file is not there.
type updateData struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

// checkUpdate asks for the latest release; nil when current is it, or newer.
func (x *bridge) checkUpdate(ctx context.Context, current string) (*updateData, error) {
	return latestRelease(ctx, &http.Client{}, current)
}

func latestRelease(ctx context.Context, c *http.Client, current string) (*updateData, error) {
	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()
	// The answers wanted are the redirects themselves.
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	to, err := redirect(ctx, c, http.MethodGet, releasesURL+"/latest")
	if err != nil {
		return nil, err
	}
	tag := path.Base(to)
	version := strings.TrimPrefix(tag, "v")
	if !strings.Contains(to, "/releases/tag/") || versionParts(version) == nil {
		return nil, fmt.Errorf("unexpected latest release: %s", to)
	}
	if !newerVersion(version, current) {
		return nil, nil
	}
	up := &updateData{Version: version, URL: releasesURL + "/tag/" + tag}
	apk := releasesURL + "/download/" + tag + "/bowel-" + version + "-android-arm64.apk"
	if _, err := redirect(ctx, c, http.MethodHead, apk); err == nil {
		up.URL = apk
	}
	return up, nil
}

// redirect is where the address sends the request on.
func redirect(ctx context.Context, c *http.Client, method, address string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, method, address, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	to := resp.Header.Get("Location")
	if resp.StatusCode/100 != 3 || to == "" {
		return "", fmt.Errorf("%s: %s", address, resp.Status)
	}
	return to, nil
}

// newerVersion: both are dotted numbers and latest is the greater. Anything
// else, as a development build's name, is not compared.
func newerVersion(latest, current string) bool {
	a, b := versionParts(latest), versionParts(current)
	if a == nil || b == nil {
		return false
	}
	for i := 0; i < len(a) || i < len(b); i++ {
		var m, n int
		if i < len(a) {
			m = a[i]
		}
		if i < len(b) {
			n = b[i]
		}
		if m != n {
			return m > n
		}
	}
	return false
}

func versionParts(v string) []int {
	out := []int{}
	for _, s := range strings.Split(v, ".") {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return nil
		}
		out = append(out, n)
	}
	return out
}
