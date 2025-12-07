package helm

import (
	"fmt"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"

	"github.com/jfrog/build-info-go/entities"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

func handlePullCommand(buildInfo *entities.BuildInfo, helmArgs []string, serviceManager artifactory.ArtifactoryServicesManager) {
	chartPath, err := getPullChartPath("pull", helmArgs)
	if err != nil || chartPath == "" {
		log.Debug(fmt.Sprintf("Could not extract chart path: %v", err))
		return
	}
	log.Debug(fmt.Sprintf("Extracting dependencies from chart: %s", chartPath))
	settings := cli.New()
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), "", "secret", func(format string, v ...interface{}) {
		log.Debug(fmt.Sprintf(format, v...))
	}); err != nil {
		log.Debug(fmt.Sprintf("Warning: failed to initialize action config: %v. Continuing with chart loading...", err))
	}

	pullClient := action.NewPull()
	chartPathOptions := &pullClient.ChartPathOptions
	updateChartPathOptionsFromArgs(chartPathOptions, helmArgs)
	resolvedChartPath, err := chartPathOptions.LocateChart(chartPath, settings)
	if err != nil {
		return
	}
	log.Debug(fmt.Sprintf("Resolved chart path: %s", resolvedChartPath))
	loadedChart, err := loader.Load(resolvedChartPath)
	if err != nil {
		return
	}
	if loadedChart.Lock == nil {
		log.Debug(fmt.Sprintf("Chart.Lock is not available: %s", loadedChart.Metadata.Name))
		return
	}
	dependencies := loadedChart.Lock.Dependencies
	if dependencies == nil {
		return
	}
	dependenciesWithChecksum, err := getDependenciesWithChecksums(dependencies, serviceManager, nil)
	if err != nil {
		return
	}
	if len(dependenciesWithChecksum) > 0 && buildInfo != nil && len(buildInfo.Modules) > 0 {
		buildInfo.Modules[0].Dependencies = append(buildInfo.Modules[0].Dependencies, dependenciesWithChecksum...)
	}
	return
}
