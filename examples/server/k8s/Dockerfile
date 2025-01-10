# 使用 Distroless 镜像运行程序
FROM gcr.io/distroless/base
WORKDIR /app
COPY ../server .
EXPOSE 80
CMD ["/app/server"]