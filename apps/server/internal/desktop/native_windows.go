//go:build windows

package desktop

import "github.com/jchv/go-webview2"

func openNative(url, profile string) bool {
	var w webview2.WebView
	func() {
		defer func() { _ = recover() }()
		w = webview2.NewWithOptions(webview2.WebViewOptions{
			Debug:     false,
			AutoFocus: true,
			DataPath:  profile,
			WindowOptions: webview2.WindowOptions{
				Title:  "Harbor",
				Width:  1280,
				Height: 800,
				Center: true,
			},
		})
	}()
	if w == nil {
		return false
	}
	defer w.Destroy()
	w.Navigate(url)
	w.Run()
	return true
}
