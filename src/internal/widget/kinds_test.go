package widget

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"claudedash/internal/grid"
	"claudedash/internal/media"
	"claudedash/internal/usage"
)

var testNow = time.Unix(1794400000, 0)

// walk visits every node of a tree, depth first.
func walk(n Node, visit func(Node)) {
	visit(n)
	children, _ := n["children"].([]Node)
	for _, child := range children {
		walk(child, visit)
	}
}

func byKind(n Node, kind string) []Node {
	var found []Node
	walk(n, func(c Node) {
		if c["kind"] == kind {
			found = append(found, c)
		}
	})
	return found
}

func byID(n Node, id string) Node {
	var found Node
	walk(n, func(c Node) {
		if c["id"] == id {
			found = c
		}
	})
	return found
}

func texts(n Node) []string {
	var out []string
	for _, t := range byKind(n, "text") {
		out = append(out, t["text"].(string))
	}
	return out
}

func TestClockSizes(t *testing.T) {
	k := clockKind{}
	if k.Name() != "clock" || k.Label() != "Clock" || !reflect.DeepEqual(k.Sizes(), []string{"3x1", "3x2"}) {
		t.Fatalf("%s %s %v", k.Name(), k.Label(), k.Sizes())
	}
	small, big := k.Build("3x1", Data{Now: testNow}), k.Build("3x2", Data{Now: testNow})
	for _, tree := range []Node{small, big} {
		clocks := byKind(tree, "clock")
		if len(clocks) != 2 || clocks[0]["format"] != "time" || clocks[1]["format"] != "date" {
			t.Fatalf("clocks %v", clocks)
		}
	}
	for i, part := range []string{"time", "date"} {
		if byKind(big, "clock")[i]["size"].(int) <= byKind(small, "clock")[i]["size"].(int) {
			t.Errorf("the 3x2 clock has a bigger %s", part)
		}
	}
}

func fullSnapshot() usage.Snapshot {
	reset := testNow.Unix() + 3600
	return usage.Snapshot{
		FetchedAt: testNow.Unix(),
		FiveHour:  &usage.Window{UsedPercent: 12, ResetsAt: reset},
		SevenDay:  &usage.Window{UsedPercent: 74, ResetsAt: reset + 86400},
		Models: []usage.ModelWindow{
			{Name: "Fable", UsedPercent: 80, ResetsAt: reset + 86400},
			{Name: "Opus", UsedPercent: 5, ResetsAt: reset + 86400},
			{Name: "Sonnet", UsedPercent: 1, ResetsAt: reset + 86400},
		},
	}
}

func TestClaudeKeepsFourBarsInOrder(t *testing.T) {
	k := claudeKind{}
	if !reflect.DeepEqual(k.Sizes(), []string{"6x1", "3x2"}) || k.Label() != "Claude usage" {
		t.Fatal(k.Sizes(), k.Label())
	}
	for _, size := range k.Sizes() {
		tree := k.Build(size, Data{Now: testNow, Usage: fullSnapshot()})
		if bars := byKind(tree, "bar"); len(bars) != 4 {
			t.Fatalf("%s: %d bars", size, len(bars))
		}
		got := strings.Join(texts(tree), "|")
		for _, want := range []string{"5-hour", "Weekly", "Fable only", "Opus only", "80% used"} {
			if !strings.Contains(got, want) {
				t.Errorf("%s: missing %q in %s", size, want, got)
			}
		}
		if strings.Contains(got, "Sonnet") {
			t.Errorf("%s: the fifth limit must be dropped: %s", size, got)
		}
		if strings.Index(got, "Fable only") > strings.Index(got, "Opus only") {
			t.Errorf("%s: models keep the API order", size)
		}
	}
}

