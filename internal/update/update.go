// Package update finds out whether a newer BitAgentsHub release exists and
// installs it in place of the running binary, the way install.ps1 and
// install.sh do.
package update

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// API is GitHub's REST API; tests point a Client at their own server.
const API = "https://api.github.com"

const repo = "MrT-coder/bitagentshub"

// maxDownload bounds an archive or checksums download.
const maxDownload = 64 << 20

// Client reads releases from API. Token, when set, is sent like the
// installers send GITHUB_TOKEN, which lifts GitHub's 60 requests an hour.
type Client struct {
	HTTP  *http.Client
	API   string
	Token string
}

// NewClient talks to GitHub with GITHUB_TOKEN from the environment, if any.
func NewClient() Client {
	return Client{HTTP: &http.Client{Timeout: 5 * time.Minute}, API: API, Token: os.Getenv("GITHUB_TOKEN")}
}

// Release is a published release: its tag and its files.
type Release struct {
	Tag    string  `json:"tag_name"`
	Assets []Asset `json:"assets"`
}

// Asset is one file of a release. URL is its API address, which serves the
// file itself when asked for application/octet-stream.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Latest returns the newest published release.
func (c Client) Latest(ctx context.Context) (Release, error) {
	url := c.API + "/repos/" + repo + "/releases/latest"
	body, err := c.get(ctx, url, "application/vnd.github+json", 1<<20)
	if err != nil {
		return Release{}, err
	}
	var r Release
	if err := json.Unmarshal(body, &r); err != nil || r.Tag == "" {
		return Release{}, fmt.Errorf("cannot read the release at %s", url)
	}
	return r, nil
}

func (c Client) get(ctx context.Context, url, accept string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "bah-upgrade")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		hint := ""
		if res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusTooManyRequests {
			hint = " (rate limited? set GITHUB_TOKEN)"
		}
		return nil, fmt.Errorf("%s: %s%s", url, res.Status, hint)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%s: larger than %d bytes", url, limit)
	}
	return body, nil
}

// Newer reports whether tag is a later release than current. A current
// version that is not a release (a "dev" build) is never behind.
func Newer(current, tag string) bool {
	c, ok := parse(current)
	if !ok {
		return false
	}
	t, ok := parse(tag)
	if !ok {
		return false
	}
	for i := range c {
		if t[i] != c[i] {
			return t[i] > c[i]
		}
	}
	return false
}

