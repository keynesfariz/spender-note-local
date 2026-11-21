package domain

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type LocalGgufModelInfo struct {
	RelativePath string `json:"relativePath"`
	SizeBytes    int64  `json:"sizeBytes"`
}

func GetModelsDir() string {
	dir := strings.TrimSpace(os.Getenv("LLM_MODELS_DIR"))
	if dir == "" {
		return "models"
	}
	return dir
}

func isStrictlyInside(parentReal, childReal string) bool {
	rel, err := filepath.Rel(parentReal, childReal)
	if err != nil {
		return false
	}
	if rel == "" || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return false
	}
	return true
}

func ListLocalGgufModels(modelsDir string) []LocalGgufModelInfo {
	realRoot, err := filepath.EvalSymlinks(modelsDir)
	if err != nil {
		return []LocalGgufModelInfo{}
	}
	info, err := os.Stat(realRoot)
	if err != nil || !info.IsDir() {
		return []LocalGgufModelInfo{}
	}

	results := []LocalGgufModelInfo{}
	_ = filepath.WalkDir(realRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || path == realRoot {
			return nil
		}
		realPath, err := filepath.EvalSymlinks(path)
		if err != nil || !isStrictlyInside(realRoot, realPath) {
			return nil
		}
		st, err := os.Stat(realPath)
		if err != nil {
			return nil
		}
		if !st.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".gguf") {
			rel, err := filepath.Rel(realRoot, realPath)
			if err == nil {
				results = append(results, LocalGgufModelInfo{
					RelativePath: filepath.ToSlash(rel),
					SizeBytes:    st.Size(),
				})
			}
		}
		return nil
	})

	sort.Slice(results, func(i, j int) bool {
		return results[i].RelativePath < results[j].RelativePath
	})
	return results
}

func ResolveSafeGgufPath(modelsDir string, relativeModelPath string) (string, error) {
	trimmed := strings.TrimSpace(relativeModelPath)
	if trimmed == "" {
		return "", fmt.Errorf("model path cannot be empty")
	}
	if strings.ContainsRune(trimmed, 0) {
		return "", fmt.Errorf("model path contains invalid null byte")
	}
	if filepath.IsAbs(trimmed) {
		return "", fmt.Errorf("model path must be relative to the models directory")
	}

	realRoot, err := filepath.EvalSymlinks(modelsDir)
	if err != nil {
		return "", fmt.Errorf("models directory does not exist: %s", modelsDir)
	}

	candidate := filepath.Join(realRoot, trimmed)
	if !isStrictlyInside(realRoot, candidate) {
		return "", fmt.Errorf("model path escapes models directory: %s", trimmed)
	}

	realTarget, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("model file not found: %s", trimmed)
	}
	if !isStrictlyInside(realRoot, realTarget) {
		return "", fmt.Errorf("model symlink target escapes models directory: %s", trimmed)
	}

	st, err := os.Stat(realTarget)
	if err != nil || !st.Mode().IsRegular() {
		return "", fmt.Errorf("model path is not a regular file: %s", trimmed)
	}
	if !strings.HasSuffix(strings.ToLower(realTarget), ".gguf") {
		return "", fmt.Errorf("model file must have .gguf extension: %s", trimmed)
	}
	f, err := os.Open(realTarget)
	if err != nil {
		return "", fmt.Errorf("model file is not readable: %s", trimmed)
	}
	_ = f.Close()
	return realTarget, nil
}
