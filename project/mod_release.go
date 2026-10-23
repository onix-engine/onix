//go:build onix_release

package project

import (
	"fmt"
	"io/fs"
)

func openModuleAuthoringProject() (*ProjectPlan, fs.FS, error) {
	return nil, nil, fmt.Errorf("release module projects require a mounted binary resource package")
}
