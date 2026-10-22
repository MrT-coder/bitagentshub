package i18n

import "golang.org/x/sys/windows"

// uiLanguages are the user's display languages, preferred first.
func uiLanguages() []string {
	names, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil {
		return nil
	}
	return names
}
