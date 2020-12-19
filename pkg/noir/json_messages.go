package noir

import "github.com/pion/webrtc/v3"

// Join message sent when initializing a peer connection
type Join struct {
	Sid   string                    `json:"sid"`
	Offer webrtc.SessionDescription `json:"offer"`
}

// Negotiation message sent when renegotiating the peer connection
type Negotiation struct {
	Desc webrtc.SessionDescription `json:"desc"`
}
