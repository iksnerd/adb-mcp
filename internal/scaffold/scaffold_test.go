package scaffold

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "app")
	files, err := Create(Options{Destination: dir, Name: "Sample App", Package: "com.example.sample"})
	if err != nil || len(files) == 0 {
		t.Fatalf("Create() files=%d err=%v", len(files), err)
	}
	if _, err := os.Stat(filepath.Join(dir, "app/src/main/java/com/example/sample/SampleActivity.kt")); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(Options{Destination: dir, Name: "Other", Package: "com.example.other"}); err == nil {
		t.Fatal("expected non-empty destination to be rejected")
	}
}

// TestCreateRejectsUnsafeName guards the settings.gradle.kts/AndroidManifest.xml
// injection: opts.Name is interpolated unescaped into a Kotlin string literal
// that Gradle executes and an XML attribute, so a stray quote must be rejected
// rather than silently corrupting the generated project.
func TestCreateRejectsUnsafeName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "app")
	_, err := Create(Options{Destination: dir, Name: `Foo" ; System.exit(1) //`, Package: "com.example.sample"})
	if err == nil {
		t.Fatal("expected an unsafe name to be rejected")
	}
}

// TestCreateDigitLeadingNameProducesValidIdentifier guards a Name whose first
// word starts with a digit: the generated Kotlin class name must still start
// with a letter, or the scaffolded project fails to compile.
func TestCreateDigitLeadingNameProducesValidIdentifier(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "app")
	files, err := Create(Options{Destination: dir, Name: "123 App", Package: "com.example.sample"})
	if err != nil {
		t.Fatalf("Create() err=%v", err)
	}
	var found bool
	for _, f := range files {
		base := filepath.Base(f)
		if base == "123Activity.kt" {
			t.Fatalf("generated file %s is not a valid Kotlin identifier", base)
		}
		if filepath.Ext(base) == ".kt" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a generated .kt file")
	}
}

// A package segment that is a Java or Kotlin reserved word writes a project that
// fails to compile; reject it up front.
func TestCreateRejectsReservedWordSegments(t *testing.T) {
	for _, pkg := range []string{"com.example.in", "com.example.class", "com.example.fun", "int.example.app", "com.example.app.new", "com.example.object"} {
		dir := filepath.Join(t.TempDir(), "app")
		if _, err := Create(Options{Destination: dir, Name: "Sample", Package: pkg}); err == nil {
			t.Errorf("package %q uses a reserved word and must be rejected", pkg)
		}
		if _, err := os.Stat(dir); err == nil {
			t.Errorf("package %q was rejected only after writing the project", pkg)
		}
	}
	// A keyword as a substring of a segment is fine.
	if _, err := Create(Options{Destination: filepath.Join(t.TempDir(), "app"), Name: "Sample", Package: "com.example.interface2"}); err != nil {
		t.Errorf("interface2 is not a keyword: %v", err)
	}
}
