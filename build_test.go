package composer_test

import (
	"bytes"
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

		expectedFormats, err := sbom.SBOM{}.InFormats(sbom.CycloneDXFormat, sbom.SPDXFormat)
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
		Expect(layer.SBOM.Formats()).To(HaveLen(2))
		cdx := layer.SBOM.Formats()[0]
		spdx := layer.SBOM.Formats()[1]

		Expect(cdx.Extension).To(Equal("cdx.json"))
		content, err := io.ReadAll(cdx.Content)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(MatchJSON(`{
			"bomFormat": "CycloneDX",
			"components": [],
			"metadata": {
				"tools": [
					{
						"name": "syft",
						"vendor": "anchore",
						"version": "[not provided]"
					}
				]
			},
			"specVersion": "1.3",
			"version": 1
		}`))

		Expect(spdx.Extension).To(Equal("spdx.json"))
		content, err = io.ReadAll(spdx.Content)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(MatchJSON(`{
			"SPDXID": "SPDXRef-DOCUMENT",
			"creationInfo": {
				"created": "0001-01-01T00:00:00Z",
				"creators": [
					"Organization: Anchore, Inc",
					"Tool: syft-"
				],
				"licenseListVersion": "3.16"
			},
			"dataLicense": "CC0-1.0",
			"documentNamespace": "https://paketo.io/packit/unknown-source-type/unknown-88cfa225-65e0-5755-895f-c1c8f10fde76",
			"name": "unknown",
			"relationships": [
				{
					"relatedSpdxElement": "SPDXRef-DOCUMENT",
					"relationshipType": "DESCRIBES",
					"spdxElementId": "SPDXRef-DOCUMENT"
				}
			],
			"spdxVersion": "SPDX-2.2"
		}`))

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
