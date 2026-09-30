#include "lwip_adapter.h"

#include <string.h>
#include "lwip/init.h"
#include "lwip/pbuf.h"
#include "lwip/tcp.h"
#include "lwip/udp.h"
#include "lwip/timeouts.h"

static err_t sp_output(struct netif *n, struct pbuf *p, const ip4_addr_t *dst) {
    struct sp_lwip *lw = n ? (struct sp_lwip *)n->state : NULL;
    (void)dst;
    if (!lw || !lw->packet_output) return ERR_IF;
    uint8_t buf[65536];
    uint32_t off = 0;
    for (struct pbuf *q = p; q; q = q->next) {
        if (off + q->len > sizeof(buf)) return ERR_BUF;
        memcpy(buf + off, q->payload, q->len);
        off += q->len;
    }
    lw->packet_output(buf, off, lw->ctx);
    return ERR_OK;
}

static err_t sp_netif_init(struct netif *n) {
    n->name[0] = 's'; n->name[1] = 'p';
    n->mtu = 1500; n->flags = NETIF_FLAG_UP;
    n->output = sp_output;
    return ERR_OK;
}

static err_t sp_recv(void *arg, struct tcp_pcb *pcb, struct pbuf *p, err_t err) {
    struct sp_lwip *lw = arg;
    if (!lw) return ERR_ABRT;
    if (!p) { if (lw->tcp_event) lw->tcp_event(pcb, ERR_OK, lw->ctx); return ERR_OK; }
    if (err != ERR_OK) { pbuf_free(p); if (lw->tcp_event) lw->tcp_event(pcb, err, lw->ctx); return err; }
    for (struct pbuf *q=p; q; q=q->next)
        if (lw->tcp_data) lw->tcp_data(pcb, q->payload, q->len, lw->ctx);
    tcp_recved(pcb, p->tot_len);
    pbuf_free(p);
    return ERR_OK;
}

static void sp_err(void *arg, err_t err) {
    struct sp_lwip *lw = arg;
    if (lw && lw->tcp_event) lw->tcp_event(NULL, err, lw->ctx);
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

void sp_lwip_timers(void) { sys_check_timeouts(); }

int sp_lwip_tcp_write(struct tcp_pcb *pcb, const void *data, uint32_t len) {
    if (!pcb || !data || !len) return ERR_ARG;
    err_t e = tcp_write(pcb, data, len, TCP_WRITE_FLAG_COPY);
    if (e != ERR_OK) return e;
    return tcp_output(pcb);
}

int sp_lwip_tcp_close(struct tcp_pcb *pcb) { return pcb ? tcp_close(pcb) : ERR_ARG; }
int sp_lwip_tcp_abort(struct tcp_pcb *pcb) { if (!pcb) return ERR_ARG; tcp_abort(pcb); return ERR_ABRT; }
