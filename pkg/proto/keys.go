package proto

func KeyRoomToWorker(roomID string) string {
	return "noir/list/worker.rooms/" + roomID
}

func KeyPeerToRoom(peerID string) string {
	return "noir/list/room.peers/" + peerID
}

func KeyRoomData(roomID string) string {
	return "noir/obj/room/" + roomID
}

func KeyWorkerData(workerID string) string {
	return "noir/obj/worker/" + workerID
}

func KeyWorkerTopic(workerID string) string {
	return "noir/to/worker/" + workerID
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
