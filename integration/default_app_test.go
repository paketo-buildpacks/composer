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

func testDefaultApp(t *testing.T, context spec.G, it spec.S) {
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
			image                    occam.Image
			composerVersionContainer occam.Container
			sbomContainer            occam.Container

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
		})

		it.After(func() {
			Expect(docker.Container.Remove.Execute(composerVersionContainer.ID)).To(Succeed())
			Expect(docker.Container.Remove.Execute(sbomContainer.ID)).To(Succeed())
			Expect(docker.Image.Remove.Execute(image.ID)).To(Succeed())
			Expect(docker.Volume.Remove.Execute(occam.CacheVolumeNames(name))).To(Succeed())
			Expect(os.RemoveAll(source)).To(Succeed())
			Expect(os.RemoveAll(sbomDir)).To(Succeed())
		})

		it("builds and puts `composer` on the $PATH", func() {
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
				MatchRegexp(fmt.Sprintf(`%s 1\.2\.3`, buildpackInfo.Buildpack.Name)),
				"  Resolving Composer version",
				"    Candidate version sources (in priority order):",
				`      BP_COMPOSER_VERSION -> "*"`,
				`      integration-test    -> "2.*"`,
				"",
				MatchRegexp(`Selected composer version \(using BP_COMPOSER_VERSION\): 2\.\d\.\d`),
			))
			Expect(logs).To(ContainLines(
				"  Executing build process",
				MatchRegexp(`\s+Installing Composer \d+\.\d+\.\d+`),
				MatchRegexp(`\s+Completed in \d+`),
			))

			composerVersionContainer, err = docker.Container.Run.
				WithCommand("which composer && composer --version").
				Execute(image.ID)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() string {
				cLogs, err := docker.Container.Logs.Execute(composerVersionContainer.ID)
				Expect(err).NotTo(HaveOccurred())
				return cLogs.String()
			}).Should(And(
				ContainSubstring("/layers/paketo-buildpacks_composer/composer/bin/composer"),
				MatchRegexp(`Composer version \d\.\d\.\d`),
			))

			// SBOM checks
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
			sbomContainer, err = docker.Container.Run.
				WithCommand("cat /layers/sbom/launch/sbom.legacy.json").
				Execute(image.ID)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() string {
				cLogs, err := docker.Container.Logs.Execute(sbomContainer.ID)
				Expect(err).NotTo(HaveOccurred())
				return cLogs.String()
			}).Should(ContainSubstring(`"name":"composer"`))

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
