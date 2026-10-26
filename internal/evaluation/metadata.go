// Package evaluation records local source identities without credentials or user data.
package evaluation

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type Source struct {
	Commit, GoTreeSHA256 string
	Dirty                bool
}

func Identity() Source {
	commit, _ := exec.Command("git", "rev-parse", "HEAD").Output()
	dirty, _ := exec.Command("git", "status", "--porcelain", "--untracked-files=normal").Output()
	files := []string{"go.mod", "go.sum"}
	for _, root := range []string{"internal", "cmd"} {
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(path, ".go") {
				files = append(files, path)
			}
			return err
		})
	}
	sort.Strings(files)
	hash := sha256.New()
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err == nil {
			fmt.Fprintf(hash, "%s\x00%x\n", filepath.ToSlash(path), sha256.Sum256(data))
		}
	}
	return Source{strings.TrimSpace(string(commit)), fmt.Sprintf("%x", hash.Sum(nil)), len(dirty) > 0}
}
