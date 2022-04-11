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
			image     occam.Image
			container occam.Container

			name   string
			source string

			env map[string]string
		)

		it.Before(func() {
			var err error
			name, err = occam.RandomName()
			Expect(err).NotTo(HaveOccurred())

			env = map[string]string{
				"BP_COMPOSER_VERSION": "*",
			}
		})

		it.After(func() {
			Expect(docker.Container.Remove.Execute(container.ID)).To(Succeed())
			Expect(docker.Image.Remove.Execute(image.ID)).To(Succeed())
			Expect(docker.Volume.Remove.Execute(occam.CacheVolumeNames(name))).To(Succeed())
			Expect(os.RemoveAll(source)).To(Succeed())
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

			container, err = docker.Container.Run.
				WithCommand("which composer && composer --version").
				Execute(image.ID)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() string {
				cLogs, err := docker.Container.Logs.Execute(container.ID)
				Expect(err).NotTo(HaveOccurred())
				return cLogs.String()
			}).Should(And(
				ContainSubstring("/layers/paketo-buildpacks_composer/composer/bin/composer"),
				MatchRegexp(`Composer version \d\.\d\.\d`),
			))
		})
	})
}
