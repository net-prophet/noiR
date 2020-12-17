package proto

func KeyNodeMap() string {
	return "noir/map/nodes"
}

func KeyRoomToNode(roomID string) string {
	return "noir/list/room2node/" + roomID
}

func KeyPeerToRoom(peerID string) string {
	return "noir/list/peer2room/" + peerID
}

func KeyRoomData(roomID string) string {
	return "noir/obj/room/" + roomID
}

func KeyWorkerData(nodeID string) string {
	return "noir/obj/node/" + nodeID
}

func KeyWorkerTopic(nodeID string) string {
	return "noir/to/worker/" + nodeID
}

func KeyPeerData(peerID string) string {
	return "noir/obj/peer/" + peerID
}

func KeyTopicToPeer(peerID string) string {
	return "noir/to/pc/" + peerID
}

func KeyTopicFromPeer(peerID string) string {
	return "noir/to/client/" + peerID
}
