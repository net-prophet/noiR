<h1 align="center">
  <br>
  noiR - Redis + ion SFU 
  <br>
</h1>
<h4 align="center">WIP NOTICE: noiR is pre-alpha and unstable, do not build on noiR</h4>
<p align="center">
  <a href="https://pion.ly/slack"><img src="https://img.shields.io/badge/ion%20chat-%20on%20slack-gray.svg?longCache=true&logo=slack&colorB=brightgreen" alt="Slack Widget"></a>
  <img src="https://github.com/net-prophet/noir/workflows/Publish%20Docker%20image/badge.svg" />
  <!--
  <a href="https://pkg.go.dev/github.com/net-prophet/noir"><img src="https://godoc.org/github.com/net-prophet/noir?status.svg" alt="GoDoc"></a>
  <a href="https://codecov.io/gh/net-prophet/noir"><img src="https://codecov.io/gh/net-prophet/noir/branch/master/graph/badge.svg" alt="Coverage Status"></a>
  -->
  <a href="https://goreportcard.com/report/github.com/net-prophet/noir"><img src="https://goreportcard.com/badge/github.com/net-prophet/noir" alt="Go Report Card"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT"></a>
</p>
<br>

*noiR: it's like R(edis)-ion, but in reverse*

### Features
+ Blazing fast full-featured WebRTC SFU for modern video conferences
+ Room Limits - Limit the number of members, publishers, or the maximum age of a room
+ "Channels" - A channel is a 1-publisher fanout room with publish authentication
+ (WIP) Basic Authentication - Require a password to join, or require a password to publish
+ (WIP) Admin API for creating rooms, managing members and seeing health statistics
+ (Planned) Stream Recording - Automatically record all streams in a room and upload them to S3
+ (Planned) Play into Room - Play a video or audio file from local disk or URL into a room or channel
+ (Planned) Admin Dashboard

### About

`noiR` is a redis-backed SFU cluster based on [`ion-sfu`](https://github.com/pion/ion-sfu).
A [selective forwarding unit](https://webrtcglossary.com/sfu/) is a video routing service which allows webrtc sessions to scale more efficiently.

`noiR` listens on the same ports as `ion-sfu` and accepts the same commands, while adding admin commands, support for
managing membership, and a *very naive* single-datacenter scaling model.

Existing `ion` jsonrpc clients can connect to any `noiR` node in the cluster.
When `noiR` receives a message from a peer, it keeps the websocket open with the peer for signalling,
but all messages are forwarded over redis to whichever `noir` node is hosting the room.

`noiR` keeps track of how many publishers or subscribers are in a room, and manages client lifecycle and cleanup.

Clients (callers) can connect directly to `noiR` over public `jsonrpc` interfaces using `ion-sdk-js`
or any regular `ion-sfu` client; management commands get sent over a separate `jsonrpc` interface.

<img src="./architecture.png" />
**Admin APIs are WIP, currently you must submit admin commands via redis**

### Usage

First of all, "don't use `noiR`"! That's just reasonable advice -- it's a very young project, we are novice golang devs,
the architecture is unproven, and the whole API might change. If you do use `noiR`, you will immediately find bugs, or
need new features, and we welcome all the help you can send! `noiR` might never be more than a neat
demonstration without your help.

###### Docker Run Demo
1. Ensure redis is running on your localhost

` docker run -p 6379:6379 --name redis sameersbn/redis`

2. Start `noiR` with Demo Mode at [`http://localhost:7070`](http://localhost:7070)

`docker run --net host ghcr.io/net-prophet/noir:latest -d :7070 -j :7000`

*(instead of host networking, you can use `-p 7070:7070 -p 7000:7000 -p 5000-5020:5000-5020/udp`)*

###### Build Binary
`make build`

###### Build Docker Image
`docker build . -t net-prophet/noir` or `make docker`

### Progress

- [x] Build a naive prototype redis messaging server for `ion-sfu`
- [x] Client JSONRPC API :7000 - JSONRPC-Redis Bridge (so the `ion-sfu/examples` work)
- [x] "Doing It Live" - Adapted my own dependent codebases to start using `noiR` immediately
- [x] "Demo Mode": Bundled `ion-sdk-react` storybooks for instant testing in a browser
- [x] Learn to write golang unit tests; write unit tests :'(
- [ ] (50%) Ensure cleanup safety, no dead peers
- [ ] (80%) Room permissions - Allow/Deny new joins, channels, basicauth room passwords, admin squelch + kick
- [ ] Admin JSONRPC API :7001 (management commands and/or multiplexed client connections)
- [ ] Admin gRPC API :50051 (management commands and/or multiplexed client connections)
- [ ] Stream permissions - Fine-grained control over audio/video publish permissions
- [ ] Load test cluster mode with `ion-load-tool` (depends on `gRPC` API)
- [ ] In-cluster SFU-SFU Relay (HA Stream Mirroring, Large Room Support)
- [ ] Cross-cluster SFU-SFU Relay (Geo-Stream Mirroring)

### Motivation

We started this project as an exercise to develop our golang skills, but also to scratch an itch -- `ion-sfu`
aspires to be the most lightweight SFU, the simplest possible version of itself. `noiR` is, by contrast, highly
opinionated and with more batteries included. If you're building a next-generation video conferencing solution,
and want a lightweight SFU that's ready out-of-the-box, `noiR` might be right for you.

### Contributing

We eagerly invite contributions and support. The golang might be messy or suboptimal, due to inexperience, 
and drive-by code reviews or notes on how to improve are greatly appreciated. If you'd like to work on `noiR`
but aren't sure how to help, you can get in touch with `oss [@] netprophet [.] tech` or open an issue.

### License

MIT License - see [LICENSE](LICENSE) for full text
