# 官方 Go 镜像，自带完整工具链
FROM golang:1.22

# Debian 登录 shell 会重置 PATH；显式保留官方镜像中的 Go 工具链路径
RUN printf '%s\n' 'export PATH="/go/bin:/usr/local/go/bin:$PATH"' > /etc/profile.d/go-path.sh

WORKDIR /app

# 先复制依赖文件并下载依赖，保证容器内离线可编译和测试
COPY go.mod go.sum ./
RUN go mod download

# 复制完整项目源码
COPY . .

# 预编译一次并保留完整 Go 工具链
RUN go build ./...

CMD ["bash"]
