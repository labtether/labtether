package remoteaccess

import "testing"

func TestSessionPreferenceScorePrefersUserSessionOverGreeter(t *testing.T) {
	userScore := SessionPreferenceScore(DesktopSessionInfo{
		Type:    DesktopSessionTypeX11,
		Class:   "user",
		State:   "active",
		Active:  true,
		Display: ":0",
	})
	greeterScore := SessionPreferenceScore(DesktopSessionInfo{
		Type:    DesktopSessionTypeX11,
		Class:   "greeter",
		State:   "active",
		Active:  true,
		Display: ":1",
	})

	if userScore <= greeterScore {
		t.Fatalf("expected user session score %d to exceed greeter score %d", userScore, greeterScore)
	}
}
