package composer_test

import (
	"bytes"
	. "github.com/onsi/gomega"
	"github.com/paketo-buildpacks/composer"
	"github.com/paketo-buildpacks/composer/fakes"
	"github.com/paketo-buildpacks/packit/v2"
	"github.com/paketo-buildpacks/packit/v2/draft"
	"github.com/paketo-buildpacks/packit/v2/fs"
	"github.com/paketo-buildpacks/packit/v2/postal"
	"github.com/paketo-buildpacks/packit/v2/scribe"
	"github.com/sclevine/spec"
	"os"
	"path/filepath"
	"testing"
)

func testBuild(t *testing.T, context spec.G, it spec.S) {
	var (
		Expect = NewWithT(t).Expect

		cnbDir     string
		workingDir string
		layersDir  string
		dependency postal.Dependency

		buffer            *bytes.Buffer
		dependencyManager *fakes.DependencyManager
		entryResolver     = draft.NewPlanner()

		build         packit.BuildFunc
		buildpackPlan packit.BuildpackPlan
	)

	it.Before(func() {
		buffer = bytes.NewBuffer(nil)
		logEmitter := scribe.NewEmitter(buffer)

		var err error
		layersDir, err = os.MkdirTemp("", "layers")
		Expect(err).NotTo(HaveOccurred())

		workingDir, err = os.MkdirTemp("", "working-dir")
		Expect(err).NotTo(HaveOccurred())

		cnbDir, err = os.MkdirTemp("", "cnb")
		Expect(err).NotTo(HaveOccurred())

		dependencyManager = &fakes.DependencyManager{}

		build = composer.Build(logEmitter, dependencyManager, entryResolver)

		composerArchive, err := os.CreateTemp(cnbDir, "composer-archive")
		Expect(err).NotTo(HaveOccurred())
		composerArchiveName := filepath.Base(composerArchive.Name())

		Expect(os.Chmod(composerArchive.Name(), 0777)).To(Succeed())

		dependency = postal.Dependency{
			ID:      "composer",
			Name:    composerArchiveName,
			Version: "composer-dependency-version",
			SHA256:  "some-sha",
		}

		dependencyManager.ResolveCall.Returns.Dependency = dependency
		dependencyManager.DeliverCall.Stub = func(dependency postal.Dependency, cnbPath, layerPath, _ string) error {
			return fs.Copy(filepath.Join(cnbPath, dependency.Name), filepath.Join(layerPath, dependency.Name))
		}

		buildpackPlan = packit.BuildpackPlan{
			Entries: []packit.BuildpackPlanEntry{
				{
					Name: "composer",
					Metadata: map[string]interface{}{
						"build":  true,
						"launch": true,
					},
				},
			},
		}
	})

	it.After(func() {
		Expect(os.RemoveAll(layersDir)).To(Succeed())
		Expect(os.RemoveAll(workingDir)).To(Succeed())
		Expect(os.RemoveAll(cnbDir)).To(Succeed())
	})

	it("returns a result that installs composer", func() {
		result, err := build(packit.BuildContext{
			WorkingDir: workingDir,
			CNBPath:    cnbDir,
			Stack:      "some-stack",
			BuildpackInfo: packit.BuildpackInfo{
				Name:    "Some Buildpack",
				Version: "some-version",
			},
			Platform: packit.Platform{Path: "platform"},
			Plan:     buildpackPlan,
			Layers:   packit.Layers{Path: layersDir},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(result).To(Equal(packit.BuildResult{
			Layers: []packit.Layer{
				{
					Name:             "composer",
					Path:             filepath.Join(layersDir, "composer"),
					SharedEnv:        packit.Environment{},
					BuildEnv:         packit.Environment{},
					LaunchEnv:        packit.Environment{},
					ProcessLaunchEnv: map[string]packit.Environment{},
					Build:            true,
					Launch:           true,
					Cache:            false,
					Metadata: map[string]interface{}{
						"dependency-sha": "some-sha",
					},
				},
			},
		}))

		binary := filepath.Join(layersDir, "composer", "bin", dependency.Name)
		Expect(binary).To(BeARegularFile())

		stat, err := os.Stat(binary)
		Expect(err).NotTo(HaveOccurred())
		Expect(stat.Mode()).To(Equal(os.FileMode(0755)))

		Expect(dependencyManager.DeliverCall.Receives.Dependency).To(Equal(dependency))
		Expect(dependencyManager.DeliverCall.Receives.CnbPath).To(Equal(cnbDir))
		Expect(dependencyManager.DeliverCall.Receives.LayerPath).To(Equal(filepath.Join(layersDir, "composer", "bin")))
		Expect(dependencyManager.DeliverCall.Receives.PlatformPath).To(Equal("platform"))
	})

	context("with build=true and launch=false", func() {
		it.Before(func() {
			buildpackPlan = packit.BuildpackPlan{
				Entries: []packit.BuildpackPlanEntry{
					{
						Name: "composer",
						Metadata: map[string]interface{}{
							"build": true,
						},
					},
				},
			}
		})

		it("sets the layer flags appropriately", func() {
			result, err := build(packit.BuildContext{
				WorkingDir: workingDir,
				CNBPath:    cnbDir,
				Stack:      "some-stack",
				BuildpackInfo: packit.BuildpackInfo{
					Name:    "Some Buildpack",
					Version: "some-version",
				},
				Platform: packit.Platform{Path: "platform"},
				Plan:     buildpackPlan,
				Layers:   packit.Layers{Path: layersDir},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(result).To(Equal(packit.BuildResult{
				Layers: []packit.Layer{
					{
						Name:             "composer",
						Path:             filepath.Join(layersDir, "composer"),
						SharedEnv:        packit.Environment{},
						BuildEnv:         packit.Environment{},
						LaunchEnv:        packit.Environment{},
						ProcessLaunchEnv: map[string]packit.Environment{},
						Build:            true,
						Launch:           false,
						Cache:            false,
						Metadata: map[string]interface{}{
							"dependency-sha": "some-sha",
						},
					},
				},
			}))
		})
	})

	context("with build=false and launch=true", func() {
		it.Before(func() {
			buildpackPlan = packit.BuildpackPlan{
				Entries: []packit.BuildpackPlanEntry{
					{
						Name: "composer",
						Metadata: map[string]interface{}{
							"launch": true,
						},
					},
				},
			}
		})

		it("sets the layer flags appropriately", func() {
			result, err := build(packit.BuildContext{
				WorkingDir: workingDir,
				CNBPath:    cnbDir,
				Stack:      "some-stack",
				BuildpackInfo: packit.BuildpackInfo{
					Name:    "Some Buildpack",
					Version: "some-version",
				},
				Platform: packit.Platform{Path: "platform"},
				Plan:     buildpackPlan,
				Layers:   packit.Layers{Path: layersDir},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(result).To(Equal(packit.BuildResult{
				Layers: []packit.Layer{
					{
						Name:             "composer",
						Path:             filepath.Join(layersDir, "composer"),
						SharedEnv:        packit.Environment{},
						BuildEnv:         packit.Environment{},
						LaunchEnv:        packit.Environment{},
						ProcessLaunchEnv: map[string]packit.Environment{},
						Build:            false,
						Launch:           true,
						Cache:            false,
						Metadata: map[string]interface{}{
							"dependency-sha": "some-sha",
						},
					},
				},
			}))
		})
	})

	context("with build missing and launch missing", func() {
		it.Before(func() {
			buildpackPlan = packit.BuildpackPlan{
				Entries: []packit.BuildpackPlanEntry{
					{
						Name: "composer",
					},
				},
			}
		})

		it("will contribute to the build phase only", func() {
			result, err := build(packit.BuildContext{
				WorkingDir: workingDir,
				CNBPath:    cnbDir,
				Stack:      "some-stack",
				BuildpackInfo: packit.BuildpackInfo{
					Name:    "Some Buildpack",
					Version: "some-version",
				},
				Platform: packit.Platform{Path: "platform"},
				Plan:     buildpackPlan,
				Layers:   packit.Layers{Path: layersDir},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(result).To(Equal(packit.BuildResult{
				Layers: []packit.Layer{
					{
						Name:             "composer",
						Path:             filepath.Join(layersDir, "composer"),
						SharedEnv:        packit.Environment{},
						BuildEnv:         packit.Environment{},
						LaunchEnv:        packit.Environment{},
						ProcessLaunchEnv: map[string]packit.Environment{},
						Build:            true,
						Launch:           false,
						Cache:            false,
						Metadata: map[string]interface{}{
							"dependency-sha": "some-sha",
						},
					},
				},
			}))
		})
	})

}
