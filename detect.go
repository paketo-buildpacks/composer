package composer

import (
	"github.com/paketo-buildpacks/packit/v2"
	"os"
)

func Detect() packit.DetectFunc {
	return func(context packit.DetectContext) (packit.DetectResult, error) {
		var requirements []packit.BuildPlanRequirement

		if version, ok := os.LookupEnv("BP_COMPOSER_VERSION"); ok {
			requirements = append(requirements, packit.BuildPlanRequirement{
				Name: "composer",
				Metadata: BuildPlanMetadata{
					VersionSource: "BP_COMPOSER_VERSION",
					Version:       version,
				},
			})
		}

		return packit.DetectResult{
			Plan: packit.BuildPlan{
				Provides: []packit.BuildPlanProvision{
					{
						Name: "composer",
					},
				},
				Requires: requirements,
			},
		}, nil
	}
}
