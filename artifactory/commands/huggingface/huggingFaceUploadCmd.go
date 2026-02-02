package huggingface

import (
	"encoding/json"
	"fmt"
	"github.com/jfrog/jfrog-cli-core/v2/utils/config"
	"os/exec"

	"github.com/jfrog/jfrog-client-go/utils/errorutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// HFUploadCmd represents a command to upload models or datasets to HuggingFace Hub
type HFUploadCmd struct {
	name          string
	folderPath    string
	repoId        string
	revision      string
	repoType      string
	serverDetails *config.ServerDetails
}

// Run executes the upload command to upload a model or dataset folder to HuggingFace Hub
func (hfu *HFUploadCmd) Run() error {
	if hfu.folderPath == "" {
		return errorutils.CheckErrorf("folder_path cannot be empty")
	}
	if hfu.repoId == "" {
		return errorutils.CheckErrorf("repo_id cannot be empty")
	}
	pythonPath, err := exec.LookPath("python3")
	if err != nil {
		return errorutils.CheckErrorf("python3 not found in PATH. Please ensure Python 3 is installed and available in your PATH")
	}
	log.Debug(fmt.Sprintf("Using Python interpreter: %s", pythonPath))
	scriptDir, err := getHuggingFaceScriptPath("huggingface_upload.py")
	if err != nil {
		return errorutils.CheckError(err)
	}
	args := map[string]interface{}{
		"folder_path": hfu.folderPath,
		"repo_id":     hfu.repoId,
	}
	if hfu.revision != "" {
		args["revision"] = hfu.revision
	}
	if hfu.repoType != "" {
		args["repo_type"] = hfu.repoType
	}
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return errorutils.CheckErrorf("failed to marshal arguments to JSON: %w", err)
	}
	pythonCmd := fmt.Sprintf(`import sys,json,importlib
try:
	m=importlib.import_module("huggingface_upload")
	f=getattr(m,"upload")
	f(**json.loads("""%s"""))
	print(json.dumps({"success":True}))
except Exception as e:
	print(json.dumps({"success":False,"error":str(e)}))
	sys.exit(1)`, string(argsJSON))
	log.Debug(fmt.Sprintf("Executing Python function to upload %s: %s to %s", args["repo_type"], hfu.folderPath, hfu.repoId))
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
	log.Info(fmt.Sprintf("Uploaded successfully to: %s", hfu.repoId))
	return nil
}

// ServerDetails returns the server details configuration for the command
func (hfu *HFUploadCmd) ServerDetails() (*config.ServerDetails, error) {
	return hfu.serverDetails, nil
}

// CommandName returns the name of the command
func (hfu *HFUploadCmd) CommandName() string {
	return hfu.name
}

// NewHFUploadCmd creates a new instance of HFUploadCmd
func NewHFUploadCmd() *HFUploadCmd {
	return &HFUploadCmd{}
}

// SetFolderPath sets the folder path to upload for the upload command
func (hfu *HFUploadCmd) SetFolderPath(folderPath string) *HFUploadCmd {
	hfu.folderPath = folderPath
	return hfu
}

// SetRepoId sets the repository ID for the upload command
func (hfu *HFUploadCmd) SetRepoId(repoId string) *HFUploadCmd {
	hfu.repoId = repoId
	return hfu
}

// SetRevision sets the revision (branch, tag, or commit) for the upload command
func (hfu *HFUploadCmd) SetRevision(revision string) *HFUploadCmd {
	hfu.revision = revision
	return hfu
}

// SetRepoType sets the repository type (model, dataset, or space) for the upload command
func (hfu *HFUploadCmd) SetRepoType(repoType string) *HFUploadCmd {
	hfu.repoType = repoType
	return hfu
}
