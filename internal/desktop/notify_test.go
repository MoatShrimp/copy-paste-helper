package desktop

import (
	"runtime"
	"testing"
)

func TestNotifyUsesConfiguredNotifier(t *testing.T) {
	defer SetNotifier(nil)
	defer SetNotificationsMuted(false)

	var gotTitle, gotBody string
	SetNotifier(func(title, body string) {
		gotTitle, gotBody = title, body
	})

	Notify("Title", "Body")
	if gotTitle != "Title" || gotBody != "Body" {
		t.Fatalf("notification = (%q, %q), want (%q, %q)", gotTitle, gotBody, "Title", "Body")
	}

	SetNotificationsMuted(true)
	Notify("Muted", "Notification")
	if gotTitle != "Title" || gotBody != "Body" {
		t.Fatalf("muted notification reached notifier: (%q, %q)", gotTitle, gotBody)
	}
}

func TestNotificationBodyMatchesPlatformCapabilities(t *testing.T) {
	input := "<b>A &amp; B</b>"
	want := input
	if runtime.GOOS == "windows" {
		want = "A & B"
	}
	if got := notificationBody(input); got != want {
		t.Fatalf("notificationBody(%q) = %q, want %q", input, got, want)
	}
}
