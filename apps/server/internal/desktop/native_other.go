//go:build !windows

package desktop

func openNative(string, string) bool { return false }
