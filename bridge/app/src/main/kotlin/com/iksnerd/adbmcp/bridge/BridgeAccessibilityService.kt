package com.iksnerd.adbmcp.bridge

import android.accessibilityservice.AccessibilityService
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.os.Build
import android.util.Log
import android.view.accessibility.AccessibilityEvent
import android.view.accessibility.AccessibilityNodeInfo
import org.json.JSONObject

/**
 * A plain AccessibilityService (no am-instrument/UiAutomator server) that
 * performs a real accessibility click on request from adb-mcp, for views
 * that ignore raw coordinate `input tap` (e.g. Compose/RN NativeTabs bars).
 * Driven entirely by a local `adb shell am broadcast`; the result is
 * reported back as one JSON line via logcat (tag AdbMcpBridge).
 */
class BridgeAccessibilityService : AccessibilityService() {

    companion object {
        private const val TAG = "AdbMcpBridge"
        const val ACTION_CLICK = "com.iksnerd.adbmcp.bridge.ACTION_CLICK"
        private const val MAX_ANCESTOR_WALK = 20
    }

    private val receiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context, intent: Intent) {
            if (intent.action != ACTION_CLICK) return
            handleClick(
                resourceId = intent.getStringExtra("resource_id"),
                text = intent.getStringExtra("text"),
                partial = intent.getBooleanExtra("partial", true),
            )
        }
    }

    override fun onServiceConnected() {
        super.onServiceConnected()
        val filter = IntentFilter(ACTION_CLICK)
        if (Build.VERSION.SDK_INT >= 33) {
            registerReceiver(receiver, filter, Context.RECEIVER_EXPORTED)
        } else {
            @Suppress("UnspecifiedRegisterReceiverFlag")
            registerReceiver(receiver, filter)
        }
        Log.i(TAG, "service connected")
    }

    override fun onDestroy() {
        runCatching { unregisterReceiver(receiver) }
        super.onDestroy()
    }

    override fun onAccessibilityEvent(event: AccessibilityEvent?) {
        // No event-driven behavior needed; the click path only needs
        // rootInActiveWindow, which is available any time the service is
        // connected, not tied to a specific event.
    }

    override fun onInterrupt() {}

    private fun handleClick(resourceId: String?, text: String?, partial: Boolean) {
        val result = JSONObject()
        val root = rootInActiveWindow
        if (root == null) {
            result.put("ok", false).put("reason", "no_root_window")
            Log.i(TAG, result.toString())
            return
        }

        val matches = mutableListOf<AccessibilityNodeInfo>()
        collectMatches(root, resourceId, text, partial, matches)

        if (matches.isEmpty()) {
            result.put("ok", false).put("reason", "no_match")
                .put("resource_id", resourceId ?: JSONObject.NULL)
                .put("text", text ?: JSONObject.NULL)
            Log.i(TAG, result.toString())
            return
        }

        val clickableMatch = matches.firstOrNull { it.isClickable }
        val target = clickableMatch ?: findClickableAncestor(matches[0]) ?: matches[0]

        val actionResult = target.performAction(AccessibilityNodeInfo.ACTION_CLICK)

        result.put("ok", actionResult)
            .put("matched_text", (matches[0].text ?: matches[0].contentDescription)?.toString() ?: JSONObject.NULL)
            .put("matched_resource_id", matches[0].viewIdResourceName ?: JSONObject.NULL)
            .put("clicked_own_node", clickableMatch != null)
            .put("clickable", target.isClickable)
            .put("action_result", actionResult)
        Log.i(TAG, result.toString())
    }

    /** DFS collecting every node whose id/text/content-desc matches, in traversal order. */
    private fun collectMatches(
        node: AccessibilityNodeInfo,
        resourceId: String?,
        text: String?,
        partial: Boolean,
        out: MutableList<AccessibilityNodeInfo>,
    ) {
        if (matches(node, resourceId, text, partial)) out.add(node)
        for (i in 0 until node.childCount) {
            val child = node.getChild(i) ?: continue
            collectMatches(child, resourceId, text, partial, out)
        }
    }

    private fun matches(node: AccessibilityNodeInfo, resourceId: String?, text: String?, partial: Boolean): Boolean {
        if (!resourceId.isNullOrEmpty()) {
            val id = node.viewIdResourceName ?: return false
            return if (partial) id.contains(resourceId) else id == resourceId
        }
        if (!text.isNullOrEmpty()) {
            val candidates = listOfNotNull(node.text?.toString(), node.contentDescription?.toString())
            return candidates.any { c ->
                if (partial) c.contains(text, ignoreCase = true) else c.equals(text, ignoreCase = true)
            }
        }
        return false
    }

    /** Walks up the node's ancestor chain looking for the nearest clickable node. */
    private fun findClickableAncestor(node: AccessibilityNodeInfo): AccessibilityNodeInfo? {
        var cur = node.parent
        var steps = 0
        while (cur != null && steps < MAX_ANCESTOR_WALK) {
            if (cur.isClickable) return cur
            cur = cur.parent
            steps++
        }
        return null
    }
}
