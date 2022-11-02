package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/joshuatcasey/libdependency/github"
	"github.com/joshuatcasey/libdependency/retrieve"
	"github.com/joshuatcasey/libdependency/versionology"
	"github.com/paketo-buildpacks/packit/v2/cargo"
	"github.com/paketo-buildpacks/packit/v2/fs"
	"github.com/paketo-buildpacks/packit/v2/pexec"
	"golang.org/x/crypto/openpgp"
)

func downloadToFile(url string) (filePath string, fileContents string, err error) {
	response, err := http.DefaultClient.Get(url)
	if err != nil {
		return "", "", fmt.Errorf("could not get project metadata: %w", err)
	}

	if err != nil {
		return "", "", err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", "", fmt.Errorf("could not read response: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "")
	if err != nil {
		return "", "", errors.New("could not create a temp dir")
	}

	filePath = filepath.Join(tempDir, filepath.Base(url))

	err = os.WriteFile(filePath, body, os.ModePerm)
	if err != nil {
		return "", "", fmt.Errorf("could not write to file")
	}

	fileContents = string(body)

	return
}

func verifyASC(signature, target, pgpKey string) error {
	file, err := os.Open(target)
	if err != nil {
		return fmt.Errorf("could not open file: %w", err)
	}
	defer file.Close()

	keyring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(pgpKey))
	if err != nil {
		return err
	}
	signer, err := openpgp.CheckDetachedSignature(keyring, file, strings.NewReader(signature))
	if signer == nil {
		return fmt.Errorf("signature not accepted: %w", err)
	}
	return err
}

func generateMetadata(versionFetcher versionology.VersionFetcher) ([]versionology.Dependency, error) {
	version := versionFetcher.Version().String()

	uri := fmt.Sprintf("https://getcomposer.org/download/%s/composer.phar", version)

	filePath, _, err := downloadToFile(uri)
	if err != nil {
		return nil, fmt.Errorf("could not download %s to file", uri)
	}

	upstreamChecksumUri := fmt.Sprintf("https://getcomposer.org/download/%s/composer.phar.sha256sum", version)
	_, upstreamChecksum, err := downloadToFile(upstreamChecksumUri)
	if err != nil {
		return nil, fmt.Errorf("could not download %s to file", upstreamChecksumUri)
	}

	downloadedChecksum, err := fs.NewChecksumCalculator().Sum(filePath)
	if err != nil {
		return nil, fmt.Errorf("unable to calculate checksum of downloaded file: %w", err)
	}

	downloadedChecksum = fmt.Sprintf("%s  composer.phar", downloadedChecksum)

	upstreamChecksum = strings.TrimSpace(upstreamChecksum)
	if downloadedChecksum != "" && downloadedChecksum != upstreamChecksum {
		return nil, fmt.Errorf("checksum mismatch. Downloaded SHA256 of '%s' should match expected checksum of '%s' from '%s'",
			downloadedChecksum,
			upstreamChecksum,
			upstreamChecksumUri)
	}

	ascUri := fmt.Sprintf("https://getcomposer.org/download/%s/composer.phar.asc", version)

	_, asc, err := downloadToFile(ascUri)
	if err != nil {
		return nil, fmt.Errorf("could not download %s to file", ascUri)
	}

	err = verifyASC(asc, filePath, composerPublicKey)
	if err != nil {
		return nil, fmt.Errorf("could not verify signature: %w", err)
	}

	sha256 := strings.Split(upstreamChecksum, " ")[0]
	checksum := fmt.Sprintf("sha256:%s", sha256)

	// Download the tarball and verify the checksum
	// Download the tarball and verify the signature

	configMetadataDependency := cargo.ConfigMetadataDependency{
		CPE:            fmt.Sprintf("cpe:2.3:a:getcomposer:composer:%s:*:*:*:*:python:*:*", version),
		Checksum:       checksum,
		ID:             "composer",
		Licenses:       retrieve.LookupLicenses(uri, pharDecompress),
		Name:           "composer",
		PURL:           retrieve.GeneratePURL("composer", version, sha256, uri),
		Source:         uri,
		SourceChecksum: checksum,
		Stacks:         []string{"*"},
		URI:            uri,
		Version:        version,
	}
	return versionology.NewDependencyArray(configMetadataDependency, "NONE")
}

func pharDecompress(artifact io.Reader, destination string) error {
	destinationFile := filepath.Join(destination, "composer.phar")

	artifactBytes, err := io.ReadAll(artifact)
	if err != nil {
		return err
	}

	err = os.WriteFile(destinationFile, artifactBytes, os.ModePerm)
	if err != nil {
		return err
	}

	pharPath, err := exec.LookPath("phar")
	if err != nil {
		return err
	}

	phar := pexec.NewExecutable(pharPath)
	err = phar.Execute(pexec.Execution{
		Dir: destination,
		Args: []string{
			"extract",
			"-f", destinationFile,
		},
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
	if err != nil {
		return err
	}

	return os.Remove(destinationFile)
}

func main() {
	getAllVersions := github.GetAllVersions(os.Getenv("GIT_TOKEN"), "composer", "composer")

	retrieve.NewMetadata("composer", getAllVersions, generateMetadata)
}
