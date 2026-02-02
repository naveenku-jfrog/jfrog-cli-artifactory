package huggingfaceupload

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt hfu <folder-path> <repo-id>"}

func GetDescription() string {
	return "Upload a model or dataset folder to HuggingFace Hub."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "folder-path",
			Description: "Path to the folder to upload.",
		},
		{
			Name:        "repo-id",
			Description: "The HuggingFace repository ID (e.g., 'username/model-name' or 'username/dataset-name').",
		},
	}
}
