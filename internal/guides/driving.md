# Driving an Android emulator with this MCP

There is no built-in Android UI-automation the way XcodeBuildMCP has for iOS.
These tools give you that: boot an AVD, look at the screen, read the real UI
hierarchy, and act on it. Driving a UI is a **tight loop**:

> **observe → locate → act → re-observe**

Never fire a chain of taps blind. UI state (modals, errors, async loads, list
reorders) drifts from your mental model fast; a one-call look prevents a chain of
taps landing on the wrong screen.

## The core loop

1. **Observe** — call `screenshot` and actually look at the returned image to
   understand the current state.
2. **Locate precisely** — call `describe_ui`. It returns every interesting
   element with its `text`, `content_desc`, `resource_id`, and a `center` in
   **true device pixels**. Use that `center` for tap coordinates. **Do not guess
   coordinates from the screenshot** — the image is downscaled, so guessed taps
   miss. The screenshot is for *seeing*; `describe_ui` is for *aiming*.
3. **Act** — `tap` (or `tap_on_text` to skip the manual lookup), `swipe`,
   `input_text`, `press_key`.
4. **Re-observe** — `screenshot` again and confirm the state actually changed
   before the next action.

`tap_on_text` fuses steps 2–3: it runs `describe_ui`, finds the element by
visible label or content-description, and taps its center. Prefer it when you
know the label of what you want to press.

Loop economy: in a long drive, `describe_ui {compact: true}` gives the same
aiming information at ~10x fewer tokens, and `{query: "..."}` answers "is X
on screen?" without the whole tree. When an action might be silently consumed
(a key press with an overlay up), `verify_change: true` on `press_key`/`tap`
replaces a full re-observe with a built-in `ui_changed` answer. For purely
time-based conditions (an app must stay backgrounded 18s to trip a native
timer), use `wait` — `wait_for_text` can't express elapsed time.

## Coordinate rule

`tap`/`swipe` always use **true device pixels**. The `center` values from
`describe_ui` are already in true pixels — pass them straight through. When a tap
"does nothing", it is almost always a stale or misscaled coordinate: re-run
`describe_ui` and use a fresh `center`.

## Gotchas that waste turns

- **A toast/overlay silently eats taps.** e.g. a React-Native LogBox banner
  ("Open debugger to view warnings") sits over the bottom tab bar and intercepts
  taps meant for tabs beneath it. Dismiss the overlay (tap its ✕) first.
- **The keyboard covers buttons after `input_text`.** Send `press_key escape`
  (or `back`) to dismiss the IME before tapping a button lower on screen.
- **Route/tab changes need a settle delay.** After navigation, take a
  `screenshot` before assuming the new screen is ready; animations and
  Fast-Refresh aren't instant.
- **`describe_ui` can transiently fail mid-animation** ("could not get idle
  state"). This tool already retries once; if it still fails, wait and call again.
- **`screenshot` and `describe_ui` can briefly disagree during a transition.**
  Mid-navigation (an app backgrounding, a dev-launcher hand-off) a `screenshot`
  may show one screen while a `describe_ui` a moment later returns a different
  tree — a timing race, not a bug. Let the screen settle (short wait, re-take)
  before trusting the two together.
- **A black `screenshot` isn't always a black screen.** `screenshot` retries an
  all-black frame automatically and, if it stays black, tells you why in the
  caption: a `FLAG_SECURE` window (e.g. a native PIN pad the OS blanks) or a
  sleeping display. Either way, **fall back to `describe_ui`** — it reads the
  hierarchy even when the pixels are blanked. Don't send a wake key on a black
  frame unless the caption says the screen is off.
- **A SYSTEM window can replace the app's hierarchy wholesale.** When a
  BiometricPrompt, permission dialog, or the shade has focus, `describe_ui`
  returns *that* window's elements — the app's tree is gone, which reads like
  "the app broke" if you don't notice. Check the `top window:` line at the top
  of every `describe_ui` response: if it names `com.android.systemui` (or
  another package than yours), dismiss/satisfy the overlay first (e.g.
  `fingerprint_touch` for a biometric prompt, or tap its button). A key press
  that "succeeds" while such a window is up may be consumed by it — pass
  `verify_change: true` to `press_key`/`tap` to learn whether anything actually
  changed.
- **KEYCODE_HOME under automation doesn't reliably background the app.** Flows
  that depend on a background-time threshold (e.g. "token clears after 15s in
  background") reproduced only ~50% of the time via home-key automation — and
  an apparent re-lock was sometimes a *cold process start* (new pid) rather
  than the background timer firing. Confirm which one you got from logcat (a
  fresh "Bootstrap starting"-style line + pid change = cold start) before
  concluding the timer works or doesn't.

## When describe_ui can't see the element (RN / Flutter / Skia)

`describe_ui` reads the **uiautomator** hierarchy. Content painted on a **canvas**
— React Native (in some cases), Flutter, or a Skia-drawn SDK PIN pad — is not in
that hierarchy at all, so `describe_ui`/`tap_on_text` return nothing (or only a
container) for it even though the `screenshot` clearly shows it. Two tells:

- The screenshot shows rows/keys but `describe_ui` lists an empty-state or just a
  big container.
- `tap_on_text` reports "no element matching …" for text you can plainly see.

When that happens, **read the coordinates off the screenshot and tap by
coordinate**. Because the screenshot is downscaled, multiply the pixel you read
by the downscale factor (or re-take the screenshot with `max_dim: 0` for a
1:1 image) before passing to `tap`. For a numeric pad specifically, `enter_pin`
takes a `grid` (the pad's bounding box → standard 3×4 layout) or explicit
`coords` so you don't hand-tap every digit — see `android://guide/pin-and-lock`.

Note: `describe_ui` also *settles* the tree (it dumps twice and, if they differ,
waits and re-dumps) to avoid handing back a stale mid-animation snapshot. If it
still looks stale, the content is almost certainly canvas-drawn — switch to the
screenshot-coordinate approach above.

## Buttons: label vs. content-desc

Buttons often expose their label as `content_desc` on a clickable `ViewGroup`,
with the visible `text` on a *non-clickable* child. `describe_ui` and
`tap_on_text` match either, and `tap_on_text` prefers the clickable node so your
tap lands on something that responds.
