package terminal

import (
	"context"
	"testing"
)

func TestRequestActorIDUsesAuthenticatedPrincipal(t *testing.T) {
	deps := &Deps{
		PrincipalActorID: func(context.Context) string { return "  apikey:key-a  " },
	}
	if got := deps.requestActorID(context.Background()); got != "apikey:key-a" {
		t.Fatalf("requestActorID() = %q, want apikey:key-a", got)
	}
}

func TestRequestActorIDFailsClosedToSystem(t *testing.T) {
	for _, deps := range []*Deps{nil, {}} {
		if got := deps.requestActorID(context.Background()); got != "system" {
			t.Fatalf("requestActorID() = %q, want system", got)
		}
	}
}
