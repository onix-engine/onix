package window

import "testing"

func TestCallbackChainsPreserveRegistrationOrder(t *testing.T) {
	first, second, third := 0, 0, 0
	var callback func(int)
	callback = chainCallback(callback, func(value int) { first += value })
	callback = chainCallback(callback, func(value int) { second += value * 2 })
	callback = chainCallback(callback, func(value int) { third += value * 3 })
	callback(4)
	if first != 4 || second != 8 || third != 12 {
		t.Fatalf("unexpected callback results: %d, %d, %d", first, second, third)
	}
}

func TestCallbackChainsHandleNilHandlers(t *testing.T) {
	called := false
	callback := chainCallback[int](nil, nil)
	callback = chainCallback(callback, func(int) { called = true })
	callback(0)
	if !called {
		t.Fatal("non-nil callback was not retained")
	}
}

func TestFileDropCallbackChainPreservesRegistrationOrder(t *testing.T) {
	called := make([]string, 0, 2)
	var callback func(float64, float64, []string)
	callback = chainCallback3(callback, func(x, y float64, files []string) {
		called = append(called, files[0])
		if x != 3 || y != 4 {
			t.Fatalf("unexpected drop position: %.1f, %.1f", x, y)
		}
	})
	callback = chainCallback3(callback, func(_ float64, _ float64, files []string) {
		called = append(called, files[0]+"-second")
	})
	callback(3, 4, []string{"example.txt"})
	if len(called) != 2 || called[0] != "example.txt" || called[1] != "example.txt-second" {
		t.Fatalf("unexpected callback order: %v", called)
	}
}
