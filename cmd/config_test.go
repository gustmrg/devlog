package cmd

import "testing"

func TestCanonicalConfigKeyIsCaseInsensitive(t *testing.T) {
	got, err := canonicalConfigKey("github.tokenEnvVar")
	if err != nil {
		t.Fatal(err)
	}
	if got != "github.tokenenvvar" {
		t.Fatalf("key = %q, want github.tokenenvvar", got)
	}
}

func TestCanonicalConfigKeyRejectsUnknownKey(t *testing.T) {
	if _, err := canonicalConfigKey("github.token"); err == nil {
		t.Fatal("unknown key was accepted")
	}
}

func TestParseConfigBool(t *testing.T) {
	got, err := parseConfigValue("llm.enabled", "true")
	if err != nil {
		t.Fatal(err)
	}
	if got != true {
		t.Fatalf("value = %#v, want true", got)
	}
	if _, err := parseConfigValue("llm.enabled", "yes"); err == nil {
		t.Fatal("invalid boolean was accepted")
	}
}
