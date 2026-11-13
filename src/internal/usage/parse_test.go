package usage

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func mustParse(t *testing.T, body string) Snapshot {
	t.Helper()
	s, err := Parse([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestParseAssumedFixture(t *testing.T) {
	body, err := os.ReadFile("testdata/usage_assumed.json")
	if err != nil {
		t.Fatal(err)
	}
	s := mustParse(t, string(body))
	if s.FiveHour == nil || *s.FiveHour != (Window{UsedPercent: 42, ResetsAt: 1794387600}) {
		t.Fatalf("five_hour %+v", s.FiveHour)
	}
	if s.SevenDay == nil || *s.SevenDay != (Window{UsedPercent: 18, ResetsAt: 1794794400}) {
		t.Fatalf("seven_day %+v", s.SevenDay)
	}
}

func TestParseRoundsAndClamps(t *testing.T) {
	cases := map[string]int{"41.6": 42, "104.2": 100, "-3": 0, "0": 0, "100": 100}
	for in, want := range cases {
		s := mustParse(t, `{"five_hour":{"utilization":`+in+`}}`)
		if s.FiveHour == nil || s.FiveHour.UsedPercent != want {
			t.Errorf("%s: got %+v want %d", in, s.FiveHour, want)
		}
	}
}

func TestParseNullResetsAt(t *testing.T) {
	s := mustParse(t, `{"five_hour":{"utilization":7,"resets_at":null}}`)
	if s.FiveHour == nil || *s.FiveHour != (Window{UsedPercent: 7}) {
		t.Fatalf("got %+v", s.FiveHour)
	}
	out, _ := json.Marshal(s)
	if strings.Contains(string(out), "resets_at") {
		t.Fatalf("resets_at should be omitted: %s", out)
	}
}

func TestParseMissingWindowOmitted(t *testing.T) {
	s := mustParse(t, `{"five_hour":{"utilization":7},"seven_day":null}`)
	if s.SevenDay != nil {
		t.Fatalf("got %+v", s.SevenDay)
	}
	out, _ := json.Marshal(s)
	if strings.Contains(string(out), "seven_day") {
		t.Fatalf("seven_day should be omitted: %s", out)
	}
}

func TestParseWindowWithoutUtilizationOmitted(t *testing.T) {
	s := mustParse(t, `{"five_hour":{"resets_at":"2026-11-11T09:00:00+00:00"},"seven_day":{"utilization":1}}`)
	if s.FiveHour != nil {
		t.Fatalf("got %+v", s.FiveHour)
	}
}

func TestParseBadResetsAtKeepsPercent(t *testing.T) {
	s := mustParse(t, `{"five_hour":{"utilization":7,"resets_at":"tomorrow"}}`)
	if s.FiveHour == nil || *s.FiveHour != (Window{UsedPercent: 7}) {
		t.Fatalf("got %+v", s.FiveHour)
	}
}

func TestParseGarbage(t *testing.T) {
	for _, body := range []string{"<html>", "", "[]", `{"five_hour":"x"}`} {
		if _, err := Parse([]byte(body)); err == nil {
			t.Errorf("%q: want error", body)
		}
	}
}

func TestParseErrorBodyIsError(t *testing.T) {
	body, err := os.ReadFile("testdata/error_401.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(body); err == nil {
		t.Fatal("an API error body has no windows and must not parse as usage")
	}
}

func TestParseRealFixture(t *testing.T) {
	body, err := os.ReadFile("testdata/usage_real.json")
	if err != nil {
		t.Fatal(err)
	}
	s := mustParse(t, string(body))
	if s.FiveHour != nil {
		t.Fatalf("this plan has no five hour window, got %+v", s.FiveHour)
	}
	// 2026-11-13T19:59:59.854844Z is shown as 20:00, not 19:59.
	if s.SevenDay == nil || *s.SevenDay != (Window{UsedPercent: 28, ResetsAt: 1794600000}) {
		t.Fatalf("seven_day %+v", s.SevenDay)
	}
}

func TestParseRealFixtureModelLimits(t *testing.T) {
	body, err := os.ReadFile("testdata/usage_real.json")
	if err != nil {
		t.Fatal(err)
	}
	s := mustParse(t, string(body))
	want := []ModelWindow{{Name: "Fable", UsedPercent: 6, ResetsAt: 1794600000}}
	if !reflect.DeepEqual(s.Models, want) {
		t.Fatalf("got %+v", s.Models)
	}
}

func TestParseModelLimitsSkipsUnusableEntries(t *testing.T) {
	s := mustParse(t, `{"seven_day":{"utilization":1},"limits":[
		{"kind":"weekly_all","percent":28},
		{"kind":"weekly_scoped","percent":150,"scope":{"model":{"display_name":"Opus"}}},
		{"kind":"weekly_scoped","percent":5,"scope":{"model":{"display_name":""}}},
		{"kind":"weekly_scoped","percent":5,"scope":null},
		{"kind":"weekly_scoped","scope":{"model":{"display_name":"NoPercent"}}},
		"junk"
	]}`)
	want := []ModelWindow{{Name: "Opus", UsedPercent: 100}}
	if !reflect.DeepEqual(s.Models, want) {
		t.Fatalf("got %+v", s.Models)
	}
}

func TestParseBrokenLimitsDoesNotFailWholeResponse(t *testing.T) {
	s := mustParse(t, `{"seven_day":{"utilization":1},"limits":"soon"}`)
	if s.SevenDay == nil || s.Models != nil {
		t.Fatalf("got %+v", s)
	}
	out, _ := json.Marshal(s)
	if strings.Contains(string(out), "models") {
		t.Fatalf("models should be omitted: %s", out)
	}
}
