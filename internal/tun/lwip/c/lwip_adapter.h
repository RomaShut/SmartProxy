#ifndef SMARTPROXY_LWIP_ADAPTER_H
#define SMARTPROXY_LWIP_ADAPTER_H
#include <stdint.h>
#include "lwip/netif.h"
#include "lwip/tcp.h"
#ifdef __cplusplus
extern "C" {
#endif
typedef void (*sp_lwip_packet_output_fn)(const uint8_t *, uint32_t, void *);
typedef void (*sp_lwip_tcp_data_fn)(struct tcp_pcb *, const uint8_t *, uint32_t, void *);
typedef void (*sp_lwip_tcp_event_fn)(struct tcp_pcb *, int, void *);
struct sp_lwip {
    struct netif netif;
    sp_lwip_packet_output_fn packet_output;
    sp_lwip_tcp_data_fn tcp_data;
    sp_lwip_tcp_event_fn tcp_event;
    void *ctx;
};
int sp_lwip_init(struct sp_lwip *, const ip4_addr_t *, const ip4_addr_t *, const ip4_addr_t *);
int sp_lwip_input(struct sp_lwip *, const void *, uint32_t);
void sp_lwip_timers(void);
int sp_lwip_tcp_write(struct tcp_pcb *, const void *, uint32_t);
int sp_lwip_tcp_close(struct tcp_pcb *);
int sp_lwip_tcp_abort(struct tcp_pcb *);
#ifdef __cplusplus
}
#endif
#endif
