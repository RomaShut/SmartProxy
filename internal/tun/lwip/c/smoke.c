#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "lwip_adapter.h"

static int g_output_packets = 0;
static uint8_t g_last_output[1500];
static uint32_t g_last_output_len = 0;

static uint64_t g_accepted_id = 0;
static uint16_t g_accepted_dst_port = 0;
static uint16_t g_accepted_src_port = 0;

static void on_output(const uint8_t *p, uint32_t n, uint64_t ctx_id) {
    (void)ctx_id;
    g_output_packets++;
    if (n <= sizeof(g_last_output)) {
        memcpy(g_last_output, p, n);
        g_last_output_len = n;
    }
}

static void on_accept(uint64_t conn_id, int is_ipv6,
                      const void *src_ip, uint16_t src_port,
                      const void *dst_ip, uint16_t dst_port,
                      uint64_t ctx_id) {
    (void)is_ipv6; (void)src_ip; (void)dst_ip; (void)ctx_id;
    g_accepted_id = conn_id;
    g_accepted_src_port = src_port;
    g_accepted_dst_port = dst_port;
}

static void on_recv(uint64_t conn_id, const uint8_t *data, uint16_t len, uint64_t ctx_id) {
    (void)conn_id; (void)data; (void)len; (void)ctx_id;
}

static void on_sent(uint64_t conn_id, uint16_t len, uint64_t ctx_id) {
    (void)conn_id; (void)len; (void)ctx_id;
}

static void on_err(uint64_t conn_id, int err, uint64_t ctx_id) {
    (void)conn_id; (void)err; (void)ctx_id;
}

static uint64_t g_udp_recv_id = 0;
static uint16_t g_udp_recv_src_port = 0;
static uint16_t g_udp_recv_dst_port = 0;
static uint8_t g_last_udp_data[1500];
static uint16_t g_last_udp_len = 0;

static void on_udp_recv(uint64_t conn_id, int is_ipv6,
                        const void *src_ip, uint16_t src_port,
                        const void *dst_ip, uint16_t dst_port,
                        const uint8_t *data, uint16_t len,
                        uint64_t ctx_id) {
    (void)is_ipv6; (void)src_ip; (void)dst_ip; (void)ctx_id;
    g_udp_recv_id = conn_id;
    g_udp_recv_src_port = src_port;
    g_udp_recv_dst_port = dst_port;
    if (len <= sizeof(g_last_udp_data)) {
        memcpy(g_last_udp_data, data, len);
        g_last_udp_len = len;
    }
}

static uint32_t build_ipv4_tcp(uint8_t *buf,
                               uint32_t src_ip, uint16_t src_port,
                               uint32_t dst_ip, uint16_t dst_port,
                               uint32_t seq, uint32_t ack,
                               uint8_t flags,
                               const uint8_t *payload, uint16_t payload_len) {
    uint16_t total_len = 20 + 20 + payload_len;
    memset(buf, 0, total_len);

    // IPv4 Header
    buf[0] = 0x45; // Version 4, IHL 5
    buf[1] = 0x00; // TOS
    buf[2] = (uint8_t)(total_len >> 8);
    buf[3] = (uint8_t)(total_len & 0xFF);
    buf[4] = 0x12; buf[5] = 0x34; // ID
    buf[6] = 0x40; buf[7] = 0x00; // DF
    buf[8] = 64;   // TTL
    buf[9] = 6;    // Protocol = TCP
    memcpy(buf + 12, &src_ip, 4);
    memcpy(buf + 16, &dst_ip, 4);

    // TCP Header
    buf[20] = (uint8_t)(src_port >> 8);
    buf[21] = (uint8_t)(src_port & 0xFF);
    buf[22] = (uint8_t)(dst_port >> 8);
    buf[23] = (uint8_t)(dst_port & 0xFF);
    buf[24] = (uint8_t)(seq >> 24);
    buf[25] = (uint8_t)((seq >> 16) & 0xFF);
    buf[26] = (uint8_t)((seq >> 8) & 0xFF);
    buf[27] = (uint8_t)(seq & 0xFF);
    buf[28] = (uint8_t)(ack >> 24);
    buf[29] = (uint8_t)((ack >> 16) & 0xFF);
    buf[30] = (uint8_t)((ack >> 8) & 0xFF);
    buf[31] = (uint8_t)(ack & 0xFF);
    buf[32] = 0x50; // Data offset 5
    buf[33] = flags;
    buf[34] = 0xFF; buf[35] = 0xFF; // Window 65535

    if (payload && payload_len > 0) {
        memcpy(buf + 40, payload, payload_len);
    }
    return total_len;
}

