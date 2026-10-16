//go:build windows

package desktop

import "golang.org/x/sys/windows"

func Alert(title, text string) {
	_, _ = windows.MessageBox(0, windows.StringToUTF16Ptr(text), windows.StringToUTF16Ptr(title), windows.MB_OK|windows.MB_ICONERROR)
}

func Info(title, text string) {
	_, _ = windows.MessageBox(0, windows.StringToUTF16Ptr(text), windows.StringToUTF16Ptr(title), windows.MB_OK|windows.MB_ICONINFORMATION)
}
