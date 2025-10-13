package search

import "github.com/jfrog/jfrog-cli-core/v2/plugins/components"

var Usage = []string{"rbs <option>"}

func GetDescription() string {
	return "release-bundle-search"
}

func GetArguments() []components.Argument {
	return []components.Argument{
		{
			Name: "option",
			Description: "Available option is : names, versions." +
				"Example: jf rbs names" +
				"Example: jf rbs versions release-bundle-name" +
				" All Available flags are not applicable with all options.\n For flags applicable to specific option, please refer to https://jfrog.com/help/r/jfrog-rest-apis/release-lifecycle-management",
		},
	}
}
