package composer

import (
	"github.com/paketo-buildpacks/packit/v2"
	"github.com/paketo-buildpacks/packit/v2/chronos"
	"github.com/paketo-buildpacks/packit/v2/draft"
	"github.com/paketo-buildpacks/packit/v2/postal"
	"github.com/paketo-buildpacks/packit/v2/scribe"
	"os"
	"path/filepath"
	"time"
)

// Note that Go 1.18 requires faux 0.21.0 (https://github.com/ryanmoran/faux/releases/tag/v0.21.0)

//go:generate faux --interface DependencyManager --output fakes/dependency_manager.go
type DependencyManager interface {
	Resolve(path, id, version, stack string) (postal.Dependency, error)
	Deliver(dependency postal.Dependency, cnbPath, layerPath, platformPath string) error
}

func Build(
	logger scribe.Emitter,
	dependencyManager DependencyManager) packit.BuildFunc {
	return func(context packit.BuildContext) (packit.BuildResult, error) {
		logger.Title("%s %s", context.BuildpackInfo.Name, context.BuildpackInfo.Version)
		logger.Process("Resolving Composer version")

		entryResolver := draft.NewPlanner()

		priorities := []interface{}{
			"BP_COMPOSER_VERSION",
		}
		entry, sortedEntries := entryResolver.Resolve("composer", context.Plan.Entries, priorities)
		logger.Candidates(sortedEntries)

		composerLayer, err := context.Layers.Get("composer")
		if err != nil {
			return packit.BuildResult{}, err
		}

		composerLayer, err = composerLayer.Reset()
		if err != nil {
			return packit.BuildResult{}, err
		}

		composerLayer.Launch, composerLayer.Build = entryResolver.MergeLayerTypes("composer", context.Plan.Entries)

		if !composerLayer.Launch && !composerLayer.Build {
			composerLayer.Build = true
		}

		// version = "" is entirely fine
		version, _ := entry.Metadata["version"].(string)

		dependency, err := dependencyManager.Resolve(
			filepath.Join(context.CNBPath, "buildpack.toml"),
			entry.Name,
			version,
			context.Stack)
		if err != nil {
			return packit.BuildResult{}, err
		}

		clock := chronos.DefaultClock

		logger.SelectedDependency(entry, dependency, clock.Now())

		logger.Process("Executing build process")
		logger.Subprocess("Installing Composer %s", dependency.Version)

		layerBinPath := filepath.Join(composerLayer.Path, "bin")
		err = os.MkdirAll(layerBinPath, os.ModePerm)
		if err != nil {
			return packit.BuildResult{}, err
		}

		duration, err := clock.Measure(func() error {
			return dependencyManager.Deliver(dependency, context.CNBPath, layerBinPath, context.Platform.Path)
		})
		if err != nil {
			return packit.BuildResult{}, err
		}
		logger.Action("Completed in %s", duration.Round(time.Millisecond))
		logger.Break()

		fullFilename := filepath.Join(layerBinPath, filepath.Base(dependency.Name))

		logger.Debug.Subprocess("Composer installed at %s", fullFilename)

		err = os.Chmod(fullFilename, 0755)
		if err != nil {
			return packit.BuildResult{}, err
		}

		composerLayer.Metadata = map[string]interface{}{
			"dependency-sha": dependency.SHA256,
		}

		logger.Debug.Subprocess("Composer layer SHA256 is %s", dependency.SHA256)

		return packit.BuildResult{
			Layers: []packit.Layer{
				composerLayer,
			},
		}, nil
	}
}