func TestClaudeValuesDropToZeroAtReset(t *testing.T) {
	snap := fullSnapshot()
	tree := claudeKind{}.Build("6x1", Data{Now: testNow, Usage: snap})
	for _, bar := range byKind(tree, "bar") {
		at, ok := bar["at"].(map[string]any)
		if !ok || at["set"].(map[string]any)["percent"] != 0 {
			t.Fatalf("bar without reset switch: %v", bar)
		}
	}
	for _, text := range byKind(tree, "text") {
		if !strings.HasSuffix(text["text"].(string), "% used") {
			continue
		}
		at, ok := text["at"].(map[string]any)
		if !ok || at["set"].(map[string]any)["text"] != "0% used" {
			t.Fatalf("used text without reset switch: %v", text)
		}
	}
	countdowns := byKind(tree, "countdown")
	if len(countdowns) < 2 || countdowns[0]["to"] != snap.FiveHour.ResetsAt || countdowns[1]["to"] != snap.SevenDay.ResetsAt {
		t.Fatalf("countdowns %v", countdowns)
	}
}

func TestClaudeAlreadyResetShowsZero(t *testing.T) {
	snap := usage.Snapshot{SevenDay: &usage.Window{UsedPercent: 74, ResetsAt: testNow.Unix() - 10}}
	tree := claudeKind{}.Build("6x1", Data{Now: testNow, Usage: snap})
	if got := strings.Join(texts(tree), "|"); !strings.Contains(got, "0% used") {
		t.Fatal(got)
	}
	if len(byKind(tree, "countdown")) != 0 {
		t.Fatal("no countdown once the reset has passed")
	}
}

func TestClaudeWithoutData(t *testing.T) {
	tree := claudeKind{}.Build("6x1", Data{Now: testNow, Usage: usage.Snapshot{Error: "Cannot reach Claude"}})
	if got := texts(tree); !reflect.DeepEqual(got, []string{"Claude", "Cannot reach Claude"}) {
		t.Fatal(got)
	}
	tree = claudeKind{}.Build("3x2", Data{Now: testNow})
	if got := texts(tree); !reflect.DeepEqual(got, []string{"Claude", "No data yet"}) {
		t.Fatal(got)
	}
}

func intp(v int) *int    { return &v }
func boolp(v bool) *bool { return &v }

func TestMediaIdle(t *testing.T) {
	k := NewMedia(nil)
	if !reflect.DeepEqual(k.Sizes(), []string{"6x2", "3x2", "6x1"}) || k.Label() != "Music" {
		t.Fatal(k.Sizes(), k.Label())
	}
	tree := k.Build("6x2", Data{Now: testNow})
	buttons := byKind(tree, "button")
	disabled := 0
	for _, b := range buttons {
		if b["disabled"] == true {
			disabled++
		}
	}
	if disabled < 3 {
		t.Fatalf("playback buttons must be disabled when nothing plays: %v", buttons)
	}
	got := strings.Join(texts(tree), "|")
	if !strings.Contains(got, "Nothing playing") || !strings.Contains(got, "Music") {
		t.Fatal(got)
	}
}

func TestMediaPlayingPredictsTaps(t *testing.T) {
	state := media.State{Active: true, Title: "Song", Artist: "Band", App: "Chrome", Playing: true, Art: "abc", Volume: intp(20), Muted: boolp(false)}
	tree := NewMedia(nil).Build("6x2", Data{Now: testNow, Media: state})
	if got := strings.Join(texts(tree), "|"); !strings.Contains(got, "Playing · Chrome") || !strings.Contains(got, "Song") {
		t.Fatal(got)
	}
	pp := byID(tree, "pp")
	if pp["icon"] != "pause" || pp["action"] != "media.play-pause" {
		t.Fatalf("play/pause %v", pp)
	}
	if !reflect.DeepEqual(pp["patch"], []PatchOp{Toggle("pp", "icon", "pause", "play")}) {
		t.Fatalf("patch %v", pp["patch"])
	}
	var up Node
	for _, b := range byKind(tree, "button") {
		if b["action"] == "media.volume-up" {
			up = b
		}
	}
	want := []PatchOp{Add("vol-bar", "percent", 5, 0, 100), Add("vol-num", "text", 5, 0, 100)}
	if up == nil || !reflect.DeepEqual(up["patch"], want) {
		t.Fatalf("volume up %v", up)
	}
	if byID(tree, "vol-bar")["percent"] != 20 || byID(tree, "vol-num")["text"] != "20" {
		t.Fatal("volume values")
	}
	if len(byKind(tree, "image")) != 1 || byKind(tree, "image")[0]["src"] != "/media/art.png?v=abc" {
		t.Fatal("cover art")
	}
	title := byID(tree, "title")
	if title["grow"] == nil && byID(tree, "track")["grow"] == nil {
		t.Fatal("the track column must grow so a long title is cut instead of pushing the buttons")
	}
}

