package server

import "net/http"

const settingThemeKey = "theme"

type settingsView struct {
	Theme              string `json:"theme"`
	SessionIdleSeconds int    `json:"sessionIdleSeconds"`
	SessionMaxSeconds  int    `json:"sessionMaxSeconds"`
	Dev                bool   `json:"dev"`
	Version            string `json:"version"`
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	theme := "dark"
	if v, ok, _ := s.store.GetSetting(settingThemeKey); ok {
		theme = v
	}
	writeJSON(w, http.StatusOK, settingsView{
		Theme:              theme,
		SessionIdleSeconds: int(s.cfg.SessionIdle.Seconds()),
		SessionMaxSeconds:  int(s.cfg.SessionMax.Seconds()),
		Dev:                s.cfg.Dev,
		Version:            Version,
	})
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Theme string `json:"theme"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Theme != "" {
		if req.Theme != "dark" && req.Theme != "light" {
			writeError(w, http.StatusBadRequest, "theme must be 'dark' or 'light'")
			return
		}
		if err := s.store.SetSetting(settingThemeKey, req.Theme); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save settings")
			return
		}
	}
	s.handleGetSettings(w, r)
}
