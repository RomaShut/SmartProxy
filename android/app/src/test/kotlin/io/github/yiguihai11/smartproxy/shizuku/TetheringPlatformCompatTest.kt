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
    fun detectsLocallyAdministeredRandomMac() {
        assertEquals(true, isLocallyAdministeredMac("02:00:00:00:00:00"))
        assertEquals(true, isLocallyAdministeredMac("da:a1:19:22:33:44"))
        assertEquals(true, isLocallyAdministeredMac("f6:44:88:aa:bb:cc"))
        assertEquals(false, isLocallyAdministeredMac("00:1a:11:22:33:44"))
        assertEquals(false, isLocallyAdministeredMac("28:cf:e9:11:22:33"))
        assertEquals(false, isLocallyAdministeredMac(""))
    }

    @Test
    fun lookupsMacVendorFromOui() {
        assertEquals("Apple", lookupMacVendor("28:cf:e9:11:22:33"))
        assertEquals("Google", lookupMacVendor("00:1a:11:22:33:44"))
        assertEquals("Espressif", lookupMacVendor("24:0a:c4:aa:bb:cc"))
        assertEquals(null, lookupMacVendor("ff:ff:ff:11:22:33"))
    }

    @Test
    fun infersDeviceOsFromHostnameAndVendor() {
        assertEquals("iOS (iPhone)", inferDeviceOs("iPhone 15 Pro", null))
        assertEquals("iPadOS (iPad)", inferDeviceOs("iPad-Air", null))
        assertEquals("macOS", inferDeviceOs("MacBook-Pro", null))
        assertEquals("Windows PC", inferDeviceOs("DESKTOP-ABC1234", null))
        assertEquals("Android (Samsung)", inferDeviceOs("Galaxy-S24", null))
        assertEquals("Android (Xiaomi)", inferDeviceOs("Xiaomi-14", null))
        assertEquals("IoT (ESP)", inferDeviceOs("esp_living_room", null))
        assertEquals("Apple (iOS / macOS)", inferDeviceOs(null, "Apple"))
        assertEquals("Windows PC", inferDeviceOs(null, "Microsoft"))
        assertEquals("IoT (ESP)", inferDeviceOs(null, "Espressif"))
        assertEquals("未知设备", inferDeviceOs(null, null))
    }

    @Test
    fun mergesSystemAndArpClients() {
        val sysClients = listOf(
            createTetheredClientInfo(mac = "28:cf:e9:dd:ee:01", ip = "192.168.43.10", hostname = "iPhone", tetheringType = 0)
        )
        val arpClients = listOf(
            createTetheredClientInfo(mac = "28:cf:e9:dd:ee:01", ip = "192.168.43.10", hostname = null, tetheringType = -1),
            createTetheredClientInfo(mac = "da:a1:19:dd:ee:02", ip = "192.168.43.20", hostname = null, tetheringType = -1)
        )
        val merged = mergeTetheredClients(sysClients, arpClients)
        assertEquals(2, merged.size)
        val first = merged.first { it.ip == "192.168.43.10" }
        assertEquals("iPhone", first.hostname)
        assertEquals("Apple", first.vendor)
        assertEquals(false, first.isRandomMac)
        assertEquals("iOS (iPhone)", first.osGuess)

        val second = merged.first { it.ip == "192.168.43.20" }
        assertEquals("da:a1:19:dd:ee:02", second.mac)
        assertEquals(true, second.isRandomMac)
        assertEquals(null, second.vendor) // random MAC masks physical vendor OUI
    }
}
