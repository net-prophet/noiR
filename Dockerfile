FROM golang:1.14.10-stretch

ENV GO111MODULE=on

WORKDIR $GOPATH/src/github.com/net-prophet/noir

COPY go.mod go.sum ./
RUN cd $GOPATH/src/github.com/net-prophet/noir && go mod download

COPY pkg/ $GOPATH/src/github.com/net-prophet/noir/pkg
COPY cmd/ $GOPATH/src/github.com/net-prophet/noir/cmd

WORKDIR $GOPATH/src/github.com/net-prophet/noir/cmd/noir
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /sfu .

FROM alpine:3.12.0

RUN apk --no-cache add ca-certificates
COPY --from=0 /sfu /usr/local/bin/sfu

COPY config.toml /configs/sfu.toml
COPY demo/ ./demo

ENTRYPOINT ["/usr/local/bin/sfu"]
CMD ["-c", "/configs/sfu.toml"]