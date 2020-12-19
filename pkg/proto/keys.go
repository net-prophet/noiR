package proto

// This is a mini-DSL for describing key hierarchy in redis
// The complex KeyMap/KeyGroup never actually get used!!
// Except as the reference implementation of the key hierarchy
// So you can use it to generate a nice looking explanation of the keys
// and some helper functions

const (
	TYPE_SET   = 0
	TYPE_SETNX = 1
	TYPE_MAP  = 2
	TYPE_LIST  = 3
)

type KeyMap struct {
	groups map[string][]KeyGroup
}

type KeyGroup struct {
	path          string
	pathParameter string
	mapParameter string
	keyType       int32
}

func NewKeyMap() KeyMap {
	return KeyMap{
		groups: map[string][]KeyGroup{
			"nodes": {
				KeyGroup{
					path:          "data",
					pathParameter: "", //  none
					mapParameter: "node",
					keyType:       TYPE_MAP,
				},
				KeyGroup{
					path:          "cleanup",
					pathParameter: "target",
					keyType:       TYPE_SETNX,
				},
			},
			"rooms": {
				KeyGroup{
					path:          "data",
					keyType:       TYPE_SET,
				},
				KeyGroup{
					path: "nodes",
					pathParameter: "node",
					mapParameter: "room",
					keyType: TYPE_MAP,
				},
			},
			"users": {
				KeyGroup{
					path:          "data",
					keyType:       TYPE_SET,
				},
				KeyGroup{
					path: "rooms",
					pathParameter: "room",
					mapParameter: "user",
					keyType: TYPE_MAP,
				},
			},
		},
	}
}

// Node Map

func KeyNodeMap() string {
	return "noir/map/nodes/"
}

// Data Keys

func KeyRoomData(roomID string) string {
	return "noir/obj/room/" + roomID
}

func KeyUserData(userID string) string {
	return "noir/obj/user/" + userID
}

// Reverse Relations

func KeyNodeRooms(nodeID string) string {
	return "noir/map/nodeRooms/" + nodeID
}

func KeyRoomUsers(roomID string) string {
	return "noir/map/roomUsers/" + roomID
}

// Channel Topics

func KeyWorkerTopic(nodeID string) string {
	return "noir/topic/worker/" + nodeID
}

func KeyTopicToPeer(peerID string) string {
	return "noir/topic/pc/" + peerID
}

func KeyTopicFromPeer(peerID string) string {
	return "noir/topic/client/" + peerID
}