func TestMediaPausedAndMuted(t *testing.T) {
	state := media.State{Active: true, Title: "Song", Playing: false, Volume: intp(30), Muted: boolp(true)}
	tree := NewMedia(nil).Build("6x2", Data{Now: testNow, Media: state})
	if got := strings.Join(texts(tree), "|"); !strings.Contains(got, "Paused") || strings.Contains(got, "Paused ·") {
		t.Fatal(got)
	}
	if byID(tree, "pp")["icon"] != "play" || byID(tree, "vol-num")["text"] != "Mute" || byID(tree, "vol-bar")["percent"] != 0 {
		t.Fatal("paused and muted values")
	}
}

func TestMediaCompactHasNoVolume(t *testing.T) {
	state := media.State{Active: true, Title: "Song", Playing: true, Volume: intp(20)}
	tree := NewMedia(nil).Build("6x1", Data{Now: testNow, Media: state})
	if byID(tree, "vol-bar") != nil || byID(tree, "vol-num") != nil {
		t.Fatal("6x1 has no volume row")
	}
	if byID(tree, "pp") == nil {
		t.Fatal("6x1 keeps the playback buttons")
	}
}

func TestDefaultCatalog(t *testing.T) {
	var sent []string
	c := Default(func(cmd string) (media.State, error) {
		sent = append(sent, cmd)
		return media.State{}, nil
	})
	var names []string
	for _, k := range c.All() {
		names = append(names, k.Name())
	}
	if !reflect.DeepEqual(names, []string{"clock", "claude", "media", "apps"}) {
		t.Fatal(names)
	}
	action, ok := c.Action("media.next")
	if !ok || action() != nil || !reflect.DeepEqual(sent, []string{"next"}) {
		t.Fatalf("%v %v", ok, sent)
	}
	if _, ok := c.Action("media.explode"); ok {
		t.Fatal("unknown action")
	}
	if _, ok := c.Get("media"); !ok {
		t.Fatal("get")
	}
}

func TestIconsEmbedded(t *testing.T) {
	for _, name := range []string{"play", "pause", "next", "previous", "note", "volume-low", "volume-high"} {
		data, ok := IconSVG(name)
		if !ok || !strings.Contains(string(data), "<svg") || len(IconHash(name)) != 12 {
			t.Errorf("icon %s", name)
		}
	}
	if _, ok := IconSVG("../catalog"); ok {
		t.Error("names must not escape the icon folder")
	}
}

// height is a widget tree's natural height under the layout rules of internal/tree.
func height(n Node) float64 {
	num := func(key string, fallback float64) float64 {
		if v, ok := n[key].(int); ok {
			return float64(v)
		}
		return fallback
	}
	switch n["kind"] {
	case "text", "clock", "countdown":
		return math.Ceil(num("size", 20) * 1.3)
	case "icon", "button":
		return num("size", 0)
	case "image":
		return num("h", 0)
	case "bar":
		return num("height", 8)
	case "rule":
		return num("thickness", 2)
	case "space":
		return num("size", 0)
	}
	children, _ := n["children"].([]Node)
	total := 0.0
	for _, child := range children {
		h := height(child)
		if n["kind"] == "row" {
			total = math.Max(total, h)
		} else {
			total += h
		}
	}
	if n["kind"] == "col" && len(children) > 1 {
		total += float64(len(children)-1) * num("gap", 0)
	}
	return total
}

