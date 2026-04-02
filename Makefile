.PHONY: all build clean install-deps freerdp go-build run

PROJECT_ROOT := $(shell pwd)
FREERDP_SRC := $(PROJECT_ROOT)/src/FreeRDP
FREERDP_BUILD := $(PROJECT_ROOT)/build/freerdp
FREERDP_INSTALL := $(PROJECT_ROOT)/install

all: build

# 安装系统依赖
install-deps:
	@echo "安装 FreeRDP 编译依赖..."
	sudo apt-get update
	sudo apt-get install -y cmake build-essential pkg-config \
		libssl-dev libx11-dev libxext-dev libxinerama-dev \
		libxcursor-dev libxdamage-dev libxv-dev libxkbfile-dev \
		libasound2-dev libcups2-dev libxml2-dev libxrandr-dev \
		libgstreamer1.0-dev libgstreamer-plugins-base1.0-dev \
		libxi-dev libavutil-dev libavcodec-dev libxtst-dev \
		libgtk-3-dev libgcrypt20-dev libpulse-dev \
		libusb-1.0-0-dev libudev-dev libdbus-glib-1-dev \
		uuid-dev libxkbcommon-dev

# 编译 FreeRDP
freerdp:
	@echo "编译 FreeRDP..."
	mkdir -p $(FREERDP_BUILD)
	mkdir -p $(FREERDP_INSTALL)
	cd $(FREERDP_BUILD) && cmake $(FREERDP_SRC) \
		-DCMAKE_INSTALL_PREFIX=$(FREERDP_INSTALL) \
		-DCMAKE_BUILD_TYPE=Release \
		-DWITH_SSE2=ON \
		-DWITH_CUPS=OFF \
		-DWITH_WAYLAND=OFF \
		-DWITH_PULSE=OFF \
		-DWITH_FFMPEG=OFF \
		-DWITH_GSTREAMER_1_0=OFF \
		-DWITH_CLIENT=OFF \
		-DWITH_SERVER=OFF \
		-DBUILD_TESTING=OFF \
		-DCHANNEL_URBDRC=OFF
	cd $(FREERDP_BUILD) && make -j$$(nproc)
	cd $(FREERDP_BUILD) && make install

# 编译 Go 项目
go-build: freerdp
	@echo "编译 Go 项目..."
	go mod download
	CGO_CFLAGS="-I$(FREERDP_INSTALL)/include" \
	CGO_LDFLAGS="-L$(FREERDP_INSTALL)/lib -L$(FREERDP_INSTALL)/lib/x86_64-linux-gnu -lfreerdp2 -lfreerdp-client2 -lwinpr2" \
	go build -o go-freerdp-webconnect

# 完整编译
build: go-build
	@echo "编译完成!"
	@echo "运行: ./run.sh -h <hostname> -u <username> -p <password>"

# 清理
clean:
	rm -rf $(FREERDP_BUILD)
	rm -rf $(FREERDP_INSTALL)
	rm -rf build
	rm -f go-freerdp-webconnect

# 运行
run:
	LD_LIBRARY_PATH=$(FREERDP_INSTALL)/lib:$(FREERDP_INSTALL)/lib/x86_64-linux-gnu:$$LD_LIBRARY_PATH \
	./go-freerdp-webconnect
