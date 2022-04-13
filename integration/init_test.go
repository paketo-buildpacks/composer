package integration_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/paketo-buildpacks/occam"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
)

var buildpackInfo struct {
	Buildpack struct {
		ID     string
		Name   string
		PackId string
	}
	Metadata struct {
		Dependencies []struct {
			Version string
		}
	}
}

var buildpacks struct {
	BuildPlan       string
	PhpDist         string
	PhpDistOffline  string
	Composer        string
	ComposerOffline string
}

var integration struct {
	BuildPlan string `json:"build-plan"`
	PhpDist   string `json:"php-dist"`
}

func TestIntegration(t *testing.T) {
	// Do not truncate Gomega matcher output
	// The buildpack output text can be large and we often want to see all of it.
	format.MaxLength = 0

	Expect := NewWithT(t).Expect

	file, err := os.Open("../integration.json")
	Expect(err).NotTo(HaveOccurred())

	Expect(json.NewDecoder(file).Decode(&integration)).To(Succeed())
	Expect(file.Close()).To(Succeed())

	file, err = os.Open("../buildpack.toml")
	Expect(err).NotTo(HaveOccurred())

	_, err = toml.NewDecoder(file).Decode(&buildpackInfo)
	Expect(err).NotTo(HaveOccurred())

	buildpackInfo.Buildpack.PackId = strings.ReplaceAll(buildpackInfo.Buildpack.ID, "/", "_")

	buildpackStore := occam.NewBuildpackStore()

	buildpacks.BuildPlan, err = buildpackStore.Get.
		Execute(integration.BuildPlan)
	Expect(err).NotTo(HaveOccurred())

	buildpacks.PhpDist, err = buildpackStore.Get.
		Execute(integration.PhpDist)
	Expect(err).NotTo(HaveOccurred())

	buildpacks.PhpDistOffline, err = buildpackStore.Get.
		WithOfflineDependencies().
		Execute(integration.PhpDist)
	Expect(err).NotTo(HaveOccurred())

	root, err := filepath.Abs("./..")
	Expect(err).ToNot(HaveOccurred())

	buildpacks.Composer, err = buildpackStore.Get.
		WithVersion("1.2.3").
		Execute(root)
	Expect(err).NotTo(HaveOccurred())

	buildpacks.ComposerOffline, err = buildpackStore.Get.
		WithOfflineDependencies().
		WithVersion("1.2.3").
		Execute(root)
	Expect(err).NotTo(HaveOccurred())

	SetDefaultEventuallyTimeout(5 * time.Second)

	suite := spec.New("Integration", spec.Report(report.Terminal{}))
	suite("BuildAndLaunch", testDefaultApp, spec.Parallel())
	suite("LayerReuse", testReusingLayerRebuild)
	suite("Offline", testOffline)
	suite("SBOM", testSbom)
	suite.Run(t)
}
