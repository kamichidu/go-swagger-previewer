package main

import (
	"archive/zip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"path"
	"path/filepath"

	log "github.com/sirupsen/logrus"
)

func downloadSwaggerUI(dir string) error {
	archiveURL, err := getLatestReleaseArchiveURL()
	if err != nil {
		return err
	}
	log.Infof("latest github release url: %s", archiveURL)

	archiveFilename := filepath.Join(dir, path.Base(archiveURL))
	if _, err := os.Stat(archiveFilename); err != nil {
		log.Info("downloading archive from github")

		resp, err := http.Get(archiveURL)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if err := os.MkdirAll(dir, os.ModeDir); err != nil {
			return err
		}

		wc, err := os.Create(archiveFilename)
		if err != nil {
			return err
		}
		defer wc.Close()

		if _, err := io.Copy(wc, resp.Body); err != nil {
			return err
		}
	}

	zrc, err := zip.OpenReader(archiveFilename)
	if err != nil {
		return err
	}
	defer zrc.Close()

	distDir := filepath.Join(dir, "dist")
	if _, err := os.Stat(distDir); err != nil {
		log.Infof("creating directory: %s", distDir)
		if err := os.MkdirAll(distDir, os.ModeDir); err != nil {
			return err
		}
	}

	// we need only dist/
	for _, zf := range zrc.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		if filepath.Base(filepath.Dir(zf.Name)) != "dist" {
			continue
		}
		localFilename := filepath.Join(distDir, filepath.Base(zf.Name))
		if _, err := os.Stat(localFilename); err == nil {
			log.Infof("file already exists: %s", localFilename)
			continue
		}

		log.Infof("extracting %s", zf.Name)

		func(rc io.ReadCloser, err error) {
			if err != nil {
				log.Error(err)
				return
			}
			defer rc.Close()

			wc, err := os.Create(filepath.Join(distDir, filepath.Base(zf.Name)))
			if err != nil {
				log.Error(err)
				return
			}
			defer wc.Close()

			if _, err := io.Copy(wc, rc); err != nil {
				log.Error(err)
				return
			}
		}(zf.Open())
	}

	return nil
}

type githubRelease struct {
	TagName string `json:"tag_name"`

	ZipballURL string `json:"zipball_url"`
}

func getLatestReleaseArchiveURL() (string, error) {
	resp, err := http.Get("https://api.github.com/repos/swagger-api/swagger-ui/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, err := httputil.DumpResponse(resp, true)
		if err != nil {
			log.Panic(err)
		}
		log.Fatal(string(b))
	}

	info := new(githubRelease)
	if err := json.NewDecoder(resp.Body).Decode(info); err != nil {
		return "", err
	}
	return info.ZipballURL, nil
}
