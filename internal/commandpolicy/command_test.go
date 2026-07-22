package commandpolicy

import (
	"reflect"
	"testing"
)

func TestParseArgvRejectsShellSyntax(t *testing.T) {
	for _, command := range []string{
		"uptime; id",
		"uptime && id",
		"cat /etc/passwd | head",
		"echo $(id)",
		"echo `id`",
		"uname\nwhoami",
	} {
		if _, err := ParseArgv(command); err == nil {
			t.Fatalf("expected %q to be rejected", command)
		}
	}
}

func TestParseArgvPreservesLiteralArguments(t *testing.T) {
	got, err := ParseArgv(`grep "hello world" 'file name.txt'`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"grep", "hello world", "file name.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseArgv = %#v, want %#v", got, want)
	}
}

func TestMatchesRuleUsesTokenBoundaries(t *testing.T) {
	argv, _ := ParseArgv("systemctl status nginx")
	if !MatchesRule(argv, "systemctl status") {
		t.Fatal("expected exact subcommand rule to match")
	}
	for _, command := range []string{"systemctl restart nginx", "systemctl-status nginx", "uptime-extra"} {
		candidate, _ := ParseArgv(command)
		if MatchesRule(candidate, "systemctl status") || MatchesRule(candidate, "uptime") {
			t.Fatalf("unexpected rule match for %q", command)
		}
	}
}

func TestQuoteArgvNeutralizesRemoteShellSyntax(t *testing.T) {
	got := QuoteArgv([]string{"grep", "a'; id; echo 'b", "file name"})
	want := `'grep' 'a'"'"'; id; echo '"'"'b' 'file name'`
	if got != want {
		t.Fatalf("QuoteArgv = %q, want %q", got, want)
	}
}
