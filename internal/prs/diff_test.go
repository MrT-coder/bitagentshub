package prs

import (
  "context"
  "errors"
  "reflect"
  "strings"
  "testing"
)

const sampleDiff = `diff --git a/internal/app.go b/internal/app.go
index 1111111..2222222 100644
--- a/internal/app.go
+++ b/internal/app.go
@@ -10,3 +10,4 @@ func Run() {
   a := 1
-  b := 2
+  b := 3
+  c := 4
   return
@@ -40 +41 @@ func Stop() {
-  old()
+  fresh()
\ No newline at end of file
diff --git a/docs/new.md b/docs/new.md
new file mode 100644
index 0000000..3333333
--- /dev/null
+++ b/docs/new.md
@@ -0,0 +1,2 @@
+# New
+text
diff --git a/old.txt b/old.txt
deleted file mode 100644
index 4444444..0000000
--- a/old.txt
+++ /dev/null
@@ -1,2 +0,0 @@
-gone
-too
diff --git a/a.txt b/b.txt
similarity index 100%
rename from a.txt
rename to b.txt
diff --git a/logo.png b/logo.png
index 5555555..6666666 100644
Binary files a/logo.png and b/logo.png differ
`

func TestParseDiffReadsFilesHunksAndLineNumbers(t *testing.T) {
  files := ParseDiff(sampleDiff)
  var paths []string
  for _, f := range files {
    paths = append(paths, f.Path)
  }
  if want := []string{"internal/app.go", "docs/new.md", "old.txt", "b.txt", "logo.png"}; !reflect.DeepEqual(paths, want) {
    t.Fatalf("paths = %q, want %q", paths, want)
  }

  app := files[0]
  if app.Added != 3 || app.Removed != 2 || len(app.Hunks) != 2 {
    t.Fatalf("app.go: +%d -%d in %d hunks, want +3 -2 in 2", app.Added, app.Removed, len(app.Hunks))
  }
  if got := app.Hunks[0].Header; got != "@@ -10,3 +10,4 @@ func Run() {" {
    t.Errorf("hunk header = %q", got)
  }
  want := []DiffLine{
    {Context, 10, 10, "  a := 1"},
    {Removed, 11, 0, "  b := 2"},
    {Added, 0, 11, "  b := 3"},
    {Added, 0, 12, "  c := 4"},
    {Context, 12, 13, "  return"},
  }
  if !reflect.DeepEqual(app.Hunks[0].Lines, want) {
    t.Errorf("first hunk lines = %+v\nwant %+v", app.Hunks[0].Lines, want)
  }
  // "@@ -40 +41 @@": a count left out is 1; the "no newline" marker is no line.
  second := app.Hunks[1].Lines
  if len(second) != 2 || second[0] != (DiffLine{Removed, 40, 0, "  old()"}) || second[1] != (DiffLine{Added, 0, 41, "  fresh()"}) {
    t.Errorf("second hunk lines = %+v", second)
  }
}

func TestParseDiffKnowsNewDeletedRenamedAndBinaryFiles(t *testing.T) {
  files := ParseDiff(sampleDiff)
  if f := files[1]; f.Status != StatusAdded || f.Added != 2 || f.Hunks[0].Lines[0].New != 1 {
    t.Errorf("new file = %+v", f)
  }
  if f := files[2]; f.Status != StatusDeleted || f.Removed != 2 {
    t.Errorf("deleted file = %+v", f)
  }
  if f := files[3]; f.Status != StatusRenamed || f.OldPath != "a.txt" || len(f.Hunks) != 0 {
    t.Errorf("renamed file = %+v", f)
  }
  if f := files[4]; !f.Binary || len(f.Hunks) != 0 {
    t.Errorf("binary file = %+v", f)
  }
  if files[0].Status != StatusModified {
    t.Errorf("a changed file is %v, want modified", files[0].Status)
  }
}

func TestParseDiffOfNothingIsNothing(t *testing.T) {
  for _, in := range []string{"", "\n", "not a diff at all"} {
    if files := ParseDiff(in); len(files) != 0 {
      t.Errorf("%q gave %+v", in, files)
    }
  }
}

func TestParseDiffToleratesWindowsLineEnds(t *testing.T) {
  crlf := strings.ReplaceAll(sampleDiff, "\n", "\r\n")
  files := ParseDiff(crlf)
  if len(files) != 5 || files[0].Hunks[0].Lines[0].Text != "  a := 1" {
    t.Errorf("CRLF diff read as %d files, first line %q", len(files), files[0].Hunks[0].Lines[0].Text)
  }
}

func TestDiffAsksGHForThePullRequestWithoutColor(t *testing.T) {
  var got []string
  run := func(_ context.Context, args ...string) ([]byte, error) {
    got = args
    return []byte(sampleDiff), nil
  }
  files, err := Diff(context.Background(), run, "acme/api", 7)
  if err != nil || len(files) != 5 {
    t.Fatalf("Diff = %d files, %v", len(files), err)
  }
  if want := []string{"pr", "diff", "7", "--repo", "acme/api", "--color=never"}; !reflect.DeepEqual(got, want) {
    t.Errorf("gh args = %q, want %q", got, want)
  }
}

func TestDiffReportsAGHFailure(t *testing.T) {
  run := func(context.Context, ...string) ([]byte, error) { return nil, errors.New("boom") }
  if _, err := Diff(context.Background(), run, "acme/api", 7); err == nil || !strings.Contains(err.Error(), "boom") {
    t.Errorf("err = %v, want it to carry the gh failure", err)
  }
}

func TestAContentLineThatLooksLikeAFileHeaderStaysALine(t *testing.T) {
	d := "diff --git a/x.sql b/x.sql\n--- a/x.sql\n+++ b/x.sql\n@@ -1,2 +1,2 @@\n--- a comment\n+++ a better comment\n keep\n"
	files := ParseDiff(d)
	if len(files) != 1 || files[0].Path != "x.sql" || files[0].Removed != 1 || files[0].Added != 1 {
		t.Fatalf("files = %+v", files)
	}
	lines := files[0].Hunks[0].Lines
	if len(lines) != 3 || lines[0].Text != "-- a comment" || lines[1].Text != "++ a better comment" {
		t.Errorf("lines = %+v", lines)
	}
}
