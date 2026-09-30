package main

import "liforra-tool/internal/localsettings"

func (a *App) GetAdministrator() bool {
	s, _ := localsettings.Load()
	return s.Administrator
}

func (a *App) SetAdministrator(enabled bool) error {
	s, _ := localsettings.Load()
	s.Administrator = enabled
	return localsettings.Save(s)
}

func (a *App) GetBetterPDFNames() bool {
	s, _ := localsettings.Load()
	return s.BetterPDFNames
}

func (a *App) SetBetterPDFNames(enabled bool) error {
	s, _ := localsettings.Load()
	s.BetterPDFNames = enabled
	return localsettings.Save(s)
}
