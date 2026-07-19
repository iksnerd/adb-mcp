package adb

import (
	"context"
	"fmt"
	"time"

	"github.com/iksnerd/adb_mcp/internal/uiauto"
)

// Step is one action in a RunSequence batch. Action names the operation; the
// other fields are its parameters (only the ones the action uses are read). The
// optional IfPresent/IfAbsent guard is evaluated against the live hierarchy
// before the step runs — a failed guard skips the step (not an error), which is
// how a conditional "cancel the prompt IF it's up" is expressed. A step that
// errors aborts the rest of the sequence unless Optional is set.
type Step struct {
	Action     string  `json:"action" jsonschema:"The operation: sleep, tap, tap_text, tap_element, key, text, swipe, launch, stop, wait_text, or describe_ui."`
	Seconds    float64 `json:"seconds,omitempty" jsonschema:"sleep: how long to pause, in seconds (fractions ok)."`
	X          int     `json:"x,omitempty" jsonschema:"tap/swipe: X in true device pixels (swipe start)."`
	Y          int     `json:"y,omitempty" jsonschema:"tap/swipe: Y in true device pixels (swipe start)."`
	X2         int     `json:"x2,omitempty" jsonschema:"swipe: end X in true device pixels."`
	Y2         int     `json:"y2,omitempty" jsonschema:"swipe: end Y in true device pixels."`
	DurationMS int     `json:"duration_ms,omitempty" jsonschema:"swipe: duration in ms (default 300)."`
	Text       string  `json:"text,omitempty" jsonschema:"tap_text/wait_text: label or content-desc to match; text: the string to type."`
	ResourceID string  `json:"resource_id,omitempty" jsonschema:"tap_element: resource id to match (substring by default)."`
	Key        string  `json:"key,omitempty" jsonschema:"key: key name (back, home, enter, ...) or a raw keycode."`
	Package    string  `json:"package,omitempty" jsonschema:"launch/stop: application package name."`
	TimeoutS   int     `json:"timeout_s,omitempty" jsonschema:"wait_text: how long to wait in seconds (default 15)."`
	Filter     string  `json:"filter,omitempty" jsonschema:"describe_ui: which nodes to capture (auto/clickable/all). Default auto."`
	Partial    *bool   `json:"partial,omitempty" jsonschema:"tap_text/tap_element: substring match instead of exact. Default true."`
	IfPresent  string  `json:"if_present,omitempty" jsonschema:"Guard: only run this step if a selector (matched against text/content_desc/resource_id) is currently on screen — e.g. 'biometric' to cancel a prompt only when it's up."`
	IfAbsent   string  `json:"if_absent,omitempty" jsonschema:"Guard: only run this step if the selector is NOT currently on screen."`
	Optional   bool    `json:"optional,omitempty" jsonschema:"If true, a failure in this step is recorded but does NOT abort the rest of the sequence."`
}

// StepResult is the outcome of one Step: status is ok, skipped (a guard
// failed), or error. Elements is populated only by describe_ui steps.
type StepResult struct {
	Index    int              `json:"index"`
	Action   string           `json:"action"`
	Status   string           `json:"status"` // ok | skipped | error
	Detail   string           `json:"detail,omitempty"`
	Error    string           `json:"error,omitempty"`
	Elements []uiauto.Element `json:"elements,omitempty"`
}

// SequenceResult is the outcome of a whole RunSequence: the per-step results,
// whether a non-optional step aborted it early, and (optionally) the final
// settled hierarchy so the caller sees the end state without a second call.
type SequenceResult struct {
	Steps   []StepResult     `json:"steps"`
	Aborted bool             `json:"aborted"`
	Final   []uiauto.Element `json:"final_hierarchy,omitempty"`
}

// RunSequence executes steps in order inside one tool call — no agent round-trip
// between them. This keeps the timing tight for flows driven by native timers
// (a background-token clear, a biometric prompt that auto-fires on RESUME),
// where a round-trip per step would perturb the very thing being tested. It
// returns a result per step; a non-optional step error aborts the rest. When
// captureFinal is set (and the run wasn't aborted) it appends the settled
// hierarchy so the caller sees the end state in the same response.
func (c *Client) RunSequence(ctx context.Context, steps []Step, captureFinal bool) (SequenceResult, error) {
	var res SequenceResult
	for i := range steps {
		s := &steps[i]
		sr := StepResult{Index: i, Action: s.Action, Status: "ok"}

		if s.IfPresent != "" || s.IfAbsent != "" {
			run, err := c.guardPasses(ctx, s.IfPresent, s.IfAbsent)
			if err != nil {
				sr.Status, sr.Error = "error", fmt.Sprintf("guard read failed: %v", err)
				res.Steps = append(res.Steps, sr)
				if !s.Optional {
					res.Aborted = true
					return res, nil
				}
				continue
			}
			if !run {
				sr.Status = "skipped"
				sr.Detail = "guard not satisfied"
				res.Steps = append(res.Steps, sr)
				continue
			}
		}

		detail, elems, err := c.runStep(ctx, s)
		if err != nil {
			sr.Status, sr.Error = "error", err.Error()
			res.Steps = append(res.Steps, sr)
			if !s.Optional {
				res.Aborted = true
				return res, nil
			}
			continue
		}
		sr.Detail, sr.Elements = detail, elems
		res.Steps = append(res.Steps, sr)
	}

	if captureFinal && !res.Aborted {
		if elems, _, err := c.describeSettled(ctx, uiauto.FilterAuto); err == nil {
			res.Final = elems
		}
	}
	return res, nil
}

