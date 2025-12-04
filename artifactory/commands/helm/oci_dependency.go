package helm

import (
	"fmt"

	"github.com/jfrog/build-info-go/entities"
	servicesUtils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// addOCIDependency adds an OCI artifact as a separate dependency
func addOCIDependency(module *entities.Module, resultItem *servicesUtils.ResultItem) {
	ociDependency := entities.Dependency{
		Id: fmt.Sprintf("%s/%s/%s", resultItem.Repo, resultItem.Path, resultItem.Name),
		Checksum: entities.Checksum{
			Sha1:   resultItem.Actual_Sha1,
			Sha256: resultItem.Sha256,
			Md5:    resultItem.Actual_Md5,
		},
	}

	module.Dependencies = append(module.Dependencies, ociDependency)
	log.Debug(fmt.Sprintf("Added OCI artifact as dependency: %s (path: %s/%s)", resultItem.Name, resultItem.Path, resultItem.Name))
}
