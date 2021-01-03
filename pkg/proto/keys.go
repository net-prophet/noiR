package proto

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

func KeyRouterTopic() string {
	return "noir/topic/router"
}

func KeyWorkerTopic(nodeID string) string {
	return "noir/topic/worker/" + nodeID
}

func KeyTopicToPeer(peerID string) string {
	return "noir/topic/pc/" + peerID
}

func KeyTopicFromPeer(peerID string) string {
	return "noir/topic/client/" + peerID
}

// Topic News Channels - PUBLISH when topic has new messages

func KeyPeerNewsChannel(peerID string) string {
	return "noir/news/peers/" + peerID
}
