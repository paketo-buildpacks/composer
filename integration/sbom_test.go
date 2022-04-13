package integration_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/paketo-buildpacks/occam"
	"github.com/sclevine/spec"

	. "github.com/onsi/gomega"
	. "github.com/paketo-buildpacks/occam/matchers"
)

func testSbom(t *testing.T, context spec.G, it spec.S) {
	var (
		Expect     = NewWithT(t).Expect
		Eventually = NewWithT(t).Eventually

		pack   occam.Pack
		docker occam.Docker
	)

	it.Before(func() {
		pack = occam.NewPack().WithVerbose().WithNoColor()
		docker = occam.NewDocker()
	})

	context("with BP_COMPOSER_VERSION set", func() {
		var (
			image     occam.Image
			container occam.Container

			name    string
			source  string
			sbomDir string

			env map[string]string
		)

		it.Before(func() {
			var err error
			name, err = occam.RandomName()
			Expect(err).NotTo(HaveOccurred())

			env = map[string]string{
				"BP_COMPOSER_VERSION": "*",
				"BP_LOG_LEVEL":        "DEBUG",
			}

			sbomDir, err = os.MkdirTemp("", "sbom")
			Expect(err).NotTo(HaveOccurred())
			Expect(os.Chmod(sbomDir, os.ModePerm)).To(Succeed())
		})

		it.After(func() {
			Expect(docker.Container.Remove.Execute(container.ID)).To(Succeed())
			Expect(docker.Image.Remove.Execute(image.ID)).To(Succeed())
			Expect(docker.Volume.Remove.Execute(occam.CacheVolumeNames(name))).To(Succeed())
			Expect(os.RemoveAll(source)).To(Succeed())
			Expect(os.RemoveAll(sbomDir)).To(Succeed())
		})

		it("generates SBOM", func() {
			var err error
			var logs fmt.Stringer

			source, err = occam.Source(filepath.Join("testdata", "default_app"))
			Expect(err).NotTo(HaveOccurred())

			image, logs, err = pack.Build.
				WithPullPolicy("never").
				WithBuildpacks(
					buildpacks.PhpDist,
					buildpacks.Composer,
					buildpacks.BuildPlan,
				).
				WithEnv(env).
				WithSBOMOutputDir(sbomDir).
				Execute(name, source)
			Expect(err).ToNot(HaveOccurred(), logs.String)

			Expect(logs).To(ContainLines(
				fmt.Sprintf("  Generating SBOM for /layers/%s/composer", buildpackInfo.Buildpack.PackId),
				MatchRegexp(`      Completed in ([0-9]*(\.[0-9]*)?[a-z]+)+`),
				"",
				"  Writing SBOM in the following format(s):",
				"    application/vnd.cyclonedx+json",
				"    application/spdx+json",
				"    application/vnd.syft+json",
				"",
			))

			// check that legacy SBOM is included via metadata
			container, err = docker.Container.Run.
				WithCommand("cat /layers/config/metadata.toml").
				Execute(image.ID)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() string {
				cLogs, err := docker.Container.Logs.Execute(container.ID)
				Expect(err).NotTo(HaveOccurred())
				return cLogs.String()
			}).Should(ContainSubstring(`[[bom]]
  name = "composer"
  [bom.metadata]`))

			// check that all required SBOM files are present
			Expect(filepath.Join(sbomDir, "sbom", "launch", buildpackInfo.Buildpack.PackId, "composer", "sbom.cdx.json")).To(BeARegularFile())
			Expect(filepath.Join(sbomDir, "sbom", "launch", buildpackInfo.Buildpack.PackId, "composer", "sbom.spdx.json")).To(BeARegularFile())
			Expect(filepath.Join(sbomDir, "sbom", "launch", buildpackInfo.Buildpack.PackId, "composer", "sbom.syft.json")).To(BeARegularFile())

			// check an SBOM file to make sure it has an entry for Composer
			contents, err := os.ReadFile(filepath.Join(sbomDir, "sbom", "launch", buildpackInfo.Buildpack.PackId, "composer", "sbom.cdx.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(contents)).To(ContainSubstring(`"name": "composer"`))
		})
	})
}
