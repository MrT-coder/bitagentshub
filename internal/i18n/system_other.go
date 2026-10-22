//go:build !windows

package i18n

// uiLanguages has nothing beyond the locale variables outside Windows.
func uiLanguages() []string { return nil }
