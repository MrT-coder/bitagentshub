package statusline

// Installed reports whether Claude Code's current status line is the command
// Install recorded. Both must agree: the state file alone survives a user who
// set another status line since, and the settings alone cannot tell
// BitAgentsHub's command from anyone else's. A missing settings file is not
// installed; settings that are not valid JSON are an error.
func Installed(settingsPath, stateDir string) (bool, error) {
	pairs, _, _, err := readSettings(settingsPath)
	if err != nil {
		return false, err
	}
	current, has := get(pairs, key)
	if !has {
		return false, nil
	}
	st, installed, err := readState(stateDir)
	if err != nil || !installed {
		return false, err
	}
	return runs(current, st.Command), nil
}
