package widget

import (
	"encoding/json"
	"os"
	"testing"
)

func marshal(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestConstructorsJSON(t *testing.T) {
	cases := map[string]string{
		marshal(t, Text("Hi", 30).Bold().ID("t")):                        `{"bold":true,"id":"t","kind":"text","size":30,"text":"Hi"}`,
		marshal(t, Space(0).Grow(1)):                                     `{"grow":1,"kind":"space","size":0}`,
		marshal(t, Bar(42, 10).ID("b")):                                  `{"height":10,"id":"b","kind":"bar","percent":42}`,
		marshal(t, Button("play", 120, "media.x")):                       `{"action":"media.x","icon":"play","kind":"button","size":120}`,
		marshal(t, Countdown(99, 20).Dim()):                              `{"dim":true,"kind":"countdown","size":20,"to":99}`,
		marshal(t, Clock("date", 30)):                                    `{"format":"date","kind":"clock","size":30}`,
		marshal(t, Image("/media/art.png?v=1", 130, 130)):                `{"h":130,"kind":"image","src":"/media/art.png?v=1","w":130}`,
		marshal(t, Rule()):                                               `{"kind":"rule","thickness":2}`,
		marshal(t, Col(8, Icon("note", 40)).Align("center")):             `{"align":"center","children":[{"kind":"icon","name":"note","size":40}],"gap":8,"kind":"col"}`,
		marshal(t, Text("5%", 20).At(100, map[string]any{"text": "0%"})): `{"at":{"set":{"text":"0%"},"time":100},"kind":"text","size":20,"text":"5%"}`,
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got  %s\nwant %s", got, want)
		}
	}
}

func TestPatchOps(t *testing.T) {
	b := Button("pause", 120, "media.play-pause").Patch(
		Toggle("pp", "icon", "pause", "play"),
		Add("vol", "percent", 5, 0, 100),
		Set("t", "text", "x"),
	).Disabled()
	want := `{"action":"media.play-pause","disabled":true,"icon":"pause","kind":"button","patch":[` +
		`{"id":"pp","op":"toggle","prop":"icon","values":["pause","play"]},` +
		`{"by":5,"id":"vol","max":100,"min":0,"op":"add","prop":"percent"},` +
		`{"id":"t","op":"set","prop":"text","value":"x"}],"size":120}`
	if got := marshal(t, b); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestRowChildren(t *testing.T) {
	r := Row(4, Text("a", 10), Text("b", 10))
	if got := marshal(t, r); got != `{"align":"center","children":[{"kind":"text","size":10,"text":"a"},{"kind":"text","size":10,"text":"b"}],"gap":4,"kind":"row"}` {
		t.Fatal(got)
	}
}

func TestHashStable(t *testing.T) {
	a := Col(4, Text("x", 10))
	b := Col(4, Text("x", 10))
	c := Col(4, Text("y", 10))
	if len(Hash(a)) != 12 || Hash(a) != Hash(b) || Hash(a) == Hash(c) {
		t.Fatalf("%s %s %s", Hash(a), Hash(b), Hash(c))
	}
}

func TestFixturesParse(t *testing.T) {
	data, err := os.ReadFile("testdata/tree_fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name   string         `json:"name"`
		Box    map[string]int `json:"box"`
		Tree   map[string]any `json:"tree"`
		Expect []struct {
			ID         string `json:"id"`
			X, Y, W, H int
		} `json:"expect"`
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures) < 7 {
		t.Fatalf("expected at least 7 fixtures, got %d", len(fixtures))
	}
	for _, f := range fixtures {
		if f.Name == "" || f.Tree["kind"] == nil || f.Box["w"] < 1 || len(f.Expect) == 0 {
			t.Errorf("incomplete fixture %+v", f)
		}
	}
}