static uint32_t build_ipv4_udp(uint8_t *buf,
                               uint32_t src_ip, uint16_t src_port,
                               uint32_t dst_ip, uint16_t dst_port,
                               const uint8_t *payload, uint16_t payload_len) {
    uint16_t total_len = 20 + 8 + payload_len;
    memset(buf, 0, total_len);

    // IPv4 Header
    buf[0] = 0x45;
    buf[1] = 0x00;
    buf[2] = (uint8_t)(total_len >> 8);
    buf[3] = (uint8_t)(total_len & 0xFF);
    buf[4] = 0x56; buf[5] = 0x78;
    buf[6] = 0x40; buf[7] = 0x00;
    buf[8] = 64;
    buf[9] = 17; // Protocol = UDP
    memcpy(buf + 12, &src_ip, 4);
    memcpy(buf + 16, &dst_ip, 4);

    // UDP Header
    buf[20] = (uint8_t)(src_port >> 8);
    buf[21] = (uint8_t)(src_port & 0xFF);
    buf[22] = (uint8_t)(dst_port >> 8);
    buf[23] = (uint8_t)(dst_port & 0xFF);
    uint16_t udp_len = 8 + payload_len;
    buf[24] = (uint8_t)(udp_len >> 8);
    buf[25] = (uint8_t)(udp_len & 0xFF);
    buf[26] = 0; buf[27] = 0; // Checksum 0 is allowed in IPv4 UDP

    if (payload && payload_len > 0) {
        memcpy(buf + 28, payload, payload_len);
    }
    return total_len;
}

static uint16_t calc_checksum(const uint8_t *b, uint32_t len) {
    uint32_t sum = 0;
    for (uint32_t i = 0; i < len - 1; i += 2) {
        sum += ((uint32_t)b[i] << 8) | b[i + 1];
    }
    if (len % 2 == 1) {
        sum += (uint32_t)b[len - 1] << 8;
    }
    while (sum >> 16) {
        sum = (sum >> 16) + (sum & 0xffff);
    }
    return (uint16_t)~sum;
}

static uint32_t build_ipv4_icmp(uint8_t *buf,
                                uint32_t src_ip, uint32_t dst_ip,
                                uint8_t type, uint8_t code,
                                uint16_t id, uint16_t seq,
                                const uint8_t *payload, uint16_t payload_len) {
    uint16_t total_len = 20 + 8 + payload_len;
    memset(buf, 0, total_len);

    // IPv4 Header
    buf[0] = 0x45;
    buf[1] = 0x00;
    buf[2] = (uint8_t)(total_len >> 8);
    buf[3] = (uint8_t)(total_len & 0xFF);
    buf[4] = 0x33; buf[5] = 0x44;
    buf[6] = 0x40; buf[7] = 0x00;
    buf[8] = 64;
    buf[9] = 1; // Protocol = ICMP
    memcpy(buf + 12, &src_ip, 4);
    memcpy(buf + 16, &dst_ip, 4);
    uint16_t ip_chk = calc_checksum(buf, 20);
    buf[10] = (uint8_t)(ip_chk >> 8);
    buf[11] = (uint8_t)(ip_chk & 0xFF);

    // ICMP Header
    buf[20] = type;
    buf[21] = code;
    buf[24] = (uint8_t)(id >> 8);
    buf[25] = (uint8_t)(id & 0xFF);
    buf[26] = (uint8_t)(seq >> 8);
    buf[27] = (uint8_t)(seq & 0xFF);
    if (payload && payload_len > 0) {
        memcpy(buf + 28, payload, payload_len);
    }
    uint16_t icmp_chk = calc_checksum(buf + 20, 8 + payload_len);
    buf[22] = (uint8_t)(icmp_chk >> 8);
    buf[23] = (uint8_t)(icmp_chk & 0xFF);

    return total_len;
}

