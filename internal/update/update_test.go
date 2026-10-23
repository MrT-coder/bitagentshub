package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNewer_ComparesReleaseVersions(t *testing.T) {
	for _, c := range []struct {
		current, tag string
		want         bool
	}{
		{"v0.3.0", "v0.4.0", true},
		{"v0.3.0", "v0.3.1", true},
		{"v0.9.0", "v0.10.0", true},
		{"v0.3.0", "v1.0.0", true},
		{"v0.4.0", "v0.4.0", false},
		{"v0.4.0", "v0.3.9", false},
		{"v0.4.0-rc.1", "v0.4.0", false},
		{"dev", "v0.4.0", false},
		{"v0.3.0", "", false},
		{"v0.3.0", "latest", false},
	} {
		if got := Newer(c.current, c.tag); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.current, c.tag, got, c.want)
		}
	}
}

func TestCheck_AsksAtMostOnceADayAndRemembersTheAnswer(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "sub", "update.json")
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	asks := 0
	latest := func(tag string, err error) func() (string, error) {
		return func() (string, error) { asks++; return tag, err }
	}

	if got := Check(cache, "v0.3.0", now, latest("v0.4.0", nil)); got != "v0.4.0" {
		t.Errorf("first check = %q, want v0.4.0", got)
	}
	if got := Check(cache, "v0.3.0", now.Add(23*time.Hour), latest("v0.5.0", nil)); got != "v0.4.0" || asks != 1 {
		t.Errorf("within a day: got %q after %d asks, want the remembered v0.4.0 after 1", got, asks)
	}
	if got := Check(cache, "v0.4.0", now.Add(time.Hour), latest("v0.5.0", nil)); got != "" {
		t.Errorf("after upgrading to the remembered tag: got %q, want nothing", got)
	}
	if got := Check(cache, "v0.3.0", now.Add(25*time.Hour), latest("v0.5.0", nil)); got != "v0.5.0" || asks != 2 {
		t.Errorf("a day later: got %q after %d asks, want v0.5.0 after 2", got, asks)
	}
}

func TestCheck_IsQuietOfflineAndForBuildsThatAreNotReleases(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "update.json")
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	asks := 0
	offline := func() (string, error) { asks++; return "", errors.New("no network") }

	if got := Check(cache, "v0.3.0", now, offline); got != "" {
		t.Errorf("offline = %q, want nothing", got)
	}
	Check(cache, "v0.3.0", now.Add(time.Hour), offline)
	if asks != 1 {
		t.Errorf("offline asked %d times within a day, want 1", asks)
	}
	if got := Check(filepath.Join(t.TempDir(), "x.json"), "dev", now, func() (string, error) { asks++; return "v9.0.0", nil }); got != "" || asks != 1 {
		t.Errorf("dev build: got %q, asks %d; want nothing and no ask", got, asks)
	}
}

// fakeGitHub serves one release whose archive for goos/goarch holds bin.
type fakeGitHub struct {
	tag      string
	archive  []byte
	name     string
	sums     string
	requests []string
}

