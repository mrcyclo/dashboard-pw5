// Package patch shows the likely result of a tap before the PC answers, like
// claudedash.koplugin/cd_patch.lua. A button carries ops on nodes of its own
// widget; predicted values are held until the PC reports the same values, or Hold.
package patch

import (
	"reflect"
	"regexp"
	"strconv"
	"time"

	"claudedash/internal/tree"
)

// Hold covers the moment the PC's reply to a command still carries the old state.
const Hold = 4 * time.Second

type Pending struct {
	Expires time.Time
	// Values holds the predicted props by node id.
	Values map[string]map[string]any
}

var integer = regexp.MustCompile(`^-?\d+$`)

func copyValue(v any) any {
	switch value := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(value))
		for k, item := range value {
			out[k] = copyValue(item)
		}
		return out
	case []any:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = copyValue(item)
		}
		return out
	}
	return v
}

func number(v any) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}

// next returns the predicted value, or false when the op does not apply.
func next(op map[string]any, current any) (any, bool) {
	switch op["op"] {
	case "set":
		value, ok := op["value"]
		return value, ok
	case "toggle":
		values, _ := op["values"].([]any)
		if len(values) < 2 {
			return nil, false
		}
		if reflect.DeepEqual(current, values[0]) {
			return values[1], true
		}
		return values[0], true
	case "add":
		by, ok := number(op["by"])
		if !ok {
			return nil, false
		}
		text, isText := current.(string)
		value, ok := number(current)
		if isText && integer.MatchString(text) {
			parsed, _ := strconv.Atoi(text)
			value, ok = float64(parsed), true
		}
		if !ok {
			return nil, false
		}
		result := value + by
		if low, ok := number(op["min"]); ok && result < low {
			result = low
		}
		if high, ok := number(op["max"]); ok && result > high {
			result = high
		}
		if isText {
			return strconv.Itoa(int(result)), true
		}
		return result, true
	}
	return nil, false
}

// Apply returns a patched copy of root and the values it set.
func Apply(root tree.Node, ops any) (tree.Node, map[string]map[string]any) {
	out, _ := copyValue(root).(map[string]any)
	set := map[string]map[string]any{}
	list, _ := ops.([]any)
	for _, item := range list {
		op, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := op["id"].(string)
		key, _ := op["prop"].(string)
		node := tree.Find(out, id)
		if node == nil || key == "" {
			continue
		}
		value, ok := next(op, node[key])
		if !ok {
			continue
		}
		node[key] = value
		if set[id] == nil {
			set[id] = map[string]any{}
		}
		set[id][key] = value
	}
	return out, set
}

// Stack applies a tap's ops to what is shown and adds them to the prediction.
func Stack(p *Pending, ops any, shown tree.Node, now time.Time) (tree.Node, *Pending) {
	result, set := Apply(shown, ops)
	if len(set) == 0 {
		return result, p
	}
	stacked := &Pending{Expires: now.Add(Hold), Values: map[string]map[string]any{}}
	if p != nil {
		for id, props := range p.Values {
			stacked.Values[id] = map[string]any{}
			for k, v := range props {
				stacked.Values[id][k] = v
			}
		}
	}
	for id, props := range set {
		if stacked.Values[id] == nil {
			stacked.Values[id] = map[string]any{}
		}
		for k, v := range props {
			stacked.Values[id][k] = v
		}
	}
	return result, stacked
}

// Reconcile returns the tree to show for a new reply and what is still pending.
func Reconcile(p *Pending, incoming tree.Node, now time.Time) (tree.Node, *Pending) {
	if p == nil || !now.Before(p.Expires) {
		return incoming, nil
	}
	agreed := true
	for id, props := range p.Values {
		node := tree.Find(incoming, id)
		if node == nil {
			return incoming, nil
		}
		for k, v := range props {
			if !reflect.DeepEqual(node[k], v) {
				agreed = false
			}
		}
	}
	if agreed {
		return incoming, nil
	}
	shown, _ := copyValue(incoming).(map[string]any)
	for id, props := range p.Values {
		node := tree.Find(shown, id)
		for k, v := range props {
			node[k] = v
		}
	}
	return shown, p
}

// Expire ends a prediction whose hold ran out while no reply came: it returns
// the PC's last tree to show again, or nil while the prediction still holds.
func Expire(p *Pending, last tree.Node, now time.Time) (tree.Node, *Pending) {
	if p == nil {
		return nil, nil
	}
	if now.Before(p.Expires) {
		return nil, p
	}
	return last, nil
}
