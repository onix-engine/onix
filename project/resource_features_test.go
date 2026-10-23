package project

import "testing"

var resourceFeatureTestCalls [2]uint8
var resourceFeatureTestCallCount int

type resourceFeatureTestFirst struct{}

func (resourceFeatureTestFirst) ConfigureResources(ResourceLoadContext, ResourceEntries) error {
	resourceFeatureTestCalls[resourceFeatureTestCallCount] = 1
	resourceFeatureTestCallCount++
	return nil
}

type resourceFeatureTestSecond struct{}

func (resourceFeatureTestSecond) ConfigureResources(ResourceLoadContext, ResourceEntries) error {
	resourceFeatureTestCalls[resourceFeatureTestCallCount] = 2
	resourceFeatureTestCallCount++
	return nil
}

func TestResourceFeatureSetConfiguresFeaturesInOrder(t *testing.T) {
	resourceFeatureTestCallCount = 0
	feature := ResourceFeatureSet[resourceFeatureTestFirst, resourceFeatureTestSecond]{}
	if err := feature.ConfigureResources(ResourceLoadContext{}, ResourceEntries{}); err != nil {
		t.Fatal(err)
	}
	if resourceFeatureTestCallCount != 2 || resourceFeatureTestCalls != [2]uint8{1, 2} {
		t.Fatalf("feature calls = %v count=%d", resourceFeatureTestCalls, resourceFeatureTestCallCount)
	}
}

func TestResourceFeatureSetSkipsEmptyInterfaceWithoutAllocation(t *testing.T) {
	resourceFeatureTestCallCount = 0
	feature := ResourceFeatureSet[ResourceFeature, resourceFeatureTestSecond]{}
	if allocations := testing.AllocsPerRun(100, func() {
		resourceFeatureTestCallCount = 0
		_ = feature.ConfigureResources(ResourceLoadContext{}, ResourceEntries{})
	}); allocations != 0 {
		t.Fatalf("resource feature composition allocated %.0f objects", allocations)
	}
	if resourceFeatureTestCallCount != 1 || resourceFeatureTestCalls[0] != 2 {
		t.Fatalf("feature calls = %v count=%d", resourceFeatureTestCalls, resourceFeatureTestCallCount)
	}
}