// parse reads vMAJOR.MINOR.PATCH, ignoring anything after the patch number.
func parse(v string) ([3]int, bool) {
	var out [3]int
	v, ok := strings.CutPrefix(v, "v")
	if !ok {
		return out, false
	}
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// Every is how often Check asks GitHub.
const Every = 24 * time.Hour

type checked struct {
	At  time.Time `json:"at"`
	Tag string    `json:"tag"`
}

// Check returns the latest release's tag when it is newer than current, or
// "" when it is not or cannot be known. It asks latest at most once every
// Every and remembers the answer in the file cache; a failed ask is
// remembered too, so being offline does not ask on every start.
func Check(cache, current string, now time.Time, latest func() (string, error)) string {
	if _, ok := parse(current); !ok {
		return ""
	}
	var last checked
	if b, err := os.ReadFile(cache); err == nil && json.Unmarshal(b, &last) == nil && now.Sub(last.At) < Every && !last.At.After(now) {
		return newerTag(current, last.Tag)
	}
	tag, err := latest()
	if err != nil {
		tag = ""
	}
	if b, err := json.Marshal(checked{At: now, Tag: tag}); err == nil {
		_ = os.MkdirAll(filepath.Dir(cache), 0o755)
		_ = os.WriteFile(cache, b, 0o644)
	}
	return newerTag(current, tag)
}

func newerTag(current, tag string) string {
	if Newer(current, tag) {
		return tag
	}
	return ""
}

// ErrUpToDate is Upgrade's answer when current is already the latest release.
var ErrUpToDate = errors.New("already the latest release")

// Upgrade replaces the binary at exe with the latest release for goos and
// goarch, checked against the release's checksums.txt, and returns its tag.
// The binary in use is moved aside to exe.old, since Windows cannot
// overwrite a running program; a failure puts it back.
func (c Client) Upgrade(ctx context.Context, current, exe, goos, goarch string) (string, error) {
	r, err := c.Latest(ctx)
	if err != nil {
		return "", err
	}
	if !Newer(current, r.Tag) {
		if _, ok := parse(current); !ok {
			return "", fmt.Errorf("this build is %s, not a release; install one with the installer", current)
		}
		return r.Tag, ErrUpToDate
	}
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	suffix := "_" + goos + "_" + goarch + ext
	var archive, sums Asset
	for _, a := range r.Assets {
		switch {
		case strings.HasPrefix(a.Name, "bah_") && strings.HasSuffix(a.Name, suffix):
			archive = a
		case a.Name == "checksums.txt":
			sums = a
		}
	}
	if archive.URL == "" || sums.URL == "" {
		return "", fmt.Errorf("release %s has no %s archive and checksums.txt", r.Tag, strings.TrimPrefix(suffix, "_"))
	}
	data, err := c.get(ctx, archive.URL, "application/octet-stream", maxDownload)
	if err != nil {
		return "", err
	}
	list, err := c.get(ctx, sums.URL, "application/octet-stream", 1<<20)
	if err != nil {
		return "", err
	}
	if err := verify(data, list, archive.Name); err != nil {
		return "", err
	}
	name := "bah"
	if goos == "windows" {
		name = "bah.exe"
	}
	bin, err := extract(data, ext, name)
	if err != nil {
		return "", fmt.Errorf("%s: %w", archive.Name, err)
	}
	if err := replace(exe, bin); err != nil {
		return "", err
	}
	return r.Tag, nil
}

// verify checks data against its line in checksums.txt.
func verify(data, list []byte, name string) error {
	sc := bufio.NewScanner(bytes.NewReader(list))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && f[1] == name {
			sum := sha256.Sum256(data)
			if !strings.EqualFold(f[0], hex.EncodeToString(sum[:])) {
				return fmt.Errorf("checksum mismatch for %s", name)
			}
			return nil
		}
	}
	return fmt.Errorf("%s is missing from checksums.txt", name)
}

// extract returns the file called name at the top of the archive.
func extract(data []byte, ext, name string) ([]byte, error) {
	if ext == ".zip" {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if f.Name == name {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return readAll(rc)
			}
		}
		return nil, fmt.Errorf("no %s inside", name)
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("no %s inside", name)
		}
		if err != nil {
			return nil, err
		}
		if h.Name == name && h.Typeflag == tar.TypeReg {
			return readAll(tr)
		}
	}
}

func readAll(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxDownload+1))
	if err == nil && len(b) > maxDownload {
		err = fmt.Errorf("larger than %d bytes", maxDownload)
	}
	return b, err
}

// replace writes bin in place of exe. The new binary is written next to exe
// first, so a full disk or a missing permission leaves exe untouched.
func replace(exe string, bin []byte) error {
	mode := os.FileMode(0o755)
	if fi, err := os.Stat(exe); err == nil {
		mode = fi.Mode().Perm() | 0o100
	}
	next := exe + ".new"
	if err := os.WriteFile(next, bin, mode); err != nil {
		return fmt.Errorf("cannot write next to %s: %w", exe, err)
	}
	// Old binaries left by earlier upgrades go once this one is aside; one
	// still running cannot be removed and stays.
	earlier, _ := filepath.Glob(exe + ".*.old")
	earlier = append(earlier, exe+".old")
	old := exe + ".old"
	if _, err := os.Stat(old); err == nil {
		old = fmt.Sprintf("%s.%d.old", exe, time.Now().UnixNano())
	}
	if err := os.Rename(exe, old); err != nil {
		os.Remove(next)
		return fmt.Errorf("cannot move %s aside: %w", exe, err)
	}
	if err := os.Rename(next, exe); err != nil {
		if back := os.Rename(old, exe); back != nil {
			return fmt.Errorf("cannot put the new binary at %s (%v), and the old one is at %s", exe, err, old)
		}
		os.Remove(next)
		return fmt.Errorf("cannot put the new binary at %s: %w", exe, err)
	}
	for _, o := range earlier {
		if o != old {
			_ = os.Remove(o)
		}
	}
	return nil
}
