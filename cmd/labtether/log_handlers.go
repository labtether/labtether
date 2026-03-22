package main

import "net/http"

func (s *apiServer) handleLogsQuery(w http.ResponseWriter, r *http.Request) {
	s.ensureLogsDeps().HandleLogsQuery(w, r)
}

func (s *apiServer) handleLogSources(w http.ResponseWriter, r *http.Request) {
	s.ensureLogsDeps().HandleLogSources(w, r)
}

func (s *apiServer) handleLogViews(w http.ResponseWriter, r *http.Request) {
	s.ensureLogsDeps().HandleLogViews(w, r)
}

func (s *apiServer) handleLogViewActions(w http.ResponseWriter, r *http.Request) {
	s.ensureLogsDeps().HandleLogViewActions(w, r)
}
