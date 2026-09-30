//go:build with_lwip && cgo

/*
 * Unity build compilation unit for lwIP and SmartProxy lwIP adapter.
 *
 * Direct compilation via cgo ensures seamless cross-compilation across all
 * architectures (Linux amd64/arm64, Android arm64-v8a/armeabi-v7a/x86_64/x86)
 * without needing precompiled static archives (.a).
 */

#include "c/lwipopts.h"

/* Core lwIP */
#include "../../../third_party/lwip/src/core/init.c"
#include "../../../third_party/lwip/src/core/def.c"
#include "../../../third_party/lwip/src/core/inet_chksum.c"
#include "../../../third_party/lwip/src/core/ip.c"
#include "../../../third_party/lwip/src/core/mem.c"
#include "../../../third_party/lwip/src/core/memp.c"
#include "../../../third_party/lwip/src/core/netif.c"
#include "../../../third_party/lwip/src/core/pbuf.c"
#include "../../../third_party/lwip/src/core/raw.c"
#include "../../../third_party/lwip/src/core/stats.c"
#include "../../../third_party/lwip/src/core/sys.c"
#include "../../../third_party/lwip/src/core/tcp.c"
#include "../../../third_party/lwip/src/core/tcp_in.c"
#include "../../../third_party/lwip/src/core/tcp_out.c"
#include "../../../third_party/lwip/src/core/timeouts.c"
#include "../../../third_party/lwip/src/core/udp.c"

/* IPv4 */
#include "../../../third_party/lwip/src/core/ipv4/acd.c"
#include "../../../third_party/lwip/src/core/ipv4/autoip.c"
#include "../../../third_party/lwip/src/core/ipv4/dhcp.c"
#include "../../../third_party/lwip/src/core/ipv4/etharp.c"
#include "../../../third_party/lwip/src/core/ipv4/icmp.c"
#include "../../../third_party/lwip/src/core/ipv4/igmp.c"
#include "../../../third_party/lwip/src/core/ipv4/ip4.c"
#include "../../../third_party/lwip/src/core/ipv4/ip4_addr.c"
#include "../../../third_party/lwip/src/core/ipv4/ip4_frag.c"

/* IPv6 */
#include "../../../third_party/lwip/src/core/ipv6/dhcp6.c"
#include "../../../third_party/lwip/src/core/ipv6/ethip6.c"
#include "../../../third_party/lwip/src/core/ipv6/icmp6.c"
#include "../../../third_party/lwip/src/core/ipv6/inet6.c"
#include "../../../third_party/lwip/src/core/ipv6/ip6.c"
#include "../../../third_party/lwip/src/core/ipv6/ip6_addr.c"
/* Rename static reassdatagrams in ip6_frag to avoid symbol collision with ip4_frag */
#define reassdatagrams ip6_reassdatagrams
#include "../../../third_party/lwip/src/core/ipv6/ip6_frag.c"
#undef reassdatagrams
#include "../../../third_party/lwip/src/core/ipv6/mld6.c"
#include "../../../third_party/lwip/src/core/ipv6/nd6.c"

/* SmartProxy adapter */
#include "c/lwip_adapter.c"
