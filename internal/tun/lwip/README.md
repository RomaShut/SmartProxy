# SmartProxy lwIP backend

## Local build

Initialize the pinned lwIP submodule:

    git submodule update --init --recursive

Run the C compile, static-link, and initialization smoke test:

    sh ./internal/tun/lwip/build.sh

The smoke test does not send real TUN traffic yet. It verifies that the selected lwIP source set, configuration, adapter, static link, and basic lwip_init/netif_add path compile and run together.

## CI

.github/workflows/lwip.yml checks out the pinned submodule and runs the same build script on every change under the lwIP backend.

## Scope of this stage

- TUN fd remains owned by Go.
- lwIP uses NO_SYS=1/raw API.
- lwIP DNS/socket/netconn are disabled.
- Production gVisor selection is unchanged.
- TCP/UDP Go adapters and packet-path integration are the next stage.

The source revision is pinned by the third_party/lwip submodule. Do not edit upstream lwIP directly.
