package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDiffTypesAndCanonicalPaths(t *testing.T) {
	_, a, client := testFacade(t)
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "deleted.txt"), "deleted baseline\n")
	mustWrite(t, filepath.Join(root, "binary.dat"), "\x00before")
	mustWrite(t, filepath.Join(root, "unicode.txt"), "\xff\xfeA\x00")
	mustWrite(t, filepath.Join(root, "large.txt"), "small")
	mustWrite(t, filepath.Join(root, ".folderwatch.toml"), "max_diff_bytes = '32B'\n")
	req := started(t, a, client, root)
	if err := os.Remove(filepath.Join(root, "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "new 世界 'quote'.txt"), "new text\n")
	mustWrite(t, filepath.Join(root, "binary.dat"), "\x00after")
	mustWrite(t, filepath.Join(root, "unicode.txt"), "\xff\xfeB\x00")
	mustWrite(t, filepath.Join(root, "large.txt"), strings.Repeat("x", 64))
	awaitChanges(t, a, req, 5)
	for key, want := range map[string]string{"deleted.txt": "text", "new 世界 'quote'.txt": "text", "binary.dat": "binary", "unicode.txt": "unsupported-text", "large.txt": "too-large"} {
		d := awaitDiff(t, a, req, key)
		if d.Status != want {
			t.Fatalf("%s: %+v", key, d)
		}
		if want != "text" && len(d.Hunks) != 0 {
			t.Fatal("non-text content rendered")
		}
	}
	for _, key := range []string{"", ".", "..", "../outside", "/etc/passwd", "sub/../../a", "sub/../binary.dat", "sub//file", "./binary.dat", "a\x00b", "C:\\secret", "sub\\file"} {
		requireCode(t, a.GetDiff(DiffRequest{SessionRequest: req, Path: key, Generation: "1", Version: "1"}).Error, "INVALID_PATH")
	}
	requireCode(t, a.GetDiff(DiffRequest{SessionRequest: req, Path: "unknown.txt", Generation: "1", Version: "1"}).Error, "NOT_CHANGED")
	requireCode(t, a.GetDiff(DiffRequest{SessionRequest: req, Path: "binary.dat"}).Error, "INVALID_VERSION")
}

func TestSymlinkEscapesAreNeverRead(t *testing.T) {
	_, a, client := testFacade(t)
	root, outside := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(outside, "secret.txt"), "DO_NOT_DISCLOSE\n")
	req := started(t, a, client, root)
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	awaitChanges(t, a, req, 2)
	requireCode(t, a.GetDiff(DiffRequest{SessionRequest: req, Path: "escape/secret.txt", Generation: "1", Version: "1"}).Error, "INVALID_PATH")
	d := awaitDiff(t, a, req, "link.txt")
	b, _ := json.Marshal(d)
	if d.Status != "unavailable" || strings.Contains(string(b), "DO_NOT_DISCLOSE") {
		t.Fatal(string(b))
	}
}

func TestPaginationAndDTOIsolation(t *testing.T) {
	_, a, client := testFacade(t)
	root := t.TempDir()
	req := started(t, a, client, root)
	for _, name := range []string{"a", "b", "c"} {
		mustWrite(t, filepath.Join(root, name), name)
	}
	awaitChanges(t, a, req, 3)
	page := a.GetChanges(ChangesRequest{SessionRequest: req, Limit: 2})
	if page.Error != nil || len(page.Changes) != 2 || page.Total != 3 || page.NextOffset != 2 {
		t.Fatal(page)
	}
	next := a.GetChanges(ChangesRequest{SessionRequest: req, Limit: 2, Offset: 2, Generation: page.Generation, Version: page.Version})
	if next.Error != nil || len(next.Changes) != 1 || next.Changes[0].Path != "c" || next.NextOffset != -1 {
		t.Fatal(next)
	}
	page.Changes[0].Path = "tampered"
	page.Changes[0].After.SizeBytes = "tampered"
	fresh := a.GetChanges(ChangesRequest{SessionRequest: req})
	if fresh.Changes[0].Path != "a" || fresh.Changes[0].After.SizeBytes != "1" {
		t.Fatal("DTO aliases core state")
	}
	for _, bad := range []ChangesRequest{{SessionRequest: req, Offset: -1}, {SessionRequest: req, Limit: 501}, {SessionRequest: req, Offset: 2}, {SessionRequest: req, Offset: 4, Generation: page.Generation, Version: page.Version}} {
		requireCode(t, a.GetChanges(bad).Error, "INVALID_PAGE")
	}
	if r := a.ResetBaseline(req); r.Error != nil {
		t.Fatal(r)
	}
	requireCode(t, a.GetChanges(ChangesRequest{SessionRequest: req, Offset: 2, Generation: page.Generation, Version: page.Version}).Error, "STALE_VERSION")
}

func TestDTOContractCountersAndNoCoreTypes(t *testing.T) {
	v := uint64(9007199254740993)
	state := idle()
	state.Sequence, state.Generation, state.Version = decimal(v), decimal(v+1), decimal(v+2)
	e := Event{Protocol: ProtocolVersion, Name: EventChanges, ClientID: "client", Status: state, Reload: true}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"sequence":"9007199254740993"`) || !strings.Contains(string(b), `"generation":"9007199254740994"`) {
		t.Fatal(string(b))
	}
	seen := map[reflect.Type]bool{}
	var walk func(reflect.Type)
	walk = func(typ reflect.Type) {
		if seen[typ] {
			return
		}
		seen[typ] = true
		if strings.Contains(typ.PkgPath(), "/internal/") {
			t.Fatalf("internal type escaped IPC: %s", typ)
		}
		switch typ.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array:
			walk(typ.Elem())
		case reflect.Struct:
			for i := 0; i < typ.NumField(); i++ {
				walk(typ.Field(i).Type)
			}
		}
	}
	for _, dto := range []any{ConnectionReply{}, Reply{}, ChangesReply{}, DiffReply{}, Event{}, StartOptions{}, DiffRequest{}, ChangesRequest{}} {
		walk(reflect.TypeOf(dto))
	}
	if !validCounter("18446744073709551615") || validCounter("18446744073709551616") || validCounter("01") || validCounter("-1") {
		t.Fatal("counter validation")
	}
	if len(bounded(strings.Repeat("界", 2000))) > 2048 {
		t.Fatal("unbounded warning")
	}
}

func TestDiffBackpressure(t *testing.T) {
	f, a, client := testFacade(t)
	root := t.TempDir()
	req := started(t, a, client, root)
	f.diffSlot <- struct{}{}
	defer func() { <-f.diffSlot }()
	requireCode(t, a.GetDiff(DiffRequest{SessionRequest: req, Path: "a", Generation: "1", Version: "1"}).Error, "BUSY")
}
