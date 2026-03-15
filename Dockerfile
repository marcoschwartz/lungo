FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN cp runtime/lungo.js pkg/lungo/lungo_runtime.js
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /server ./_example/

FROM scratch
COPY --from=builder /server /server
EXPOSE 3000
ENTRYPOINT ["/server"]