// The longest content a Claude widget gets: four limits with different resets, stale.
func worstSnapshot() usage.Snapshot {
	reset := testNow.Unix() + 3600
	return usage.Snapshot{
		FetchedAt: testNow.Unix() - 600,
		Stale:     true,
		FiveHour:  &usage.Window{UsedPercent: 100, ResetsAt: reset},
		SevenDay:  &usage.Window{UsedPercent: 100, ResetsAt: reset + 86400},
		Models: []usage.ModelWindow{
			{Name: "Sonnet", UsedPercent: 100, ResetsAt: reset + 2*86400},
			{Name: "Opus", UsedPercent: 100},
		},
	}
}

func TestEverySizeFitsItsBox(t *testing.T) {
	vol := 100
	playing := media.State{Active: true, Title: "Song", Artist: "Band", App: "Chrome", Playing: true, Art: "a", Volume: &vol}
	data := Data{Now: testNow, Usage: worstSnapshot(), Media: playing, Apps: tiles(20)}
	for _, k := range Default(nil).All() {
		for _, size := range k.Sizes() {
			s, _ := grid.ParseSize(size)
			inner := float64(grid.CellRect(0, 0, s).H - 48)
			if got := height(k.Build(size, data)); got > inner {
				t.Errorf("%s %s: content is %.0f px tall, the box has %.0f", k.Name(), size, got, inner)
			}
		}
	}
}

