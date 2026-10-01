package io.github.yiguihai11.smartproxy.shizuku

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Test

class TetheringConfigRelocationTest {

    @Test
    fun relocatesRoutingFilesAndDisablesListenersForPrivilegedRouting() {
        val original = """
            {
              "listen": {
                "admin_port": 9090,
                "admin_socket": "/tmp/admin.sock",
                "port": 1080
              },
              "routing": {
                "chnroute_file": "/data/user/0/io.github.yiguihai11.smartproxy/cache/chnroute.txt",
                "acl_file": "/data/user/0/io.github.yiguihai11.smartproxy/cache/acl.txt"
              }
            }
        """.trimIndent()

        val stagedDir = "/data/local/tmp/smartproxy-tethering-assets"
        val relocated = ShizukuTetheringService.relocateRoutingFilesForShell(original, stagedDir)
        val json = JSONObject(relocated)

        val routing = json.getJSONObject("routing")
        assertEquals("$stagedDir/chnroute.txt", routing.getString("chnroute_file"))
        assertEquals("$stagedDir/acl.txt", routing.getString("acl_file"))

        val listen = json.getJSONObject("listen")
        assertEquals(0, listen.getInt("admin_port"))
        assertEquals("", listen.getString("admin_socket"))
        assertEquals(0, listen.getInt("port"))
    }

    @Test
    fun insertsListenBlockIfMissingToPreventBindingDefaultPorts() {
        val original = """{"routing": {}}"""
        val stagedDir = "/data/local/tmp/smartproxy-tethering-assets"
        val relocated = ShizukuTetheringService.relocateRoutingFilesForShell(original, stagedDir)
        val json = JSONObject(relocated)

        val listen = json.getJSONObject("listen")
        assertEquals(0, listen.getInt("admin_port"))
        assertEquals("", listen.getString("admin_socket"))
        assertEquals(0, listen.getInt("port"))
    }
}
