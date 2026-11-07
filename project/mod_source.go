//go:build !onix_release

package project

import (
	"io/fs"

	"github.com/onix-engine/onix/resource"
)

func openModuleAuthoringProject() (*ProjectPlan, fs.FS, error) {
	source := resource.OpenDirectory(".")
	plan, err := CompileProject(source, ".")
	return plan, source, err
}
