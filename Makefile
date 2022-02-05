
.PHONY: build
build: build_echosrv build_spammer

.PHONY: build_echosrv
build_echosrv:
	go build ./cmd/echosrv

.PHONY: build_spammer
build_spammer:
	go build ./cmd/spammer

.PHONY: clean_echosrv
clean_echosrv:
	rm echosrv || true


.PHONY: clean_spammer
clean_spammer:
	rm spammer || true

clean: clean_echosrv clean_spammer
