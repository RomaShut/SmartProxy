include Makefile

smoke: libsmartproxy_lwip.a smoke.o
	$(CC) smoke.o libsmartproxy_lwip.a -o smoke
	./smoke

smoke.o: smoke.c lwip_adapter.h lwipopts.h
	$(CC) $(CPPFLAGS) $(CFLAGS) -c $< -o $@

.PHONY: smoke
