package notification

import (
	"context"
	"testing"
)

func TestADefinitionIsDottedAndItsKeysAreKeys(t *testing.T) {
	ok := Definition{Type: "webhook.delivery_failed", TitleKey: "notification.webhook.delivery_failed.title", BodyKey: "notification.webhook.delivery_failed.body"}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]Definition{
		"no dot":           {Type: "failed", TitleKey: "a.b", BodyKey: "a.b"},
		"upper case":       {Type: "webhook.Failed", TitleKey: "a.b", BodyKey: "a.b"},
		"a sentence title": {Type: "webhook.failed", TitleKey: "Delivery failed.", BodyKey: "a.b"},
		"an empty body":    {Type: "webhook.failed", TitleKey: "a.b"},
	} {
		if bad.Validate() == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
}

func TestTheRecorderKeepsBothAudiences(t *testing.T) {
	r := &Recorder{}
	ctx := context.Background()
	_ = r.ToUser(ctx, "u1", "webhook.delivery_failed", map[string]any{"attempts": 5})
	_ = r.ToTenant(ctx, "t1", "webhook.delivery_failed", nil)
	s := r.Sent()
	if len(s) != 2 || s[0].To != "user:u1" || s[0].Context["attempts"] != 5 || s[1].To != "tenant:t1" {
		t.Fatalf("%+v", s)
	}
}