func (f *fakeGitHub) server(t *testing.T) Client {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests = append(f.requests, r.URL.Path+" "+r.Header.Get("Accept")+" "+r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/repos/MrT-coder/bitagentshub/releases/latest":
			fmt.Fprintf(w, `{"tag_name":%q,"assets":[{"name":%q,"url":"%s/assets/1"},{"name":"checksums.txt","url":"%s/assets/2"}]}`, f.tag, f.name, srv.URL, srv.URL)
		case "/assets/1":
			w.Write(f.archive)
		case "/assets/2":
			fmt.Fprint(w, f.sums)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return Client{HTTP: srv.Client(), API: srv.URL, Token: "tok"}
}

func release(t *testing.T, goos, file string, bin []byte) *fakeGitHub {
	t.Helper()
	var buf bytes.Buffer
	name := "bah_0.4.0_" + goos + "_amd64"
	if goos == "windows" {
		name += ".zip"
		zw := zip.NewWriter(&buf)
		for _, n := range []string{"README.md", file} {
			w, _ := zw.Create(n)
			if n == file {
				w.Write(bin)
			} else {
				w.Write([]byte("readme"))
			}
		}
		zw.Close()
	} else {
		name += ".tar.gz"
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		tw.WriteHeader(&tar.Header{Name: file, Mode: 0o755, Size: int64(len(bin)), Typeflag: tar.TypeReg})
		tw.Write(bin)
		tw.Close()
		gz.Close()
	}
	sum := sha256.Sum256(buf.Bytes())
	return &fakeGitHub{tag: "v0.4.0", archive: buf.Bytes(), name: name,
		sums: "0000000000000000000000000000000000000000000000000000000000000000  bah_0.4.0_other.zip\n" + hex.EncodeToString(sum[:]) + "  " + name + "\n"}
}

func TestUpgrade_ReplacesTheBinaryWithTheCheckedRelease(t *testing.T) {
	for _, goos := range []string{"windows", "linux"} {
		t.Run(goos, func(t *testing.T) {
			file := "bah"
			if goos == "windows" {
				file = "bah.exe"
			}
			gh := release(t, goos, file, []byte("new binary"))
			c := gh.server(t)
			exe := filepath.Join(t.TempDir(), file)
			os.WriteFile(exe, []byte("old binary"), 0o755)
			os.WriteFile(exe+".1.old", []byte("from an earlier upgrade"), 0o755)
			os.WriteFile(filepath.Join(filepath.Dir(exe), "bah-notes.old"), []byte("not ours"), 0o644)

			tag, err := c.Upgrade(context.Background(), "v0.3.0", exe, goos, "amd64")
			if err != nil || tag != "v0.4.0" {
				t.Fatalf("Upgrade = %q, %v; want v0.4.0", tag, err)
			}
			if b, _ := os.ReadFile(exe); string(b) != "new binary" {
				t.Errorf("binary = %q, want the release's", b)
			}
			if b, _ := os.ReadFile(exe + ".old"); string(b) != "old binary" {
				t.Errorf("%s.old = %q, want the binary that was running", file, b)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "bah-notes.old")); err != nil {
				t.Errorf("a file that is not an old binary was removed")
			}
			if _, err := os.Stat(exe + ".1.old"); !os.IsNotExist(err) {
				t.Errorf("an earlier upgrade's old binary is still there")
			}
			if _, err := os.Stat(exe + ".new"); !os.IsNotExist(err) {
				t.Errorf("the download was left at %s.new", file)
			}
			if fi, _ := os.Stat(exe); runtime.GOOS != "windows" && fi.Mode().Perm()&0o100 == 0 {
				t.Errorf("mode = %v, want executable", fi.Mode())
			}
			want := []string{
				"/repos/MrT-coder/bitagentshub/releases/latest application/vnd.github+json Bearer tok",
				"/assets/1 application/octet-stream Bearer tok",
				"/assets/2 application/octet-stream Bearer tok",
			}
			if strings.Join(gh.requests, "\n") != strings.Join(want, "\n") {
				t.Errorf("requests =\n%s\nwant\n%s", strings.Join(gh.requests, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

func TestUpgrade_LeavesTheBinaryAloneWhenTheDownloadIsNotTheReleasedOne(t *testing.T) {
	for name, spoil := range map[string]func(*fakeGitHub){
		"checksum mismatch":      func(f *fakeGitHub) { f.archive = append(f.archive, 'x') },
		"missing from checksums": func(f *fakeGitHub) { f.sums = "abc  something-else.zip\n" },
		"no binary inside":       func(f *fakeGitHub) { *f = *release(t, "windows", "other.exe", []byte("x")) },
		"no archive for os":      func(f *fakeGitHub) { f.name = "bah_0.4.0_linux_amd64.tar.gz" },
	} {
		t.Run(name, func(t *testing.T) {
			gh := release(t, "windows", "bah.exe", []byte("new binary"))
			spoil(gh)
			c := gh.server(t)
			exe := filepath.Join(t.TempDir(), "bah.exe")
			os.WriteFile(exe, []byte("old binary"), 0o755)

			if _, err := c.Upgrade(context.Background(), "v0.3.0", exe, "windows", "amd64"); err == nil {
				t.Fatal("Upgrade succeeded, want an error")
			}
			if b, _ := os.ReadFile(exe); string(b) != "old binary" {
				t.Errorf("binary = %q, want it untouched", b)
			}
			if left, _ := filepath.Glob(exe + ".*"); left != nil {
				t.Errorf("left behind: %v", left)
			}
		})
	}
}

func TestUpgrade_SaysWhenThereIsNothingNewer(t *testing.T) {
	gh := release(t, "windows", "bah.exe", []byte("new binary"))
	c := gh.server(t)
	exe := filepath.Join(t.TempDir(), "bah.exe")
	os.WriteFile(exe, []byte("old binary"), 0o755)

	if tag, err := c.Upgrade(context.Background(), "v0.4.0", exe, "windows", "amd64"); !errors.Is(err, ErrUpToDate) || tag != "v0.4.0" {
		t.Errorf("same version: %q, %v; want v0.4.0, ErrUpToDate", tag, err)
	}
	if _, err := c.Upgrade(context.Background(), "dev", exe, "windows", "amd64"); err == nil || !strings.Contains(err.Error(), "not a release") {
		t.Errorf("dev build: %v, want it to say it is not a release", err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "old binary" {
		t.Errorf("binary = %q, want it untouched", b)
	}
	if len(gh.requests) != 2 {
		t.Errorf("requests = %v, want only the two release lookups", gh.requests)
	}
}

func TestLatest_SaysWhenGitHubRateLimits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	_, err := Client{HTTP: srv.Client(), API: srv.URL}.Latest(context.Background())
	if err == nil || !strings.Contains(err.Error(), "set GITHUB_TOKEN") {
		t.Errorf("err = %v, want a hint to set GITHUB_TOKEN", err)
	}
}
