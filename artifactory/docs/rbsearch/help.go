package rbsearch

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rt rbsearch <subcommand>"}

func GetDescription() string {
	return "rbsearch"
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name:        "subcommand name",
			Description: "Available Subcommands are : names, versions, artifacts, environment, status, signature. Availables flags are not applicable on all subcommands.\n For flags applicable to specific subcommands, please refer to https://jfrog.com/help/r/jfrog-rest-apis/release-lifecycle-management",
		},
	}
}
