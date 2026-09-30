#ifndef SMARTPROXY_LWIP_ARCH_CC_H
#define SMARTPROXY_LWIP_ARCH_CC_H

#include <stdint.h>
#include <stdlib.h>
#include <stdio.h>
#include <assert.h>

#define LWIP_PLATFORM_ASSERT(x) do {     fprintf(stderr, "lwIP assert: %s\\n", (x));     abort(); } while (0)

#define LWIP_PLATFORM_DIAG(x) do { fprintf(stderr, x); } while (0)

#define PACK_STRUCT_FIELD(x) x
#define PACK_STRUCT_STRUCT __attribute__((packed))
#define PACK_STRUCT_BEGIN
#define PACK_STRUCT_END

#define LWIP_RAND() ((u32_t)rand())

#endif
