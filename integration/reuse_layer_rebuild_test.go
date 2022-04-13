package integration_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/paketo-buildpacks/occam"
	"github.com/sclevine/spec"

	. "github.com/onsi/gomega"
)

func testReusingLayerRebuild(t *testing.T, context spec.G, it spec.S) {
	var (
		Expect = NewWithT(t).Expect

		docker occam.Docker
		pack   occam.Pack

		imageIDs     map[string]struct{}
		containerIDs map[string]struct{}

		name   string
		source string
	)

	it.Before(func() {
		var err error
		name, err = occam.RandomName()
		Expect(err).NotTo(HaveOccurred())

		docker = occam.NewDocker()
		pack = occam.NewPack()
		imageIDs = map[string]struct{}{}
		containerIDs = map[string]struct{}{}
	})

	it.After(func() {
		for id := range containerIDs {
			Expect(docker.Container.Remove.Execute(id)).To(Succeed())
		}

		for id := range imageIDs {
			Expect(docker.Image.Remove.Execute(id)).To(Succeed())
		}

		Expect(docker.Volume.Remove.Execute(occam.CacheVolumeNames(name))).To(Succeed())
		Expect(os.RemoveAll(source)).To(Succeed())
	})

	context("when an app is rebuilt and composer version does not change", func() {
		it("reuses a layer from a previous build", func() {
			var (
				err         error
				logs        fmt.Stringer
				firstImage  occam.Image
				secondImage occam.Image
			)

			source, err = occam.Source(filepath.Join("testdata", "default_app"))
			Expect(err).NotTo(HaveOccurred())

			build := pack.WithNoColor().Build.
				WithPullPolicy("never").
				WithBuildpacks(
					buildpacks.PhpDist,
					buildpacks.Composer,
					buildpacks.BuildPlan,
				)

			firstImage, logs, err = build.Execute(name, source)
			Expect(err).NotTo(HaveOccurred())

			imageIDs[firstImage.ID] = struct{}{}

			Expect(firstImage.Buildpacks).To(HaveLen(3))

			Expect(firstImage.Buildpacks[1].Key).To(Equal(buildpackInfo.Buildpack.ID))
			Expect(firstImage.Buildpacks[1].Layers).To(HaveKey("composer"))

			Expect(logs.String()).To(ContainSubstring("  Executing build process"))

			// Second pack build
			secondImage, logs, err = build.Execute(name, source)
			Expect(err).NotTo(HaveOccurred())

			imageIDs[secondImage.ID] = struct{}{}

			Expect(secondImage.Buildpacks).To(HaveLen(3))

			Expect(secondImage.Buildpacks[1].Key).To(Equal(buildpackInfo.Buildpack.ID))
			Expect(secondImage.Buildpacks[1].Layers).To(HaveKey("composer"))

			Expect(logs.String()).NotTo(ContainSubstring("  Executing build process"))
			Expect(logs.String()).To(ContainSubstring("Reusing cached layer /layers/paketo-buildpacks_composer/composer"))

			Expect(secondImage.Buildpacks[1].Layers["composer"].SHA).To(Equal(firstImage.Buildpacks[1].Layers["composer"].SHA))
		})
	})

	context("when an app is rebuilt and there is a change in composer version", func() {
		it("rebuilds the layer", func() {
			var (
				err         error
				logs        fmt.Stringer
				firstImage  occam.Image
				secondImage occam.Image
			)

			source, err = occam.Source(filepath.Join("testdata", "default_app"))
			Expect(err).NotTo(HaveOccurred())

			build := pack.WithNoColor().Build.
				WithPullPolicy("never").
				WithBuildpacks(
					buildpacks.PhpDist,
					buildpacks.Composer,
					buildpacks.BuildPlan,
				)

			firstImage, logs, err = build.Execute(name, source)
			Expect(err).NotTo(HaveOccurred())

			imageIDs[firstImage.ID] = struct{}{}

			Expect(firstImage.Buildpacks).To(HaveLen(3))

			Expect(firstImage.Buildpacks[1].Key).To(Equal(buildpackInfo.Buildpack.ID))
			Expect(firstImage.Buildpacks[1].Layers).To(HaveKey("composer"))

			Expect(logs.String()).To(ContainSubstring("  Executing build process"))

			// Second pack build
			secondImage, logs, err = build.WithEnv(map[string]string{"BP_COMPOSER_VERSION": buildpackInfo.Metadata.Dependencies[0].Version}).Execute(name, source)
			Expect(err).NotTo(HaveOccurred())

			imageIDs[secondImage.ID] = struct{}{}

			Expect(secondImage.Buildpacks).To(HaveLen(3))

			Expect(secondImage.Buildpacks[1].Key).To(Equal(buildpackInfo.Buildpack.ID))
			Expect(secondImage.Buildpacks[1].Layers).To(HaveKey("composer"))

			Expect(logs.String()).To(ContainSubstring("  Executing build process"))
			Expect(logs.String()).NotTo(ContainSubstring("Reusing cached layer /layers/paketo-buildpacks_composer/composer"))

			Expect(secondImage.Buildpacks[1].Layers["composer"].SHA).NotTo(Equal(firstImage.Buildpacks[1].Layers["composer"].SHA))
		})
	})
}
