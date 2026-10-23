//go:build onix_release

package project

import (
	"errors"
	iofs "io/fs"
)

func PreprocessManifests(iofs.FS, PreprocessOptions) ([]PreprocessedFile, error) {
	return nil, errors.New("project preprocessing is unavailable in release builds")
}

func PreprocessModuleAssets(iofs.FS, *ProjectPlan, PreprocessOptions) ([]PreprocessedFile, error) {
	return nil, errors.New("module preprocessing is unavailable in release builds")
}

func PreprocessModShaders(iofs.FS, *ModPlan, PreprocessOptions) ([]PreprocessedFile, error) {
	return nil, errors.New("shader preprocessing is unavailable in release builds")
}
