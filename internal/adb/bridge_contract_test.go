package adb

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The Go side and the Kotlin accessibility bridge agree on a handful of strings
// by hand: the package, the service class, the broadcast action, the log tag and
// priority, the broadcast extras and the JSON keys of the result line. Renaming
// one side breaks tap_on_text / tap_element(via_accessibility) only on a device,
// so this test reads the Kotlin, manifest and gradle sources and compares.

func bridgeFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "bridge", rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func firstGroup(t *testing.T, re, src string) string {
	t.Helper()
	m := regexp.MustCompile(re).FindStringSubmatch(src)
	if m == nil {
		t.Fatalf("pattern %q found nothing: the bridge source changed shape, update this test with it", re)
	}
	return m[1]
}

// The bridge's versionCode/versionName come from the repo-root VERSION file, so
// an installed bridge can be told stale from current. A hardcoded literal
// would bring back the "stuck at 1.0" problem silently.
var hardcodedVersionRe = regexp.MustCompile(`version(?:Code\s*=\s*\d|Name\s*=\s*")`)

func bridgeHardcodesVersion(gradle string) bool { return hardcodedVersionRe.MatchString(gradle) }

func TestBridgeGradleDerivesVersionFromVERSIONFile(t *testing.T) {
	gradle := bridgeFile(t, "app/build.gradle.kts")
	if bridgeHardcodesVersion(gradle) {
		t.Error("bridge/app/build.gradle.kts hardcodes versionCode/versionName; derive them from the repo-root VERSION file")
	}
	if !strings.Contains(gradle, "VERSION") {
		t.Error("bridge/app/build.gradle.kts does not read the VERSION file")
	}
}

func TestBridgeHardcodesVersionDetector(t *testing.T) {
	for src, want := range map[string]bool{
		`versionCode = 1`:                 true,
		`versionName = "1.0"`:             true,
		`versionCode=42`:                  true,
		`versionCode = bridgeVersionCode`: false,
		`versionName = bridgeVersionName`: false,
	} {
		if got := bridgeHardcodesVersion(src); got != want {
			t.Errorf("bridgeHardcodesVersion(%q) = %v, want %v", src, got, want)
		}
	}
}

const kotlinService = "app/src/main/kotlin/com/iksnerd/adbmcp/bridge/BridgeAccessibilityService.kt"

func TestBridgeConstantsMatchTheKotlinBridge(t *testing.T) {
	kt := bridgeFile(t, kotlinService)
	manifest := bridgeFile(t, "app/src/main/AndroidManifest.xml")
	gradle := bridgeFile(t, "app/build.gradle.kts")

	if got := firstGroup(t, `(?m)^package\s+(\S+)`, kt); got != BridgePackage {
		t.Errorf("Kotlin package = %q, Go BridgePackage = %q", got, BridgePackage)
	}
	if got := firstGroup(t, `namespace\s*=\s*"([^"]+)"`, gradle); got != BridgePackage {
		t.Errorf("gradle namespace = %q, Go BridgePackage = %q", got, BridgePackage)
	}
	if got := firstGroup(t, `applicationId\s*=\s*"([^"]+)"`, gradle); got != BridgePackage {
		t.Errorf("gradle applicationId = %q, Go BridgePackage = %q", got, BridgePackage)
	}

	// Component: <package>/.<class>, with the class declared in Kotlin and the
	// manifest naming it relative to the package.
	class := firstGroup(t, `(?m)^class\s+(\w+)\s*:\s*AccessibilityService\(\)`, kt)
	if want := BridgePackage + "/." + class; BridgeServiceComponent != want {
		t.Errorf("Go BridgeServiceComponent = %q, Kotlin class gives %q", BridgeServiceComponent, want)
	}
	if got := firstGroup(t, `<service[^>]*android:name="([^"]+)"`, manifest); got != "."+class {
		t.Errorf("manifest service name = %q, Kotlin class is %q", got, "."+class)
	}

	if got := firstGroup(t, `const val ACTION_CLICK\s*=\s*"([^"]+)"`, kt); got != BridgeClickAction {
		t.Errorf("Kotlin ACTION_CLICK = %q, Go BridgeClickAction = %q", got, BridgeClickAction)
	}
	if got := firstGroup(t, `const val TAG\s*=\s*"([^"]+)"`, kt); got != bridgeLogTag {
		t.Errorf("Kotlin TAG = %q, Go bridgeLogTag = %q", got, bridgeLogTag)
	}

	// Priority: Go reads `logcat -s <tag>:I`, so every line the bridge reports a
	// result on must be Log.i.
	goSrc, err := os.ReadFile("bridge.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(goSrc), `bridgeLogTag+":I"`) {
		t.Error(`bridge.go no longer filters logcat at priority I (bridgeLogTag+":I"); update this test and the Kotlin Log calls together`)
	}
	for _, m := range regexp.MustCompile(`Log\.(\w)\(TAG, result\.toString\(\)\)`).FindAllStringSubmatch(kt, -1) {
		if m[1] != "i" {
			t.Errorf("Kotlin reports a result with Log.%s; Go reads priority I only", m[1])
		}
	}
	if !regexp.MustCompile(`Log\.i\(TAG, result\.toString\(\)\)`).MatchString(kt) {
		t.Error("Kotlin no longer logs the result JSON with Log.i(TAG, result.toString())")
	}
}

