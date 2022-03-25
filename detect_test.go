package composer_test

import (
	"github.com/paketo-buildpacks/composer"
	"os"
	"testing"

	"github.com/paketo-buildpacks/packit/v2"
	"github.com/sclevine/spec"

	. "github.com/onsi/gomega"
)

func testDetect(t *testing.T, context spec.G, it spec.S) {
	var (
		Expect = NewWithT(t).Expect

		detect packit.DetectFunc
	)

	it.Before(func() {
		detect = composer.Detect()
	})

	it.After(func() {
		Expect(os.Unsetenv("BP_COMPOSER_VERSION")).To(Succeed())
	})

	context("when BP_COMPOSER_VERSION is not set", func() {
		it(`provides "composer" without requiring anything`, func() {
			detectResult, err := detect(packit.DetectContext{})
			Expect(err).NotTo(HaveOccurred())

			Expect(detectResult.Plan).To(Equal(packit.BuildPlan{
				Provides: []packit.BuildPlanProvision{
					{
						Name: "composer",
					},
				},
			}))
		})
	})

	context("when BP_COMPOSER_VERSION is set", func() {
		it.Before(func() {
			Expect(os.Setenv("BP_COMPOSER_VERSION", "composer.version.from-env")).To(Succeed())
		})

		it(`provides "composer" and requires "composer" with version metadata`, func() {
			detectResult, err := detect(packit.DetectContext{})
			Expect(err).NotTo(HaveOccurred())

			Expect(detectResult.Plan).To(Equal(packit.BuildPlan{
				Provides: []packit.BuildPlanProvision{
					{
						Name: "composer",
					},
				},
				Requires: []packit.BuildPlanRequirement{{
					Name: "composer",
					Metadata: composer.BuildPlanMetadata{
						Version:       "composer.version.from-env",
						VersionSource: "BP_COMPOSER_VERSION",
					}},
				},
			}))
		})
	})
}
