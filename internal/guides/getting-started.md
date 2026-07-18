# Getting started: booting and connecting

## Boot an emulator

1. `list_avds` → the AVDs you can boot.
2. `boot_emulator` with `avd: "<name>"`. It launches detached (so the emulator
   outlives the call), waits for full boot by default, and returns the device
   **serial** (e.g. `emulator-5554`).
3. Confirm with `list_devices` — the device should be in state `device`.

If you already have an emulator running, skip straight to `list_devices`.

## Targeting a device (the `serial` argument)

Every device-facing tool takes an optional `serial`. When exactly one device is
attached you can omit it — the tool auto-selects that device. With **multiple**
devices attached, you must pass `serial` (from `list_devices`) or the tool
returns an actionable error telling you to pick one.

## A first real interaction

```
boot_emulator {avd: "Pixel_9a"}          → emulator-5554
screenshot                               → look at the home screen
describe_ui                              → elements with centers
tap_on_text {text: "Settings"}           → opens Settings
screenshot                               → confirm the screen changed
```

## Driving an RN/Expo dev build? Do this first

A dev client that cannot reach its Metro dev server **silently falls back to
the embedded bundle** — the app runs, but none of your code edits are in it,
and you can burn a whole session "testing" code that was never loaded. Before
driving a dev build:

```
adb_reverse {device_port: 8081}         → emulator can reach Metro on the host
launch_dev_client {scheme: "myapp"}     → open the dev build straight at Metro,
                                          skipping the Dev Launcher's server picker
```

`launch_dev_client` builds the `<scheme>://expo-development-client/?url=…` deep
link for you (host/port default to `localhost:8081`); for a plain Expo Go client
use `open_url` with the `exp://` URL instead. Suspect an embedded-bundle fallback
whenever edits appear to have no effect or expected logs are absent.

## Tips

- `screenshot` auto-downscales large screens (default max dimension 760px) so the
  image reader accepts it. Set `max_dim: 0` for full resolution if you need it.
- Prefer `tap_on_text` over `tap` when you know the label — it looks up the true
  pixel center for you, so you never miscalculate a coordinate.
- Read the `android://guide/driving` resource for the full observe→act loop and
  the gotchas that waste turns.
