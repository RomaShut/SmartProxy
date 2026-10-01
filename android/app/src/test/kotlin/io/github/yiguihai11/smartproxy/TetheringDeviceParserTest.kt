package io.github.yiguihai11.smartproxy

import io.github.yiguihai11.smartproxy.shizuku.HotspotRoutingConfig
import io.github.yiguihai11.smartproxy.shizuku.inferDeviceOs
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class TetheringDeviceParserTest {

    @Test
    fun singleClientGetsNatTrafficAttributedAndHidesVirtualIp() {
        val statsJson = """
            {
              "clients": [
                {
                  "mac": "da:a1:19:22:33:44",
                  "ip": "192.168.140.204",
                  "hostname": "",
                  "type": 0,
                  "vendor": "",
                  "is_random_mac": true,
                  "os_guess": "局域网设备 (私有/随机 MAC)"
                }
              ],
              "apps": [
                {
                  "uid": 1000,
                  "conns": [
                    {
                      "proto": 6,
                      "host": "example.com",
                      "port": 443,
                      "up": 1024,
                      "down": 4096,
                      "src_ip": "192.0.2.2"
                    }
                  ]
                }
              ]
            }
        """.trimIndent()

        val devices = TetheringDeviceParser.parse(statsJson)
        assertEquals(1, devices.size)

        val dev = devices.first()
        assertEquals("192.168.140.204", dev.ip)
        assertEquals("da:a1:19:22:33:44", dev.mac)
        assertEquals(1024L, dev.upBytes)
        assertEquals(4096L, dev.downBytes)
        assertEquals(1, dev.conns.size)
        assertEquals("192.168.140.204", dev.conns.first().srcIp)
        assertEquals("example.com", dev.conns.first().host)
        assertTrue(devices.none { it.ip == HotspotRoutingConfig.SHIZUKU_TUN_IP_V4 })
    }

    @Test
    fun multipleClientsRetainNatAggregatePool() {
        val statsJson = """
            {
              "clients": [
                {
                  "mac": "da:a1:19:22:33:44",
                  "ip": "192.168.140.204",
                  "hostname": "Phone-A",
                  "type": 0,
                  "vendor": "",
                  "is_random_mac": true,
                  "os_guess": "局域网设备"
                },
                {
                  "mac": "da:a1:19:22:33:55",
                  "ip": "192.168.140.205",
                  "hostname": "Phone-B",
                  "type": 0,
                  "vendor": "",
                  "is_random_mac": true,
                  "os_guess": "局域网设备"
                }
              ],
              "apps": [
                {
                  "uid": 1000,
                  "conns": [
                    {
                      "proto": 6,
                      "host": "api.github.com",
                      "port": 443,
                      "up": 2048,
                      "down": 8192,
                      "src_ip": "192.0.2.2"
                    }
                  ]
                }
              ]
            }
        """.trimIndent()

        val devices = TetheringDeviceParser.parse(statsJson)
        assertEquals(3, devices.size)

        val natDev = devices.firstOrNull { it.ip == HotspotRoutingConfig.SHIZUKU_TUN_IP_V4 }
        org.junit.Assert.assertNotNull(natDev)
        assertEquals(2048L, natDev!!.upBytes)
        assertEquals(8192L, natDev.downBytes)
        assertTrue(natDev.osGuess.contains("NAT"))
    }

    @Test
    fun randomMacInferenceReturnsDescriptiveLabel() {
        assertEquals("局域网设备 (私有/随机 MAC)", inferDeviceOs(null, null, isRandomMac = true))
        assertEquals("未知设备", inferDeviceOs(null, null, isRandomMac = false))
        assertEquals("iOS (iPhone)", inferDeviceOs("iPhone-15", null, isRandomMac = true))
    }
}
