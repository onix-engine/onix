package graphics

import "testing"

func TestResourceUsage(t *testing.T) {
	resource := NewResource([]byte{1, 2, 3}, ResourceUsageGPU)
	if resource.Usage.Has(ResourceUsageCPU) || !resource.Usage.Has(ResourceUsageGPU) {
		t.Fatalf("unexpected resource usage %b", resource.Usage)
	}
	if !ResourceUsageDefault.Has(ResourceUsageCPU | ResourceUsageGPU) {
		t.Fatal("default resource usage must retain CPU and GPU data")
	}
}