int main(void) {
    struct sp_lwip lw;
    memset(&lw, 0, sizeof(lw));
    lw.packet_output = on_output;
    lw.tcp_accept = on_accept;
    lw.tcp_recv = on_recv;
    lw.tcp_sent = on_sent;
    lw.tcp_err = on_err;
    lw.udp_recv = on_udp_recv;

    ip4_addr_t ip, mask, gw;
    IP4_ADDR(&ip, 10, 0, 0, 2);
    IP4_ADDR(&mask, 255, 255, 255, 0);
    IP4_ADDR(&gw, 10, 0, 0, 1);

    if (sp_lwip_init(&lw, &ip, &mask, &gw) != 0) {
        fprintf(stderr, "sp_lwip_init failed\n");
        return 1;
    }

    sp_lwip_timers();

    // Test 1: Send TCP SYN from client (10.0.0.2:45678) to internet target (1.1.1.1:80)
    uint32_t client_ip;
    IP4_ADDR((ip4_addr_t *)&client_ip, 10, 0, 0, 2);
    uint32_t target_ip;
    IP4_ADDR((ip4_addr_t *)&target_ip, 1, 1, 1, 1);

    uint8_t packet[1500];
    uint32_t pkt_len = build_ipv4_tcp(packet, client_ip, 45678, target_ip, 80, 1000, 0, 0x02, NULL, 0);

    g_output_packets = 0;
    int err = sp_lwip_input(&lw, packet, pkt_len);
    if (err != 0) {
        fprintf(stderr, "sp_lwip_input SYN failed: %d\n", err);
        return 1;
    }

    if (g_output_packets != 1) {
        fprintf(stderr, "expected 1 output packet (SYN/ACK), got %d\n", g_output_packets);
        return 1;
    }

    // Verify SYN/ACK: TCP flags at byte 33 should be 0x12 (SYN | ACK)
    uint8_t flags = g_last_output[33];
    if ((flags & 0x12) != 0x12) {
        fprintf(stderr, "expected SYN/ACK (0x12), got 0x%02x\n", flags);
        return 1;
    }

    uint32_t syn_ack_seq = ((uint32_t)g_last_output[24] << 24) |
                           ((uint32_t)g_last_output[25] << 16) |
                           ((uint32_t)g_last_output[26] << 8) |
                           (uint32_t)g_last_output[27];

    // Test 2: Client sends ACK to complete 3-way handshake
    pkt_len = build_ipv4_tcp(packet, client_ip, 45678, target_ip, 80, 1001, syn_ack_seq + 1, 0x10, NULL, 0);
    err = sp_lwip_input(&lw, packet, pkt_len);
    if (err != 0) {
        fprintf(stderr, "sp_lwip_input ACK failed: %d\n", err);
        return 1;
    }

    // Verify on_accept was called
    if (g_accepted_id == 0) {
        fprintf(stderr, "expected connection to be accepted, but on_accept was not called\n");
        return 1;
    }
    if (g_accepted_src_port != 45678 || g_accepted_dst_port != 80) {
        fprintf(stderr, "accepted ports mismatch: src=%u, dst=%u\n", g_accepted_src_port, g_accepted_dst_port);
        return 1;
    }

    // Test 3: Write data from server side back to client
    int before_write_outputs = g_output_packets;
    int written = sp_lwip_tcp_write(&lw, g_accepted_id, "PONG", 4);
    if (written != 4) {
        fprintf(stderr, "sp_lwip_tcp_write failed: %d\n", written);
        return 1;
    }
    if (g_output_packets <= before_write_outputs) {
        fprintf(stderr, "expected data packet output, but none sent\n");
        return 1;
    }

    // Test 4: Gracefully close connection
    if (sp_lwip_tcp_close(&lw, g_accepted_id) != 0) {
        fprintf(stderr, "sp_lwip_tcp_close failed\n");
        return 1;
    }

    // Test 5: UDP datagram from client (10.0.0.2:53000) to DNS (8.8.8.8:53)
    uint32_t dns_ip;
    IP4_ADDR((ip4_addr_t *)&dns_ip, 8, 8, 8, 8);
    const char *dns_query = "PING_UDP";
    pkt_len = build_ipv4_udp(packet, client_ip, 53000, dns_ip, 53, (const uint8_t *)dns_query, strlen(dns_query));

    g_udp_recv_id = 0;
    g_udp_recv_src_port = 0;
    g_udp_recv_dst_port = 0;
    g_last_udp_len = 0;

    err = sp_lwip_input(&lw, packet, pkt_len);
    if (err != 0) {
        fprintf(stderr, "sp_lwip_input UDP failed: %d\n", err);
        return 1;
    }

    if (g_udp_recv_id == 0) {
        fprintf(stderr, "expected UDP datagram received, but on_udp_recv was not called\n");
        return 1;
    }
    if (g_udp_recv_src_port != 53000 || g_udp_recv_dst_port != 53) {
        fprintf(stderr, "UDP ports mismatch: src=%u (want 53000), dst=%u (want 53)\n", g_udp_recv_src_port, g_udp_recv_dst_port);
        return 1;
    }
    if (g_last_udp_len != strlen(dns_query) || memcmp(g_last_udp_data, dns_query, strlen(dns_query)) != 0) {
        fprintf(stderr, "UDP payload mismatch\n");
        return 1;
    }

    // Test 6: Reply to UDP client pretending to be 8.8.8.8:53
    int before_udp_reply_outputs = g_output_packets;
    const char *dns_resp = "PONG_UDP";
    int u_sent = sp_lwip_udp_send(&lw, g_udp_recv_id, 0, &dns_ip, 53, dns_resp, strlen(dns_resp));
    if (u_sent != (int)strlen(dns_resp)) {
        fprintf(stderr, "sp_lwip_udp_send failed: %d\n", u_sent);
        return 1;
    }
    if (g_output_packets <= before_udp_reply_outputs) {
        fprintf(stderr, "expected UDP reply output packet, but none sent\n");
        return 1;
    }

    // Verify output packet: IPv4 (protocol 17 at byte 9), source 8.8.8.8:53, destination 10.0.0.2:53000
    if (g_last_output[9] != 17) {
        fprintf(stderr, "expected UDP output (proto 17), got %u\n", g_last_output[9]);
        return 1;
    }
    uint16_t resp_src_port = ((uint16_t)g_last_output[20] << 8) | g_last_output[21];
    uint16_t resp_dst_port = ((uint16_t)g_last_output[22] << 8) | g_last_output[23];
    if (resp_src_port != 53 || resp_dst_port != 53000) {
        fprintf(stderr, "UDP response port mismatch: src=%u (want 53), dst=%u (want 53000)\n", resp_src_port, resp_dst_port);
        return 1;
    }

    // Test 7: Close UDP connection
    if (sp_lwip_udp_close(&lw, g_udp_recv_id) != 0) {
        fprintf(stderr, "sp_lwip_udp_close failed\n");
        return 1;
    }

    // Test 8: ICMP Echo Request (10.0.0.2 -> 8.8.8.8)
    const char *ping_msg = "PING_SMOKE";
    pkt_len = build_ipv4_icmp(packet, client_ip, dns_ip, 8, 0, 0x1234, 1, (const uint8_t *)ping_msg, strlen(ping_msg));
    int before_icmp_outputs = g_output_packets;
    err = sp_lwip_input(&lw, packet, pkt_len);
    if (err != 0) {
        fprintf(stderr, "sp_lwip_input ICMP failed: %d\n", err);
        return 1;
    }
    if (g_output_packets <= before_icmp_outputs) {
        fprintf(stderr, "expected ICMP Echo Reply output, but none sent\n");
        return 1;
    }
    // Verify output: protocol 1 (ICMP), type 0 (Echo Reply), code 0
    if (g_last_output[9] != 1) {
        fprintf(stderr, "expected ICMP output (proto 1), got %u\n", g_last_output[9]);
        return 1;
    }
    if (g_last_output[20] != 0 || g_last_output[21] != 0) {
        fprintf(stderr, "expected ICMP Echo Reply (type 0, code 0), got type=%u, code=%u\n", g_last_output[20], g_last_output[21]);
        return 1;
    }
    if (memcmp(g_last_output + 28, ping_msg, strlen(ping_msg)) != 0) {
        fprintf(stderr, "ICMP payload mismatch\n");
        return 1;
    }

    sp_lwip_free(&lw);

    puts("smartproxy lwIP smoke test: ALL PASSED (TCP, UDP, ICMP Echo Reply verified)");
    return 0;
}
