package huggingface

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
)

// getHuggingFaceScriptPath returns the absolute path to the directory containing Python scripts
func getHuggingFaceScriptPath(scriptName string) (string, error) {
	_, filename, _, ok := runtime.Caller(1)
	if !ok {
		return "", errorutils.CheckErrorf("failed to get current file path")
	}
	scriptDir := filepath.Dir(filename)
	scriptPath := filepath.Join(scriptDir, scriptName)
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return "", errorutils.CheckErrorf("Python script not found: %s in directory: %s", scriptName, scriptDir)
	}
	return scriptDir, nil
}
