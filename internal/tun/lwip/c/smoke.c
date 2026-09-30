#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include "lwip_adapter.h"

static void output(const uint8_t *p, uint32_t n, void *ctx) {
    (void)p; (void)n; (void)ctx;
}

int main(void) {
    struct sp_lwip lw;
    memset(&lw, 0, sizeof(lw));
    lw.packet_output = output;

    ip4_addr_t ip, mask, gw;
    IP4_ADDR(&ip, 10, 0, 0, 2);
    IP4_ADDR(&mask, 255, 255, 255, 0);
    IP4_ADDR(&gw, 10, 0, 0, 1);

    if (sp_lwip_init(&lw, &ip, &mask, &gw) != 0) {
        fprintf(stderr, "sp_lwip_init failed\n");
        return 1;
    }

    sp_lwip_timers();
    puts("smartproxy lwIP smoke test: OK");
    return 0;
}