// The wide size keeps the original look: a titled header with the shared reset, then the bars.
func TestClaudeWideKeepsTheOriginalLook(t *testing.T) {
	reset := testNow.Unix() + 3600
	snap := usage.Snapshot{
		SevenDay: &usage.Window{UsedPercent: 77, ResetsAt: reset},
		Models:   []usage.ModelWindow{{Name: "Fable", UsedPercent: 82, ResetsAt: reset + 1}},
	}
	tree := claudeKind{}.Build("6x1", Data{Now: testNow, Usage: snap})
	got := texts(tree)
	want := []string{"Claude · weekly limits", "All models", "77% used", "Fable only", "82% used"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
	countdowns := byKind(tree, "countdown")
	if len(countdowns) != 1 || countdowns[0]["to"] != reset {
		t.Fatalf("one shared countdown in the header: %v", countdowns)
	}
	if len(byKind(tree, "rule")) != 1 {
		t.Fatal("a rule under the header")
	}
}

func TestClaudeWideNamesTheFiveHourLimit(t *testing.T) {
	got := strings.Join(texts(claudeKind{}.Build("6x1", Data{Now: testNow, Usage: fullSnapshot()})), "|")
	if !strings.HasPrefix(got, "Claude · usage limits|5-hour|") || !strings.Contains(got, "|Weekly|") {
		t.Fatal(got)
	}
}

func TestCountdownsAreNotDimmed(t *testing.T) {
	for _, size := range []string{"6x1", "3x2"} {
		for _, c := range byKind(claudeKind{}.Build(size, Data{Now: testNow, Usage: worstSnapshot()}), "countdown") {
			if c["dim"] == true {
				t.Fatalf("%s: reset times must be easy to read: %v", size, c)
			}
		}
	}
}

// The tall size spreads its limits over the whole height instead of packing them at the top.
func TestClaudeTallSpreadsItsRows(t *testing.T) {
	tree := claudeKind{}.Build("3x2", Data{Now: testNow, Usage: fullSnapshot()})
	children := tree["children"].([]Node)
	growing := 0
	for _, child := range children {
		if child["kind"] == "space" && child["grow"] != nil {
			growing++
		}
	}
	if growing != 5 {
		t.Fatalf("four limits need five growing spaces around them, got %d", growing)
	}
	last := children[len(children)-1]
	if last["kind"] != "space" || last["grow"] == nil {
		t.Fatal("the room below the last limit is shared too")
	}
}

func TestMediaSquareKeepsEveryControl(t *testing.T) {
	vol := 20
	state := media.State{Active: true, Title: "Song", Artist: "Band", App: "Chrome", Playing: true, Art: "a", Volume: &vol}
	tree := NewMedia(nil).Build("3x2", Data{Now: testNow, Media: state})
	for _, id := range []string{"pp", "vol-bar", "vol-num", "track"} {
		if byID(tree, id) == nil {
			t.Errorf("3x2 is missing %s", id)
		}
	}
	if got := strings.Join(texts(tree), "|"); !strings.Contains(got, "Playing · Chrome") || !strings.Contains(got, "Song") {
		t.Fatal(got)
	}
	// At half width the cover, the title and three buttons cannot share one line.
	for _, row := range byKind(tree, "row") {
		if byID(row, "pp") != nil && len(byKind(row, "image")) > 0 {
			t.Fatal("the buttons get a line of their own, below the cover and title")
		}
	}
	actions := map[string]bool{}
	for _, b := range byKind(tree, "button") {
		actions[b["action"].(string)] = true
	}
	if len(actions) != 5 {
		t.Fatalf("all five controls: %v", actions)
	}
}

// The tall size uses the same header as the wide one.
func TestClaudeTallHeaderMatchesWide(t *testing.T) {
	reset := testNow.Unix() + 3600
	snap := usage.Snapshot{
		SevenDay: &usage.Window{UsedPercent: 77, ResetsAt: reset},
		Models:   []usage.ModelWindow{{Name: "Fable", UsedPercent: 82, ResetsAt: reset + 1}},
	}
	tree := claudeKind{}.Build("3x2", Data{Now: testNow, Usage: snap})
	got := texts(tree)
	// Half width has no room for the long title next to the reset time.
	if len(got) == 0 || got[0] != "Claude" {
		t.Fatalf("got %q", got)
	}
	countdowns := byKind(tree, "countdown")
	if len(countdowns) != 1 || countdowns[0]["to"] != reset {
		t.Fatalf("one shared countdown in the header, none per row: %v", countdowns)
	}
}

func TestClockAlignment(t *testing.T) {
	k := clockKind{}
	var configurable Configurable = k
	options := configurable.Options()
	if len(options) != 1 || options[0].Key != "align" || options[0].Default != "left" || len(options[0].Choices) != 3 {
		t.Fatalf("options %+v", options)
	}
	cases := map[string]string{"": "start", "left": "start", "center": "center", "right": "end", "sideways": "start"}
	for _, size := range k.Sizes() {
		for value, want := range cases {
			d := Data{Now: testNow, Options: map[string]map[string]string{"clock": {"align": value}}}
			if got := k.Build(size, d)["align"]; got != want {
				t.Errorf("%s align %q: got %v want %s", size, value, got, want)
			}
		}
	}
}

func TestBigClockUsesTheRoom(t *testing.T) {
	if size := byKind(clockKind{}.Build("3x2", Data{Now: testNow}), "clock")[0]["size"].(int); size < 160 {
		t.Fatalf("the 3x2 time is only %d px", size)
	}
}

func TestBarsAreThickEnoughToRead(t *testing.T) {
	vol := 40
	data := Data{Now: testNow, Usage: fullSnapshot(), Media: media.State{Active: true, Title: "Song", Volume: &vol}}
	for _, k := range Default(nil).All() {
		for _, size := range k.Sizes() {
			for _, bar := range byKind(k.Build(size, data), "bar") {
				if bar["height"].(int) < 20 {
					t.Errorf("%s %s: a %d px bar is hard to read", k.Name(), size, bar["height"])
				}
			}
		}
	}
}
