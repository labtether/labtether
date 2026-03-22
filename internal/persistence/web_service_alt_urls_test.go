package persistence

import "testing"

func TestAltURLTypes(t *testing.T) {
	entry := WebServiceAltURL{
		ID: "test-1", WebServiceID: "svc-abc",
		URL: "http://example.com", Source: "auto",
	}
	if entry.WebServiceID != "svc-abc" {
		t.Fatalf("expected svc-abc, got %s", entry.WebServiceID)
	}

	rule := WebServiceNeverGroupRule{
		ID: "rule-1", URLA: "http://a.com", URLB: "http://b.com",
	}
	if rule.URLA != "http://a.com" {
		t.Fatalf("expected http://a.com, got %s", rule.URLA)
	}

	setting := WebServiceURLGroupingSetting{
		ID: "s-1", SettingKey: "mode", SettingValue: "balanced",
	}
	if setting.SettingKey != "mode" {
		t.Fatalf("expected mode, got %s", setting.SettingKey)
	}
}
