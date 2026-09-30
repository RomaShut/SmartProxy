#ifndef SMARTPROXY_LWIPOPTS_H
#define SMARTPROXY_LWIPOPTS_H

#define NO_SYS 1
#define SYS_LIGHTWEIGHT_PROT 0
#define LWIP_SOCKET 0
#define LWIP_NETCONN 0
#define LWIP_DNS 0

#define LWIP_IPV4 1
#define LWIP_IPV6 1
#define LWIP_TCP 1
#define LWIP_UDP 1
#define LWIP_ICMP 1
#define LWIP_ICMP6 1

#define LWIP_ARP 0
#define LWIP_ETHERNET 0
#define LWIP_DHCP 0
#define LWIP_DHCP6 0
#define LWIP_AUTOIP 0
#define LWIP_IGMP 0
#define LWIP_MLD 0
#define LWIP_SNMP 0
#define LWIP_HTTPD 0

#define LWIP_STATS 0
#define LWIP_DEBUG 0
#define LWIP_PROVIDE_ERRNO 0

#define MEM_ALIGNMENT 4
#define MEM_SIZE (64 * 1024)
#define MEMP_NUM_TCP_PCB 128
#define MEMP_NUM_TCP_PCB_LISTEN 32
#define MEMP_NUM_TCP_SEG 256
#define MEMP_NUM_UDP_PCB 128

#define TCP_MSS 1460
#define TCP_SND_BUF (16 * TCP_MSS)
#define TCP_WND (16 * TCP_MSS)
#define TCP_QUEUE_OOSEQ 0

#endif