// guardPasses evaluates an IfPresent/IfAbsent guard against a single (cheap)
// hierarchy read: IfPresent requires the selector to be on screen, IfAbsent
// requires it not to be. Both empty means no guard (always runs).
func (c *Client) guardPasses(ctx context.Context, ifPresent, ifAbsent string) (bool, error) {
	elems, err := c.dumpOnce(ctx)
	if err != nil {
		return false, err
	}
	if ifPresent != "" && len(uiauto.FilterByQuery(elems, ifPresent)) == 0 {
		return false, nil
	}
	if ifAbsent != "" && len(uiauto.FilterByQuery(elems, ifAbsent)) > 0 {
		return false, nil
	}
	return true, nil
}

// runStep dispatches one step to the matching client command. It returns a short
// human detail, any captured elements (describe_ui only), and an error.
func (c *Client) runStep(ctx context.Context, s *Step) (string, []uiauto.Element, error) {
	partial := s.Partial == nil || *s.Partial
	switch s.Action {
	case "sleep":
		if s.Seconds <= 0 {
			return "", nil, fmt.Errorf("sleep needs a positive 'seconds'")
		}
		d := time.Duration(s.Seconds * float64(time.Second))
		select {
		case <-ctx.Done():
			return "", nil, ctx.Err()
		case <-time.After(d):
		}
		return fmt.Sprintf("slept %gs", s.Seconds), nil, nil

	case "tap":
		return fmt.Sprintf("tapped (%d,%d)", s.X, s.Y), nil, c.Tap(ctx, s.X, s.Y)

	case "tap_text":
		elems, err := c.dumpOnce(ctx)
		if err != nil {
			return "", nil, err
		}
		e, ok := uiauto.FindByText(elems, s.Text, partial)
		if !ok {
			return "", nil, fmt.Errorf("no element matching text %q on screen", s.Text)
		}
		return fmt.Sprintf("tapped %q at (%d,%d)", s.Text, e.Center.X, e.Center.Y), nil, c.Tap(ctx, e.Center.X, e.Center.Y)

	case "tap_element":
		elems, _, err := c.dumpFiltered(ctx, uiauto.FilterAll)
		if err != nil {
			return "", nil, err
		}
		e, ok := uiauto.FindByResourceID(elems, s.ResourceID, partial)
		if !ok {
			return "", nil, fmt.Errorf("no element with resource_id %q on screen", s.ResourceID)
		}
		return fmt.Sprintf("tapped %q at (%d,%d)", s.ResourceID, e.Center.X, e.Center.Y), nil, c.Tap(ctx, e.Center.X, e.Center.Y)

	case "key":
		code, err := ResolveKey(s.Key)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("pressed %s", s.Key), nil, c.PressKey(ctx, code)

	case "text":
		return fmt.Sprintf("typed %q", s.Text), nil, c.InputText(ctx, s.Text)

	case "swipe":
		return fmt.Sprintf("swiped (%d,%d)->(%d,%d)", s.X, s.Y, s.X2, s.Y2), nil,
			c.Swipe(ctx, s.X, s.Y, s.X2, s.Y2, s.DurationMS)

	case "launch":
		if s.Package == "" {
			return "", nil, fmt.Errorf("launch needs a 'package'")
		}
		comp, err := c.LaunchApp(ctx, s.Package)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("launched %s (%s)", s.Package, comp), nil, nil

	case "stop":
		if s.Package == "" {
			return "", nil, fmt.Errorf("stop needs a 'package'")
		}
		return "stopped " + s.Package, nil, c.StopApp(ctx, s.Package)

	case "wait_text":
		timeout := time.Duration(s.TimeoutS) * time.Second
		e, err := c.WaitForText(ctx, s.Text, partial, timeout)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("%q appeared at (%d,%d)", s.Text, e.Center.X, e.Center.Y), nil, nil

	case "describe_ui":
		filter, err := uiauto.ParseUIFilter(s.Filter)
		if err != nil {
			return "", nil, err
		}
		elems, _, err := c.describeSettled(ctx, filter)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("captured %d element(s)", len(elems)), elems, nil

	default:
		return "", nil, fmt.Errorf("unknown action %q (use: sleep, tap, tap_text, tap_element, key, text, swipe, launch, stop, wait_text, describe_ui)", s.Action)
	}
}
