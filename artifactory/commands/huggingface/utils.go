package cli

import (
	"encoding/json"
	"fmt"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// timestampPattern matches ISO 8601 timestamp format: _YYYY-MM-DDTHH:MM:SS.sssZ
var timestampPattern = regexp.MustCompile(`_\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`)

const minPythonVersion = 3

// PythonScriptTemplate is the base template for executing Python functions via importlib.
// It accepts format arguments: module name, function name, JSON args, and success output expression.
const PythonScriptTemplate = `import sys,json,importlib
try:
	m=importlib.import_module("%s")
	f=getattr(m,"%s")
	%s
except Exception as e:
	print(json.dumps({"success":False,"error":str(e)}))
	sys.exit(1)`

// PythonUploadSuccessBlock is the success block for upload operations
const PythonUploadSuccessBlock = `f(**json.loads("""%s"""))
	print(json.dumps({"success":True}))`

// PythonDownloadSuccessBlock is the success block for download operations
const PythonDownloadSuccessBlock = `r=f(**json.loads("""%s"""))
	print(json.dumps({"success":True,"model_path":r}))`

// BuildPythonUploadCmd builds the Python command string for upload operations
func BuildPythonUploadCmd(argsJSON string) string {
	successBlock := fmt.Sprintf(PythonUploadSuccessBlock, argsJSON)
	return fmt.Sprintf(PythonScriptTemplate, "huggingface_upload", "upload", successBlock)
}

// BuildPythonDownloadCmd builds the Python command string for download operations
func BuildPythonDownloadCmd(argsJSON string) string {
	successBlock := fmt.Sprintf(PythonDownloadSuccessBlock, argsJSON)
	return fmt.Sprintf(PythonScriptTemplate, "huggingface_download", "download", successBlock)
}

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

// GetPythonPath finds a valid Python 3+ interpreter in PATH.
// It first tries "python3", then falls back to "python", and verifies the version is 3+.
func GetPythonPath() (string, error) {
	pythonPath, err := exec.LookPath("python3")
	if err == nil {
		log.Debug("Found Python interpreter: ", pythonPath)
		if err := verifyPythonVersion(pythonPath); err == nil {
			return pythonPath, nil
		}
	}
	pythonPath, err = exec.LookPath("python")
	if err != nil {
		return "", errorutils.CheckErrorf("neither python3 nor python found in PATH. Please ensure Python 3 is installed and available in your PATH")
	}
	log.Debug("Found Python interpreter: ", pythonPath)
	if err := verifyPythonVersion(pythonPath); err != nil {
		return "", err
	}
	return pythonPath, nil
}

// verifyPythonVersion checks that the Python interpreter is version 3 or higher
func verifyPythonVersion(pythonPath string) error {
	cmd := exec.Command(pythonPath, "-c", "import sys; print(sys.version_info.major)")
	output, err := cmd.Output()
	if err != nil {
		return errorutils.CheckErrorf("failed to get Python version: %w", err)
	}
	versionStr := strings.TrimSpace(string(output))
	majorVersion, err := strconv.Atoi(versionStr)
	if err != nil {
		return errorutils.CheckErrorf("failed to parse Python version '%s': %w", versionStr, err)
	}
	if majorVersion < minPythonVersion {
		return errorutils.CheckErrorf("Python version %d found, but version %d or higher is required", majorVersion, minPythonVersion)
	}
	log.Debug("Python version ", majorVersion, " verified (minimum required: ", minPythonVersion, ")")
	return nil
}

// InstallHuggingFaceHub checks if huggingface_hub is installed and installs it if not
func InstallHuggingFaceHub(pythonPath string) error {
	// Check if huggingface_hub is already installed
	checkCmd := exec.Command(pythonPath, "-c", "import huggingface_hub")
	if err := checkCmd.Run(); err == nil {
		log.Debug("huggingface_hub is already installed")
		return nil
	}
	// Install huggingface_hub using pip with --user flag for externally-managed environments (PEP 668)
	log.Info("Installing huggingface_hub library...")
	installCmd := exec.Command(pythonPath, "-m", "pip", "install", "huggingface_hub", "--user", "--quiet")
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr
	if err := installCmd.Run(); err != nil {
		// If --user fails, try with --break-system-packages as fallback
		log.Debug("User install failed, trying with --break-system-packages")
		fallbackCmd := exec.Command(pythonPath, "-m", "pip", "install", "huggingface_hub", "--break-system-packages", "--quiet")
		fallbackCmd.Stdout = os.Stdout
		fallbackCmd.Stderr = os.Stderr
		if fallbackErr := fallbackCmd.Run(); fallbackErr != nil {
			return errorutils.CheckErrorf("failed to install huggingface_hub: %w. Please install manually using: pip install huggingface_hub --user", err)
		}
	}
	log.Info("huggingface_hub installed successfully")
	return nil
}

// InstallHFTransfer checks if hf_transfer is installed and installs it if not
// hf_transfer is a Rust-based library that speeds up downloads/uploads with HuggingFace Hub
func InstallHFTransfer(pythonPath string) error {
	// Check if hf_transfer is already installed
	checkCmd := exec.Command(pythonPath, "-c", "import hf_transfer")
	if err := checkCmd.Run(); err == nil {
		log.Debug("hf_transfer is already installed")
		return nil
	}
	// Install hf_transfer using pip with --user flag for externally-managed environments (PEP 668)
	log.Info("Installing hf_transfer library for faster downloads...")
	installCmd := exec.Command(pythonPath, "-m", "pip", "install", "hf_transfer", "--user", "--quiet")
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr
	if err := installCmd.Run(); err != nil {
		// If --user fails, try with --break-system-packages as fallback
		log.Debug("User install failed, trying with --break-system-packages")
		fallbackCmd := exec.Command(pythonPath, "-m", "pip", "install", "hf_transfer", "--break-system-packages", "--quiet")
		fallbackCmd.Stdout = os.Stdout
		fallbackCmd.Stderr = os.Stderr
		if fallbackErr := fallbackCmd.Run(); fallbackErr != nil {
			return errorutils.CheckErrorf("failed to install hf_transfer: %w. Please install manually using: pip install hf_transfer --user", err)
		}
	}
	log.Info("hf_transfer installed successfully")
	return nil
}

// HasTimestamp checks if a revision ID already contains a timestamp suffix
// Example: "main_2026-02-09T09:01:17.646Z" returns true, "main" returns false
func HasTimestamp(revision string) bool {
	return timestampPattern.MatchString(revision)
}

// GetRepoKeyFromHFEndpoint extracts the repository key from HF_ENDPOINT environment variable
func GetRepoKeyFromHFEndpoint() (string, error) {
	endpoint := os.Getenv("HF_ENDPOINT")
	if endpoint == "" {
		return "", errorutils.CheckErrorf("HF_ENDPOINT environment variable is not set")
	}
	endpoint = strings.TrimSuffix(endpoint, "/")
	parts := strings.Split(endpoint, "/")
	if len(parts) == 0 {
		return "", errorutils.CheckErrorf("invalid HF_ENDPOINT format: %s", endpoint)
	}
	repoKey := parts[len(parts)-1]
	if repoKey == "" {
		return "", errorutils.CheckErrorf("could not extract repo key from HF_ENDPOINT: %s", endpoint)
	}
	log.Debug("Extracted repo key from HF_ENDPOINT: ", repoKey)
	return repoKey, nil
}

// GetHuggingFaceCliPath finds the huggingface-cli or hf command in PATH
// Returns the command path and any prefix args needed (for Python module mode)
func GetHuggingFaceCliPath() (cmdPath string, prefixArgs []string, err error) {
	// Try huggingface-cli first
	hfCliPath, lookErr := exec.LookPath("huggingface-cli")
	if lookErr == nil {
		log.Debug("Found huggingface-cli: ", hfCliPath)
		return hfCliPath, nil, nil
	}
	// Try hf as fallback
	hfCliPath, lookErr = exec.LookPath("hf")
	if lookErr == nil {
		log.Debug("Found hf: ", hfCliPath)
		return hfCliPath, nil, nil
	}
	// Fall back to Python module mode
	pythonPath, pythonErr := GetPythonPath()
	if pythonErr != nil {
		return "", nil, errorutils.CheckErrorf("neither huggingface-cli nor hf found in PATH, and Python is not available: %w", pythonErr)
	}
	log.Debug("Using Python module mode: ", pythonPath, " -m huggingface_hub.commands.huggingface_cli")
	return pythonPath, []string{"-m", "huggingface_hub.commands.huggingface_cli"}, nil
}

// updateReaderContents updates the reader contents by writing the specified JSON value to all file paths
func updateReaderContents(reader *content.ContentReader, repo, path, name string) error {
	if reader == nil {
		return fmt.Errorf("reader is nil")
	}
	jsonData := map[string]interface{}{
		"results": []map[string]interface{}{
			{
				"repo": repo,
				"path": path,
				"name": name,
				"type": "folder",
			},
		},
	}
	jsonBytes, err := json.MarshalIndent(jsonData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	filesPaths := reader.GetFilesPaths()
	for _, filePath := range filesPaths {
		err := os.WriteFile(filePath, jsonBytes, 0644)
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to write JSON to file %s: %s", filePath, err))
			continue
		}
		log.Debug(fmt.Sprintf("Successfully updated file %s with JSON content", filePath))
	}
	return nil
}

func addBuildPropertiesOnArtifacts(serviceManager artifactory.ArtifactoryServicesManager, reader *content.ContentReader, buildProps string) {
	propsParams := services.PropsParams{
		Reader:      reader,
		Props:       buildProps,
		IsRecursive: true,
	}
	_, _ = serviceManager.SetProps(propsParams)
}
