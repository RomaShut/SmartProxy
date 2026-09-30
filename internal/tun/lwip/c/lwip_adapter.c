#ifndef _POSIX_C_SOURCE
#define _POSIX_C_SOURCE 200809L
#endif

#include "lwip_adapter.h"

#include <string.h>
#include <time.h>
#include "lwip/init.h"
#include "lwip/sys.h"
#include "lwip/pbuf.h"
#include "lwip/tcp.h"
#include "lwip/udp.h"
#include "lwip/timeouts.h"

static err_t sp_output_pbuf(struct netif *n, struct pbuf *p) {
    struct sp_lwip *lw = n ? (struct sp_lwip *)n->state : NULL;
    if (!lw || !lw->packet_output) return ERR_IF;
    if (p->next == NULL) {
        lw->packet_output((const uint8_t *)p->payload, p->tot_len, lw->ctx);
        return ERR_OK;
    }
    u16_t copied = pbuf_copy_partial(p, lw->output_buf, (u16_t)p->tot_len, 0);
    if (copied != p->tot_len) return ERR_BUF;
    lw->packet_output(lw->output_buf, copied, lw->ctx);
    return ERR_OK;
}

static err_t sp_output(struct netif *n, struct pbuf *p, const ip4_addr_t *dst) {
    (void)dst;
    return sp_output_pbuf(n, p);
}

#if LWIP_IPV6
static err_t sp_output_ip6(struct netif *n, struct pbuf *p, const ip6_addr_t *dst) {
    (void)dst;
    return sp_output_pbuf(n, p);
}
#endif

static err_t sp_netif_init(struct netif *n) {
    n->name[0] = 's'; n->name[1] = 'p';
    n->mtu = 1500; n->flags = NETIF_FLAG_UP;
    n->output = sp_output;
#if LWIP_IPV6
    n->output_ip6 = sp_output_ip6;
#endif
    return ERR_OK;
}

static err_t sp_recv(void *arg, struct tcp_pcb *pcb, struct pbuf *p, err_t err) {
    struct sp_lwip *lw = (struct sp_lwip *)arg;
    if (!lw) return ERR_ABRT;
    if (!p) { if (lw->tcp_event) lw->tcp_event(pcb, ERR_OK, lw->ctx); return ERR_OK; }
    if (err != ERR_OK) { pbuf_free(p); if (lw->tcp_event) lw->tcp_event(pcb, err, lw->ctx); return err; }
    for (struct pbuf *q=p; q; q=q->next)
        if (lw->tcp_data) lw->tcp_data(pcb, (const uint8_t *)q->payload, q->len, lw->ctx);
    tcp_recved(pcb, p->tot_len);
    pbuf_free(p);
    return ERR_OK;
}

static void sp_err(void *arg, err_t err) {
    struct sp_lwip *lw = (struct sp_lwip *)arg;
    if (lw && lw->tcp_event) lw->tcp_event(NULL, err, lw->ctx);
}

void sp_lwip_tcp_set_callbacks(struct tcp_pcb *pcb, struct sp_lwip *lw) {
    if (!pcb || !lw) return;
    tcp_arg(pcb, lw);
    tcp_recv(pcb, sp_recv);
    tcp_err(pcb, sp_err);
}

int sp_lwip_init(struct sp_lwip *lw, const ip4_addr_t *ip, const ip4_addr_t *mask, const ip4_addr_t *gw) {
    if (!lw) return -1;
    lwip_init();
    memset(&lw->netif, 0, sizeof(lw->netif));
    lw->netif.state = lw;
    if (!netif_add(&lw->netif, ip, mask, gw, lw, sp_netif_init, ip_input)) return -2;
    netif_set_up(&lw->netif);
    netif_set_default(&lw->netif);
    return 0;
}

int sp_lwip_input(struct sp_lwip *lw, const void *data, uint32_t len) {
    if (!lw || !data || !len || len > 65535) return ERR_ARG;
    struct pbuf *p = pbuf_alloc(PBUF_RAW, (u16_t)len, PBUF_POOL);
    if (!p) return ERR_MEM;
    if (pbuf_take(p, data, len) != ERR_OK) { pbuf_free(p); return ERR_BUF; }
    err_t e = lw->netif.input(p, &lw->netif);
    if (e != ERR_OK) pbuf_free(p);
    return e;
}

u32_t sys_now(void) {
    struct timespec ts;
    clock_gettime(CLOCK_MONOTONIC, &ts);
    return (u32_t)(ts.tv_sec * 1000ULL + ts.tv_nsec / 1000000ULL);
}

void sp_lwip_timers(void) { sys_check_timeouts(); }

int sp_lwip_tcp_write(struct tcp_pcb *pcb, const void *data, uint32_t len) {
    if (!pcb || !data || !len) return ERR_ARG;
    err_t e = tcp_write(pcb, data, len, TCP_WRITE_FLAG_COPY);
    if (e != ERR_OK) return e;
    return tcp_output(pcb);
}

int sp_lwip_tcp_close(struct tcp_pcb *pcb) { return pcb ? tcp_close(pcb) : ERR_ARG; }
int sp_lwip_tcp_abort(struct tcp_pcb *pcb) { if (!pcb) return ERR_ARG; tcp_abort(pcb); return ERR_ABRT; }
