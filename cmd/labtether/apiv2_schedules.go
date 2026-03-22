package main

import "net/http"

func (s *apiServer) handleV2Schedules(w http.ResponseWriter, r *http.Request) {
	s.ensureSchedulesDeps().HandleV2Schedules(w, r)
}

func (s *apiServer) handleV2ScheduleActions(w http.ResponseWriter, r *http.Request) {
	s.ensureSchedulesDeps().HandleV2ScheduleActions(w, r)
}
