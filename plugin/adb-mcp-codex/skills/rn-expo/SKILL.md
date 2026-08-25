---
name: rn-expo
description: The RN/Expo dev-build recipe for adb-mcp — adb_reverse before launching (or the dev client silently runs its embedded bundle), app_state to confirm it is on Metro, on the RIGHT Metro, and not serving stale JavaScript after a git checkout, then reload_app. Use whenever driving a React Native or Expo dev build, or when code edits appear to have no effect on the device.
---

Read the `android://guide/rn-expo` resource from the `adb` MCP server before driving a React Native or Expo dev build — the ordering is the part that costs sessions.
