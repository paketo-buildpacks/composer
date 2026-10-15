package composer_test

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/paketo-buildpacks/composer"
	"github.com/paketo-buildpacks/composer/fakes"
	"github.com/paketo-buildpacks/packit/v2"
	"github.com/paketo-buildpacks/packit/v2/fs"
	"github.com/paketo-buildpacks/packit/v2/postal"
	"github.com/paketo-buildpacks/packit/v2/sbom"
	"github.com/paketo-buildpacks/packit/v2/scribe"
	"github.com/sclevine/spec"
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
		sbomGenerator     *fakes.SBOMGenerator

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
		sbomGenerator = &fakes.SBOMGenerator{}
		sbomGenerator.GenerateFromDependencyCall.Returns.SBOM = sbom.SBOM{}

		build = composer.Build(logEmitter, dependencyManager, sbomGenerator)

		composerArchive, err := os.CreateTemp(cnbDir, "composer-archive")
		Expect(err).NotTo(HaveOccurred())
		composerArchiveName := filepath.Base(composerArchive.Name())

		Expect(os.Chmod(composerArchive.Name(), 0777)).To(Succeed())

		dependency = postal.Dependency{
			ID:       "composer",
			Name:     composerArchiveName,
			Version:  "composer-dependency-version",
			Checksum: "some-sha",
		}

		dependencyManager.ResolveCall.Returns.Dependency = dependency
		dependencyManager.DeliverCall.Stub = func(dependency postal.Dependency, cnbPath, layerPath, _ string) error {
			return fs.Copy(filepath.Join(cnbPath, dependency.Name), filepath.Join(layerPath, dependency.Name))
		}
		dependencyManager.GenerateBillOfMaterialsCall.Returns.BOMEntrySlice = []packit.BOMEntry{
			{
				Name: "composer",
			},
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
		fixtureDependency := postal.Dependency{Name: "sbom-fixture-composer", Version: "1.2.3"}
		fixtureSBOM, err := sbom.GenerateFromDependency(fixtureDependency, filepath.Join(layersDir, "composer"))
		Expect(err).NotTo(HaveOccurred())
		sbomGenerator.GenerateFromDependencyCall.Returns.SBOM = fixtureSBOM

		result, err := build(packit.BuildContext{
			WorkingDir: workingDir,
			CNBPath:    cnbDir,
			Stack:      "some-stack",
			BuildpackInfo: packit.BuildpackInfo{
				Name:        "Some Buildpack",
				Version:     "some-version",
				SBOMFormats: []string{sbom.CycloneDXFormat, sbom.SPDXFormat},
			},
			Platform: packit.Platform{Path: "platform"},
			Plan:     buildpackPlan,
			Layers:   packit.Layers{Path: layersDir},
		})
		Expect(err).NotTo(HaveOccurred())

		expectedFormats, err := fixtureSBOM.InFormats(sbom.CycloneDXFormat, sbom.SPDXFormat)
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
					Cache:            true,
					Metadata: map[string]interface{}{
						"dependency-checksum": "some-sha",
					},
					SBOM: expectedFormats,
				},
			},
			Launch: packit.LaunchMetadata{
				BOM: []packit.BOMEntry{
					{Name: "composer"},
				},
			},
			Build: packit.BuildMetadata{
				BOM: []packit.BOMEntry{
					{Name: "composer"},
				},
			},
		}))

		Expect(buffer).To(ContainSubstring("Executing build process"))

		binary := filepath.Join(layersDir, "composer", "bin", dependency.Name)
		Expect(binary).To(BeARegularFile())

		stat, err := os.Stat(binary)
		Expect(err).NotTo(HaveOccurred())
		Expect(stat.Mode()).To(Equal(os.FileMode(0755)))

		Expect(dependencyManager.DeliverCall.Receives.Dependency).To(Equal(dependency))
		Expect(dependencyManager.DeliverCall.Receives.CnbPath).To(Equal(cnbDir))
		Expect(dependencyManager.DeliverCall.Receives.LayerPath).To(Equal(filepath.Join(layersDir, "composer", "bin")))
		Expect(dependencyManager.DeliverCall.Receives.PlatformPath).To(Equal("platform"))
		Expect(sbomGenerator.GenerateFromDependencyCall.Receives.Dependency).To(Equal(dependency))
		Expect(sbomGenerator.GenerateFromDependencyCall.Receives.Dir).To(Equal(filepath.Join(layersDir, "composer")))

		layer := result.Layers[0]
		var extensions []string
		for _, format := range layer.SBOM.Formats() {
			extensions = append(extensions, format.Extension)
			content, err := io.ReadAll(format.Content)
			Expect(err).NotTo(HaveOccurred())
			var document map[string]interface{}
			Expect(json.Unmarshal(content, &document)).To(Succeed())

			switch format.Extension {
			case "cdx.json":
				Expect(document).To(HaveKeyWithValue("components", ContainElement(And(
					HaveKeyWithValue("name", fixtureDependency.Name),
					HaveKeyWithValue("version", fixtureDependency.Version),
				))))
			case "spdx.json":
				Expect(document).To(HaveKeyWithValue("packages", ContainElement(And(
					HaveKeyWithValue("name", fixtureDependency.Name),
					HaveKeyWithValue("versionInfo", fixtureDependency.Version),
				))))
			}
		}
		Expect(extensions).To(ConsistOf("cdx.json", "spdx.json"))

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
						Cache:            true,
						Metadata: map[string]interface{}{
							"dependency-checksum": "some-sha",
						},
						SBOM: sbom.Formatter{},
					},
				},
				Build: packit.BuildMetadata{
					BOM: []packit.BOMEntry{
						{Name: "composer"},
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
							"dependency-checksum": "some-sha",
						},
						SBOM: sbom.Formatter{},
					},
				},
				Launch: packit.LaunchMetadata{
					BOM: []packit.BOMEntry{
						{Name: "composer"},
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

		it("will contribute an ignored layer", func() {
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
						Launch:           false,
						Cache:            false,
						Metadata: map[string]interface{}{
							"dependency-checksum": "some-sha",
						},
						SBOM: sbom.Formatter{},
					},
				},
				Launch: packit.LaunchMetadata{},
				Build:  packit.BuildMetadata{},
			}))
		})
	})

	context("when the layer is cached", func() {
		it.Before(func() {
			dependencyManager.ResolveCall.Returns.Dependency.Checksum = "cached-sha"

			err := os.WriteFile(filepath.Join(layersDir, "composer.toml"),
				[]byte(`[metadata]
dependency-checksum = "cached-sha"
`), os.ModePerm)
			Expect(err).NotTo(HaveOccurred())
		})

		it("reuses the cached version of the SDK dependency", func() {
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

			Expect(buffer).NotTo(ContainSubstring("Executing build process"))

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
						Cache:            true,
						Metadata: map[string]interface{}{
							"dependency-checksum": "cached-sha",
						},
					},
				},
				Launch: packit.LaunchMetadata{
					BOM: []packit.BOMEntry{
						{Name: "composer"},
					},
				},
				Build: packit.BuildMetadata{
					BOM: []packit.BOMEntry{
						{Name: "composer"},
					},
				},
			}))
		})
	})
}
