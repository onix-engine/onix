//go:build android || ios

package onix

func appRunOwnsLifecycle() bool { return false }
