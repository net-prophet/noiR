<h1 align="center">
  <br>
  noiR - Redis + ion SFU 
  <br>
</h1>
<h4 align="center"></h4>
<p align="center">
  <a href="https://pion.ly/slack"><img src="https://img.shields.io/badge/join-us%20on%20slack-gray.svg?longCache=true&logo=slack&colorB=brightgreen" alt="Slack Widget"></a>
  <a href="https://pkg.go.dev/github.com/net-prophet/noir"><img src="https://godoc.org/github.com/net-prophet/noir?status.svg" alt="GoDoc"></a>
  <a href="https://codecov.io/gh/net-prophet/noir"><img src="https://codecov.io/gh/net-prophet/noir/branch/master/graph/badge.svg" alt="Coverage Status"></a>
  <a href="https://goreportcard.com/report/github.com/net-prophet/noir"><img src="https://goreportcard.com/badge/github.com/net-prophet/noir" alt="Go Report Card"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT"></a>
</p>
<br>

*noiR: it's like R(edis)-ion, but in reverse*

### About

`noiR` is a redis-backed SFU cluster based on [`ion-sfu`](https://github.com/pion/ion-sfu).
It listens on the same ports as `ion-sfu` and accepts the same commands, while adding extra support for
managing membership, and a naive single-datacenter scaling model.

Any ion client can send signaling messages to any `noiR` node in the cluster.
When `noiR` receives a message from a peer, it forwards the messages to whichever `noir` node is hosting the room.
`noiR` keeps track of how many publishers or subscribers are in a room, and manages client lifecycle and cleanup.

Clients (callers) can connect directly to `noiR` over public `gRPC` or `jsonrpc` interfaces,
using any regular `ion-sfu` client; management commands will be sent over a separate `jsonrpc` interface.

<img src="./architecture.png" />

### Usage

You'll need `ion-sfu:v1.4.2` running in `gRPC` mode, and with open UDP ports for conferencing; for development or testing --

`docker run --net=host docker.io/pionwebrtc/ion-sfu:v1.4.2-grpc`

###### Docker Run Demo
`docker run -p 7000:7000 -p 7001:7001 docker.netprophet.tech/netp/noir:latest`

###### Build Binary
`make build`

###### Build Docker Image
`docker build . -t net-prophet/noir` or `make docker`

### Progress

- [x] Build a naive prototype redis messaging server for `ion-sfu`
- [x] Client JSONRPC API :7000 - JSONRPC-Redis Bridge (so the `ion-sfu/examples` work)
- [x] Adapt my dependent codebases to start using `noiR` immediately
- [ ] Learn to write golang unit tests; write unit tests :'(
- [ ] Admin JSONRPC API :7001 (management commands and/or multiplexed client connections)
- [ ] Admin gRPC API :50051 (management commands and/or multiplexed client connections)
- [ ] "Demo Mode": Bundled `ion-sdk-react` storybooks for instant testing in a browser
- [ ] Room permissions - Allow/Deny new joins, basicauth room passwords, admin squelch + kick
- [ ] Stream permissions - Fine-grained control over audio/video publish permissions
- [ ] In-cluster SFU-SFU Relay (HA Stream Mirroring, Large Room Support)
- [ ] Cross-cluster SFU-SFU Relay (Geo-Stream Mirroring)

### Motivation

We started this project as an exercise to develop our golang skills, and also to scratch an itch -- `ion-sfu`
aspires to be the most lightweight, the simplest possible version of itself. `noiR` is, by contrast, highly
opinionated and with more batteries included. If you're building a next-generation video conferencing solution,
and want a natural, flexible API for managing your sessions, `noiR` might be right for you.

### Contributing

We eagerly invite contributions and support. The golang might be messy or suboptimal, due to inexperience, 
and drive-by code reviews or notes on how to improve are greatly appreciated. If you'd like to work on `noiR`
but aren't sure how to help, you can get in touch with `oss [@] netprophet [.] tech` or open an issue.

### License

MIT License - see [LICENSE](LICENSE) for full text
