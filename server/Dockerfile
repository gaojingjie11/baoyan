# 多阶段构建：编译阶段用 golang 镜像，运行阶段用极小的 alpine
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/server .

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/server /app/server
RUN addgroup -S app && adduser -S -G app app
USER app
EXPOSE 8080
CMD ["/app/server"]
