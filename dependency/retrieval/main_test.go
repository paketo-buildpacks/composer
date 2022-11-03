package main_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/joshuatcasey/libdependency/versionology"
	"github.com/paketo-buildpacks/packit/v2/cargo"
	"github.com/sclevine/spec"

	. "github.com/onsi/gomega"

	"github.com/paketo-buildpacks/occam/matchers"
	"github.com/paketo-buildpacks/pipenv/retrieval"
)

func testRetrieval(t *testing.T, context spec.G, it spec.S) {
	var (
		Expect = NewWithT(t).Expect
	)

	context("GenerateMetadata", func() {
		it("will generate metadata for a valid version", func() {
			metadata, err := main.GenerateMetadata(versionology.NewSimpleVersionFetcher(semver.MustParse("2.4.4")))
			Expect(err).NotTo(HaveOccurred())

			Expect(metadata).To(ConsistOf(versionology.Dependency{
				ConfigMetadataDependency: cargo.ConfigMetadataDependency{
					Checksum:       "sha256:c252c2a2219956f88089ffc242b42c8cb9300a368fd3890d63940e4fc9652345",
					CPE:            "cpe:2.3:a:getcomposer:composer:2.4.4:*:*:*:*:python:*:*",
					PURL:           "pkg:generic/composer@2.4.4?checksum=c252c2a2219956f88089ffc242b42c8cb9300a368fd3890d63940e4fc9652345&download_url=https://getcomposer.org/download/2.4.4/composer.phar",
					ID:             "composer",
					Licenses:       []interface{}{"MIT"},
					Name:           "composer",
					Source:         "https://getcomposer.org/download/2.4.4/composer.phar",
					SourceChecksum: "sha256:c252c2a2219956f88089ffc242b42c8cb9300a368fd3890d63940e4fc9652345",
					Stacks:         []string{"*"},
					URI:            "https://getcomposer.org/download/2.4.4/composer.phar",
					Version:        "2.4.4",
				},
				SemverVersion: semver.MustParse("2.4.4"),
				Target:        "NONE",
			}))
		})
	})

	context("PharDecompress", func() {
		var (
			pharPath, destination string

			err error
		)

		it.Before(func() {
			pharPath = filepath.Join("testdata", "phar", "composer-2.4.4.phar")
			destination = t.TempDir()
		})

		it("will decompress a phar", func() {
			var pharBytes []byte

			pharBytes, err = os.ReadFile(pharPath)
			Expect(err).NotTo(HaveOccurred())

			err = main.PharDecompress(bytes.NewReader(pharBytes), destination)
			Expect(err).NotTo(HaveOccurred())

			Expect(filepath.Join(destination, "LICENSE")).
				To(matchers.BeAFileMatching(ContainSubstring("Copyright (c) Nils Adermann, Jordi Boggiano")))
		})
	})
}

func ExamplePharDecompress_printsNothingOnSuccess() {
	pharPath := filepath.Join("testdata", "phar", "composer-2.4.4.phar")
	destination, err := os.MkdirTemp("", "")

	defer func() {
		err := os.RemoveAll(destination)
		if err != nil {
			fmt.Println(err)
		}
	}()

	pharBytes, err := os.ReadFile(pharPath)
	if err != nil {
		fmt.Println(err)
	}

	err = main.PharDecompress(bytes.NewReader(pharBytes), destination)
	if err != nil {
		fmt.Println(err)
	}

	// Output:
}
