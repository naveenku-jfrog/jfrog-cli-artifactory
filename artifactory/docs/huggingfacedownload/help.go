package huggingfacedownload

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt hfd <model-name>"}

func GetDescription() string {
	return "Download a model/dataset from HuggingFace Hub."
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "model/dataset name",
			Description: "The HuggingFace model repository ID (e.g., 'bert-base-uncased' or 'username/model-name').",
		},
	}
}
