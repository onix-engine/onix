package project

import (
	"fmt"
	"io/fs"

	"github.com/diiyw/onix/internal/typedbin"
	"github.com/diiyw/onix/resource"
)

const ModulePlanFile = "resources/modules.bin"

// MarshalModulePlan is build-time work. The binary retains resolved indices,
// typed slot defaults and behavior entries for every registered scene.
func MarshalModulePlan(plan *ProjectPlan) ([]byte, error) {
	if plan == nil {
		return nil, fmt.Errorf("module project plan is nil")
	}
	return typedbin.Marshal(*plan)
}

func ReadModulePlan(source fs.FS) (*ProjectPlan, error) {
	data, err := fs.ReadFile(source, ModulePlanFile)
	if err != nil {
		return nil, fmt.Errorf("load module plan: %w", err)
	}
	plan, err := typedbin.Unmarshal[ProjectPlan](data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ModulePlanFile, err)
	}
	if int(plan.Entry) >= len(plan.Scenes) || plan.Scenes[plan.Entry].ID != plan.Spec.Entry {
		return nil, fmt.Errorf("%s: invalid entry index", ModulePlanFile)
	}
	return &plan, nil
}

// OpenModuleProject uses the mounted package source in published applications.
// Authoring builds without a package mount read the current project directory.
func OpenModuleProject() (*ProjectPlan, fs.FS, error) {
	if source, root, ok := resource.Default(); ok {
		if root != "." {
			var err error
			source, err = resource.Sub(source, root)
			if err != nil {
				return nil, nil, err
			}
		}
		plan, err := ReadModulePlan(source)
		return plan, source, err
	}
	return openModuleAuthoringProject()
}
