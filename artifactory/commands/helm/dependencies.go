package helm

import (
	"fmt"
	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// deduplicateDependencies removes duplicate dependencies
func deduplicateDependencies(buildInfo *entities.BuildInfo) {
	if buildInfo == nil || len(buildInfo.Modules) == 0 {
		return
	}

	for moduleIdx := range buildInfo.Modules {
		module := &buildInfo.Modules[moduleIdx]
		if len(module.Dependencies) == 0 {
			continue
		}

		seenChecksums := make(map[string]int)
		uniqueDeps := make([]entities.Dependency, 0, len(module.Dependencies))

		for _, dep := range module.Dependencies {
			if dep.Sha256 == "" {
				uniqueDeps = append(uniqueDeps, dep)
				continue
			}

			if existingIdx, found := seenChecksums[dep.Sha256]; found {
				existingDep := uniqueDeps[existingIdx]
				if len(dep.Id) > len(existingDep.Id) {
					uniqueDeps[existingIdx] = dep
					log.Debug(fmt.Sprintf("Removing duplicate dependency %s (keeping %s with same SHA256: %s)",
						existingDep.Id, dep.Id, dep.Sha256))
				} else {
					log.Debug(fmt.Sprintf("Removing duplicate dependency %s (keeping %s with same SHA256: %s)",
						dep.Id, existingDep.Id, dep.Sha256))
				}
			} else {
				seenChecksums[dep.Sha256] = len(uniqueDeps)
				uniqueDeps = append(uniqueDeps, dep)
			}
		}

		if len(uniqueDeps) < len(module.Dependencies) {
			log.Debug(fmt.Sprintf("Deduplicated dependencies in module[%d]: %d -> %d", moduleIdx, len(module.Dependencies), len(uniqueDeps)))
			module.Dependencies = uniqueDeps
		}
	}
}
