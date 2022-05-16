package integration_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paketo-buildpacks/occam"
	"github.com/sclevine/spec"

	. "github.com/onsi/gomega"
	. "github.com/paketo-buildpacks/occam/matchers"
)

func testOffline(t *testing.T, context spec.G, it spec.S) {
	var (
		Expect     = NewWithT(t).Expect
		Eventually = NewWithT(t).Eventually
		pack       occam.Pack
		docker     occam.Docker
	)

	it.Before(func() {
		pack = occam.NewPack()
		docker = occam.NewDocker()
	})

	context("when offline", func() {
		var (
			image     occam.Image
			container occam.Container
			name      string
			source    string
		)

		it.Before(func() {
			var err error
			name, err = occam.RandomName()
			Expect(err).NotTo(HaveOccurred())
		})

		it.After(func() {
			Expect(docker.Container.Remove.Execute(container.ID)).To(Succeed())
			Expect(docker.Image.Remove.Execute(image.ID)).To(Succeed())
			Expect(docker.Volume.Remove.Execute(occam.CacheVolumeNames(name))).To(Succeed())
			Expect(os.RemoveAll(source)).To(Succeed())
		})

		it("creates a working OCI image with specified version of php on PATH", func() {
			var err error
			source, err = occam.Source(filepath.Join("testdata", "default_app"))
			Expect(err).NotTo(HaveOccurred())

			var logs fmt.Stringer
			image, logs, err = pack.WithNoColor().Build.
				WithPullPolicy("never").
				WithBuildpacks(
					buildpacks.PhpDistOffline,
					buildpacks.ComposerOffline,
					buildpacks.BuildPlan,
				).
				WithNetwork("none").
				Execute(name, source)
			Expect(err).NotTo(HaveOccurred(), logs.String())

			Expect(logs).To(ContainLines(
				MatchRegexp(fmt.Sprintf(`%s 1\.2\.3`, buildpackInfo.Buildpack.Name)),
				"  Resolving Composer version",
				"    Candidate version sources (in priority order):",
				`      integration-test -> "2.*"`,
				"",
				MatchRegexp(`Selected composer version \(using integration-test\): 2\.\d\.\d`),
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
				ContainSubstring(fmt.Sprintf("/layers/%s/composer/bin/composer", strings.ReplaceAll(buildpackInfo.Buildpack.ID, "/", "_"))),
				MatchRegexp(`Composer version \d\.\d\.\d`),
			))
		})
	})
}
