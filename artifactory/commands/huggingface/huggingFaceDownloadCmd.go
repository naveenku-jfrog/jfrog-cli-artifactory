package huggingface

import (
	"encoding/json"
	"fmt"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"os/exec"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// HuggingFaceResult represents the result of a HuggingFace operation
type HuggingFaceResult struct {
	Success   bool   `json:"success"`
	ModelPath string `json:"model_path,omitempty"`
	Error     string `json:"error,omitempty"`
}

// HFDownloadCmd represents a command to download models or datasets from HuggingFace Hub
type HFDownloadCmd struct {
	name          string
	repoId        string
	revision      string
	repoType      string
	etagTimeout   int
	serverDetails *config.ServerDetails
}

// Run executes the download command to fetch a model or dataset from HuggingFace Hub
func (hfd *HFDownloadCmd) Run() error {
	if hfd.repoId == "" {
		return errorutils.CheckErrorf("repo_id cannot be empty")
	}
	pythonPath, err := exec.LookPath("python3")
	if err != nil {
		return errorutils.CheckErrorf("python3 not found in PATH. Please ensure Python 3 is installed and available in your PATH")
	}
	log.Debug(fmt.Sprintf("Using Python interpreter: %s", pythonPath))
	scriptDir, err := getHuggingFaceScriptPath("huggingface_download.py")
	if err != nil {
		return errorutils.CheckError(err)
	}
	args := map[string]interface{}{
		"repo_id":      hfd.repoId,
		"revision":     hfd.revision,
		"etag_timeout": hfd.etagTimeout,
	}
	if hfd.repoType != "" {
		args["repo_type"] = hfd.repoType
	} else {
		args["repo_type"] = "model"
	}
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return errorutils.CheckErrorf("failed to marshal arguments to JSON: %w", err)
	}
	pythonCmd := fmt.Sprintf(`import sys,json,importlib
try:
	m=importlib.import_module("huggingface_download")
	f=getattr(m,"download")
	r=f(**json.loads("""%s"""))
	print(json.dumps({"success":True,"model_path":r}))
except Exception as e:
	print(json.dumps({"success":False,"error":str(e)}))
	sys.exit(1)`, string(argsJSON))
	log.Debug(fmt.Sprintf("Executing Python function to download %s: %s", args["repo_type"], hfd.repoId))
	cmd := exec.Command(pythonPath, "-c", pythonCmd)
	cmd.Dir = scriptDir
	output, err := cmd.CombinedOutput()
	if len(output) == 0 {
		if err != nil {
			return errorutils.CheckErrorf("Python script produced no output and exited with error: %w", err)
		}
		return errorutils.CheckErrorf("Python script produced no output. The script may not be executing correctly.")
	}
	var result HuggingFaceResult
	if jsonErr := json.Unmarshal(output, &result); jsonErr != nil {
		if err != nil {
			return errorutils.CheckErrorf("failed to execute Python script: %w, output: %s", err, string(output))
		}
		return errorutils.CheckErrorf("failed to parse Python script output: %w, output: %s", jsonErr, string(output))
	}
	if !result.Success {
		return errorutils.CheckErrorf("%s", result.Error)
	}
	if err != nil {
		return errorutils.CheckErrorf("Python script execution failed: %w", err)
	}
	log.Info(fmt.Sprintf("Downloaded successfully to: %s", result.ModelPath))
	return nil
}

// ServerDetails returns the server details configuration for the command
func (hfd *HFDownloadCmd) ServerDetails() (*config.ServerDetails, error) {
	return hfd.serverDetails, nil
}

// CommandName returns the name of the command
func (hfd *HFDownloadCmd) CommandName() string {
	return hfd.name
}

// NewHFDownloadCmd creates a new instance of HFDownloadCmd
func NewHFDownloadCmd() *HFDownloadCmd {
	return &HFDownloadCmd{}
}

// SetRepoId sets the repository ID for the download command
func (hfd *HFDownloadCmd) SetRepoId(repoId string) *HFDownloadCmd {
	hfd.repoId = repoId
	return hfd
}

// SetRevision sets the revision (branch, tag, or commit) for the download command
func (hfd *HFDownloadCmd) SetRevision(revision string) *HFDownloadCmd {
	hfd.revision = revision
	return hfd
}

// SetRepoType sets the repository type (model, dataset, or space) for the download command
func (hfd *HFDownloadCmd) SetRepoType(repoType string) *HFDownloadCmd {
	hfd.repoType = repoType
	return hfd
}

// SetEtagTimeout sets the ETag validation timeout in seconds for the download command
func (hfd *HFDownloadCmd) SetEtagTimeout(etageTimeout int) *HFDownloadCmd {
	hfd.etagTimeout = etageTimeout
	return hfd
}
