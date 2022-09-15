
.PHONY: barbe
barbe:
	BARBE_RELEASE=1 ./build.sh

.PHONY: barbe-dev
barbe-dev:
	BARBE_DEV=1 BARBE_RELEASE=1 ./build.sh