package render

import "testing"

func TestTargetsKeepDeviceResidencyAndIndependentResize(t *testing.T) {
	firstDriver, secondDriver := &renderTargetTestDriver{}, &renderTargetTestDriver{}
	first, _ := NewDevice(firstDriver)
	second, _ := NewDevice(secondDriver)
	defer first.Destroy()
	defer second.Destroy()
	target, err := NewRenderTarget(RenderTargetDescriptor{Width: 16, Height: 16})
	if err != nil {
		t.Fatal(err)
	}
	firstColor, err := target.ColorTextureFor(first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := target.ColorTextureFor(second); err != nil {
		t.Fatal(err)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		color, err := target.ColorTextureFor(first)
		if err != nil || color != firstColor {
			t.Fatal("first target lost residency")
		}
		if _, err := target.ColorTextureFor(second); err != nil {
			t.Fatal(err)
		}
	}); allocations != 0 {
		t.Fatalf("target switches allocations=%v", allocations)
	}
	if firstDriver.created != 1 || secondDriver.created != 1 {
		t.Fatal("switch recreated target")
	}
	target.Resize(32, 32)
	if err := target.BeginFor(first, RenderTargetClear{}); err != nil {
		t.Fatal(err)
	}
	second.Activate()
	if err := target.End(); err != nil {
		t.Fatal(err)
	}
	if firstDriver.resized != 1 || secondDriver.resized != 0 || firstDriver.ends != 1 || secondDriver.ends != 0 {
		t.Fatal("target operations used wrong device")
	}
	if _, err := target.ColorTextureFor(second); err != nil {
		t.Fatal(err)
	}
	first.Destroy()
	if err := target.Dispose(); err != nil {
		t.Fatal(err)
	}
	if firstDriver.destroyed != 0 || secondDriver.destroyed != 1 {
		t.Fatal("target dispose used a destroyed device")
	}
}

func TestStatisticsRemainOnTheirDevice(t *testing.T) {
	firstDriver := &statisticsTestDriver{}
	secondDriver := &statisticsTestDriver{}
	first, _ := NewDevice(firstDriver)
	second, _ := NewDevice(secondDriver)
	defer first.Destroy()
	defer second.Destroy()
	second.Activate()
	firstDriver.beginFrameStatistics(100, 50)
	firstDriver.recordTextureUploadStatistics(1, 64)
	firstDriver.recordShapeStatistics(ShapeBatch{Instances: make([]ShapeInstance2D, 2)})
	if got := first.Statistics(); got.Shapes != 2 || got.TextureUploads != 1 {
		t.Fatalf("first statistics=%+v", got)
	}
	if got := second.Statistics(); got != (FrameStatistics{}) {
		t.Fatalf("second statistics=%+v", got)
	}
	if err := firstDriver.Present(); err != nil {
		t.Fatal(err)
	}
	second.Destroy()
	if first.Driver() != firstDriver {
		t.Fatal("destroying active device destroyed inactive one")
	}
}

type statisticsTestDriver struct {
	renderTargetTestDriver
	driverStatistics
}

func TestDefaultDeviceReplacementPreservesIndependentOwnership(t *testing.T) {
	Uninstall()
	independentDriver := &deviceLifecycleTestDriver{}
	independent, _ := NewDevice(independentDriver)
	defer independent.Destroy()
	independent.Activate()
	first := &deviceLifecycleTestDriver{}
	if err := Install(first); err != nil {
		t.Fatal(err)
	}
	if err := Install(first); err != nil {
		t.Fatal(err)
	}
	if first.destroyedDevice != 0 {
		t.Fatal("reinstalling the same driver destroyed it")
	}
	second := &deviceLifecycleTestDriver{}
	if err := Install(second); err != nil {
		t.Fatal(err)
	}
	if first.destroyedDevice != 1 || independentDriver.destroyedDevice != 0 {
		t.Fatal("default replacement destroyed wrong owner")
	}
	independent.Activate()
	Uninstall()
	if second.destroyedDevice != 1 || independentDriver.destroyedDevice != 0 || Current() != independentDriver {
		t.Fatal("default uninstall destroyed active independent device")
	}
}

type deviceLifecycleTestDriver struct {
	renderTargetTestDriver
	destroyedDevice int
}

func (driver *deviceLifecycleTestDriver) Destroy() { driver.destroyedDevice++ }
