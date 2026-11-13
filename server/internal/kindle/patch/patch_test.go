package patch

import (
	"testing"
	"time"

	"claudedash/internal/tree"
)

func media(icon string, volume float64, text string) tree.Node {
	return tree.Node{"kind": "col", "children": []any{
		map[string]any{"kind": "button", "id": "pp", "icon": icon, "size": 120.0},
		map[string]any{"kind": "bar", "id": "vol-bar", "percent": volume, "height": 16.0},
		map[string]any{"kind": "text", "id": "vol-num", "text": text, "size": 43.0},
	}}
}

var toggle = []any{map[string]any{"id": "pp", "op": "toggle", "prop": "icon", "values": []any{"pause", "play"}}}
var up = []any{
	map[string]any{"id": "vol-bar", "op": "add", "prop": "percent", "by": 5.0, "min": 0.0, "max": 100.0},
	map[string]any{"id": "vol-num", "op": "add", "prop": "text", "by": 5.0, "min": 0.0, "max": 100.0},
}

func prop(root tree.Node, id, key string) any {
	return tree.Find(root, id)[key]
}

func TestApplyCopies(t *testing.T) {
	original := media("pause", 20, "20")
	changed, _ := Apply(original, toggle)
	if prop(changed, "pp", "icon") != "play" || prop(original, "pp", "icon") != "pause" {
		t.Fatal("toggle must copy")
	}
	back, _ := Apply(changed, toggle)
	if prop(back, "pp", "icon") != "pause" {
		t.Fatal("toggle back")
	}
	other, _ := Apply(media("stop", 20, "20"), toggle)
	if prop(other, "pp", "icon") != "pause" {
		t.Fatal("anything else becomes the first value")
	}
}

func TestAdd(t *testing.T) {
	once, _ := Apply(media("pause", 20, "20"), up)
	twice, _ := Apply(once, up)
	if prop(twice, "vol-bar", "percent") != 30.0 || prop(twice, "vol-num", "text") != "30" {
		t.Fatalf("%v %v", prop(twice, "vol-bar", "percent"), prop(twice, "vol-num", "text"))
	}
	top, _ := Apply(media("pause", 98, "98"), up)
	if prop(top, "vol-bar", "percent") != 100.0 {
		t.Fatal("clamped at max")
	}
	down := []any{map[string]any{"id": "vol-bar", "op": "add", "prop": "percent", "by": -5.0, "min": 0.0, "max": 100.0}}
	low, _ := Apply(media("pause", 3, "3"), down)
	if prop(low, "vol-bar", "percent") != 0.0 {
		t.Fatal("clamped at min")
	}
	mute, _ := Apply(media("pause", 0, "Mute"), up)
	if prop(mute, "vol-num", "text") != "Mute" {
		t.Fatal("add skips text that is not a number")
	}
	set, _ := Apply(media("pause", 0, "0"), []any{map[string]any{"id": "pp", "op": "set", "prop": "icon", "value": "next"}})
	if prop(set, "pp", "icon") != "next" {
		t.Fatal("set")
	}
	missing, _ := Apply(media("pause", 0, "0"), []any{map[string]any{"id": "nope", "op": "set", "prop": "icon", "value": "x"}})
	if prop(missing, "pp", "icon") != "pause" {
		t.Fatal("missing id is ignored")
	}
}

var now = time.Unix(1000, 0)

func TestStackAddsUp(t *testing.T) {
	original := media("pause", 20, "20")
	shown, pending := Stack(nil, up, original, now)
	shown, pending = Stack(pending, up, shown, now.Add(time.Second))
	if prop(shown, "vol-bar", "percent") != 30.0 || pending == nil {
		t.Fatal("two taps")
	}
	shown, pending = Stack(nil, toggle, original, now)
	shown, _ = Stack(pending, toggle, shown, now)
	if prop(shown, "pp", "icon") != "pause" {
		t.Fatal("two play/pause taps cancel out")
	}
}

func TestReconcile(t *testing.T) {
	original := media("pause", 20, "20")
	_, pending := Stack(nil, toggle, original, now)
	stale := media("pause", 25, "25")

	result, still := Reconcile(pending, stale, now.Add(time.Second))
	if prop(result, "pp", "icon") != "play" || prop(result, "vol-bar", "percent") != 25.0 || still == nil {
		t.Fatal("prediction kept, other values from the PC")
	}
	if prop(stale, "pp", "icon") != "pause" {
		t.Fatal("the incoming tree is not changed")
	}
	result, still = Reconcile(pending, media("play", 25, "25"), now.Add(time.Second))
	if prop(result, "pp", "icon") != "play" || still != nil {
		t.Fatal("agreed")
	}
	result, still = Reconcile(pending, stale, now.Add(Hold))
	if prop(result, "pp", "icon") != "pause" || still != nil {
		t.Fatal("expired")
	}
	empty := tree.Node{"kind": "col", "children": []any{}}
	result, still = Reconcile(pending, empty, now.Add(time.Second))
	if still != nil || result["kind"] != "col" {
		t.Fatal("missing node drops the prediction")
	}
	result, still = Reconcile(nil, stale, now)
	if prop(result, "pp", "icon") != "pause" || still != nil {
		t.Fatal("nothing pending")
	}
}

func TestBadPatchesAreIgnored(t *testing.T) {
	original := media("pause", 20, "20")
	shown, pending := Stack(nil, nil, original, now)
	if prop(shown, "pp", "icon") != "pause" || pending != nil {
		t.Fatal("nil patch")
	}
	shown, _ = Apply(original, []any{nil, "x", map[string]any{"id": "pp", "op": "explode", "prop": "icon"}})
	if prop(shown, "pp", "icon") != "pause" {
		t.Fatal("unknown op")
	}
}

func TestExpire(t *testing.T) {
	original := media("pause", 20, "20")
	_, pending := Stack(nil, toggle, original, now)
	if result, still := Expire(pending, original, now.Add(3*time.Second)); result != nil || still != pending {
		t.Fatal("still held before 4 seconds")
	}
	result, still := Expire(pending, original, now.Add(Hold))
	if prop(result, "pp", "icon") != "pause" || still != nil {
		t.Fatal("back to what the PC last said")
	}
	if result, still := Expire(nil, original, now.Add(time.Hour)); result != nil || still != nil {
		t.Fatal("nothing pending, nothing to do")
	}
}
