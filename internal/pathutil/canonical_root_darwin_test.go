package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeRootUsesFilesystemSpellingForAliases(t *testing.T) {
	for _, names := range [][2]string{{"MiXeDRoot", "mixedroot"}, {"Róót", "Ro\u0301o\u0301t"}} {
		t.Run(names[0], func(t *testing.T) {
			parent := t.TempDir()
			actual, alias := filepath.Join(parent, names[0]), filepath.Join(parent, names[1])
			if err := os.Mkdir(actual, 0700); err != nil {
				t.Fatal(err)
			}
			original, err := os.Stat(actual)
			if err != nil {
				t.Fatal(err)
			}
			alternate, err := os.Stat(alias)
			if os.IsNotExist(err) {
				t.Skip("filesystem treats these spellings as distinct names")
			}
			if err != nil || !os.SameFile(original, alternate) {
				t.Fatalf("alias identity: %v", err)
			}
			want, err := NormalizeRoot(actual, ".")
			if err != nil {
				t.Fatal(err)
			}
			got, err := NormalizeRoot(alias, ".")
			if err != nil || got != want {
				t.Fatalf("aliases must share one namespace: %q != %q (%v)", got, want, err)
			}
			// Canonicalizing an explicit root is not permission to fold file keys.
			key, err := Key(got, "MiXeD-é.txt")
			if err != nil || key != "MiXeD-é.txt" {
				t.Fatalf("descendant key spelling changed: %q %v", key, err)
			}
		})
	}
}