// Every key BridgeClickResult decodes must be one the bridge actually writes. The
// bridge also writes "resource_id" and "text" on a no_match line; Go does not
// decode them (the caller already knows what it asked for), so they are the only
// keys allowed to be unmapped.
func TestBridgeClickResultKeysMatchTheKotlinPuts(t *testing.T) {
	kt := bridgeFile(t, kotlinService)

	kotlinKeys := map[string]bool{}
	for _, m := range regexp.MustCompile(`\.put\("([a-z_]+)"`).FindAllStringSubmatch(kt, -1) {
		kotlinKeys[m[1]] = true
	}

	var goKeys []string
	rt := reflect.TypeOf(BridgeClickResult{})
	for i := range rt.NumField() {
		name, _, _ := strings.Cut(rt.Field(i).Tag.Get("json"), ",")
		goKeys = append(goKeys, name)
	}
	if len(goKeys) == 0 {
		t.Fatal("BridgeClickResult has no JSON fields")
	}
	goSet := map[string]bool{}
	for _, k := range goKeys {
		goSet[k] = true
		if !kotlinKeys[k] {
			t.Errorf("BridgeClickResult decodes %q, which the Kotlin bridge never writes", k)
		}
	}

	allowedUnmapped := map[string]bool{"resource_id": true, "text": true}
	var extra []string
	for k := range kotlinKeys {
		if !goSet[k] && !allowedUnmapped[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		t.Errorf("the Kotlin bridge writes %v, which BridgeClickResult does not decode; add the field or list the key as deliberately unmapped here", extra)
	}
}

// The broadcast extras Go sends must be the ones Kotlin reads.
func TestBridgeBroadcastExtrasMatchTheKotlinReads(t *testing.T) {
	kt := bridgeFile(t, kotlinService)
	goSrc, err := os.ReadFile("bridge.go")
	if err != nil {
		t.Fatal(err)
	}
	sent := map[string]bool{}
	for _, m := range regexp.MustCompile(`"(?:-e|--es|--ez)", "([a-z_]+)"`).FindAllStringSubmatch(string(goSrc), -1) {
		sent[m[1]] = true
	}
	read := map[string]bool{}
	for _, m := range regexp.MustCompile(`intent\.get\w+Extra\("([a-z_]+)"`).FindAllStringSubmatch(kt, -1) {
		read[m[1]] = true
	}
	if !reflect.DeepEqual(sent, read) {
		t.Errorf("Go sends extras %v, Kotlin reads %v", sent, read)
	}
}
