package io.github.yiguihai11.smartproxy.shizuku

import org.junit.Assert.assertEquals
import org.junit.Test

class TetheringPlatformCompatTest {

    @Test
    fun usesPublicTetheringApiStartingAtApi36() {
        assertEquals(false, isPublicTetheringApiLevel(33))
        assertEquals(false, isPublicTetheringApiLevel(35))
        assertEquals(true, isPublicTetheringApiLevel(36))
        assertEquals(true, isPublicTetheringApiLevel(37))
    }

    @Test
    fun buildsOnlyValidTetheringTypeBits() {
        assertEquals(1, tetheringTypeBit(0))
        assertEquals(1 shl 15, tetheringTypeBit(15))
        assertEquals(0, tetheringTypeBit(-1))
        assertEquals(0, tetheringTypeBit(31))
    }

    @Test
    fun acceptsOnlyTheOwnedUpstreamInterface() {
        assertEquals(true, TetheringPlatformCompat.isProtectedUpstream("testtun17", "testtun17"))
        assertEquals(true, TetheringPlatformCompat.isProtectedUpstream(" testtun17 ", "testtun17"))
        assertEquals(
            false,
            TetheringPlatformCompat.isProtectedUpstream("testtun17, testtun17", "testtun17"),
        )
        assertEquals(
            false,
            TetheringPlatformCompat.isProtectedUpstream("testtun17, testtun17, testtun17", "testtun17"),
        )
        assertEquals(false, TetheringPlatformCompat.isProtectedUpstream("", "testtun17"))
        assertEquals(false, TetheringPlatformCompat.isProtectedUpstream("eth0", "testtun17"))
        assertEquals(
            false,
            TetheringPlatformCompat.isProtectedUpstream("testtun17, eth0", "testtun17"),
        )
    }

    @Test
    fun mergesSystemAndArpClients() {
        val sysClients = listOf(
            TetheredClientInfo(mac = "aa:bb:cc:dd:ee:01", ip = "192.168.43.10", hostname = "iPhone", tetheringType = 0)
        )
        val arpClients = listOf(
            TetheredClientInfo(mac = "aa:bb:cc:dd:ee:01", ip = "192.168.43.10", hostname = null, tetheringType = -1),
            TetheredClientInfo(mac = "aa:bb:cc:dd:ee:02", ip = "192.168.43.20", hostname = null, tetheringType = -1)
        )
        val merged = mergeTetheredClients(sysClients, arpClients)
        assertEquals(2, merged.size)
        val first = merged.first { it.ip == "192.168.43.10" }
        assertEquals("iPhone", first.hostname)
        val second = merged.first { it.ip == "192.168.43.20" }
        assertEquals("aa:bb:cc:dd:ee:02", second.mac)
    }
}
